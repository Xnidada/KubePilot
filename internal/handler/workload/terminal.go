package workload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许同源请求，生产环境应配置具体域名
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// 允许 localhost 和请求来源一致
		host := r.Host
		return origin == "http://"+host || origin == "https://"+host
	},
}

// PodTerminal Pod终端WebSocket连接
func (h *Handler) PodTerminal(c *gin.Context) {
	releaseSession, ok := beginTerminalSession(c)
	if !ok {
		return
	}
	defer releaseSession()

	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")
	containerName := c.Query("container")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取Pod信息
	ctx := c.Request.Context()
	pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pod not found"})
		return
	}

	// 如果没有指定容器，使用第一个容器
	if containerName == "" && len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
	}

	// 升级WebSocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("failed to upgrade websocket", zap.Error(err))
		return
	}
	session := newWebsocketSession(conn)
	defer session.Close()
	ctx = session.Context()

	// 尝试不同的shell
	shells := []string{"/bin/sh", "/bin/bash", "/bin/ash", "sh", "bash"}
	var shellCmd string
	for _, shell := range shells {
		// 测试shell是否可用
		testReq := client.Clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(namespace).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: containerName,
				Command:   []string{shell, "-c", "echo ok"},
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)

		testExecutor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", testReq.URL())
		if err == nil {
			var stdout, stderr bytes.Buffer
			err = testExecutor.StreamWithContext(ctx, remotecommand.StreamOptions{
				Stdout: &stdout,
				Stderr: &stderr,
			})
			if err == nil && stdout.String() == "ok\n" {
				shellCmd = shell
				break
			}
		}
	}

	if shellCmd == "" {
		session.SendText("错误: 容器中没有可用的shell (distroless镜像不支持终端连接)\r\n")
		session.SendText("提示: 此容器使用distroless基础镜像，不包含shell\r\n")
		return
	}

	// 创建exec请求
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   []string{shellCmd},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", req.URL())
	if err != nil {
		logger.Error("failed to create executor", zap.Error(err))
		session.SendText(fmt.Sprintf("Error: %v", err))
		return
	}

	// 创建流
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	var wg sync.WaitGroup

	// 从WebSocket读取发送到stdin
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdinWriter.Close()
		for {
			_, message, err := session.ReadMessage()
			if err != nil {
				return
			}
			_, err = stdinWriter.Write(message)
			if err != nil {
				return
			}
		}
	}()

	// 从stdout读取发送到WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdoutReader.Close()
		buf := make([]byte, 1024)
		for {
			n, err := stdoutReader.Read(buf)
			if err != nil {
				return
			}
			if !session.Send(websocket.TextMessage, buf[:n]) {
				return
			}
		}
	}()

	// 从stderr读取发送到WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stderrReader.Close()
		buf := make([]byte, 1024)
		for {
			n, err := stderrReader.Read(buf)
			if err != nil {
				return
			}
			if !session.Send(websocket.TextMessage, buf[:n]) {
				return
			}
		}
	}()

	// 执行命令
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdinReader,
		Stdout: stdoutWriter,
		Stderr: stderrWriter,
		Tty:    true,
	})

	if err != nil {
		logger.Error("stream error", zap.Error(err))
	}

	// Closing the session unblocks the WebSocket reader before waiting.
	session.Close()
	stdinWriter.Close()
	stdoutWriter.Close()
	stderrWriter.Close()
	wg.Wait()
}

// GetPodContainers 获取Pod容器列表
func (h *Handler) GetPodContainers(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()
	pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pod not found"})
		return
	}

	containers := make([]map[string]string, 0)
	for _, c := range pod.Spec.Containers {
		containers = append(containers, map[string]string{
			"name":  c.Name,
			"image": c.Image,
		})
	}

	// 也添加初始化容器
	for _, c := range pod.Spec.InitContainers {
		containers = append(containers, map[string]string{
			"name":  c.Name,
			"image": c.Image,
			"type":  "init",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": containers,
	})
}
