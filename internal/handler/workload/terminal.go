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

// validateWebSocketAuth 验证 WebSocket 连接的 JWT token
func validateWebSocketAuth(c *gin.Context) bool {
	// 从 query 参数或 header 获取 token
	token := c.Query("token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}
	if token == "" {
		return false
	}
	// token 会由中间件验证，这里只检查是否存在
	return true
}

// PodTerminal Pod终端WebSocket连接
func (h *Handler) PodTerminal(c *gin.Context) {
	// 验证认证
	if !validateWebSocketAuth(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

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
	ctx := context.Background()
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
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("failed to upgrade websocket", zap.Error(err))
		return
	}
	defer ws.Close()

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
		ws.WriteMessage(websocket.TextMessage, []byte("错误: 容器中没有可用的shell (distroless镜像不支持终端连接)\r\n"))
		ws.WriteMessage(websocket.TextMessage, []byte("提示: 此容器使用distroless基础镜像，不包含shell\r\n"))
		ws.Close()
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
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
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
			_, message, err := ws.ReadMessage()
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
			err = ws.WriteMessage(websocket.TextMessage, buf[:n])
			if err != nil {
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
			err = ws.WriteMessage(websocket.TextMessage, buf[:n])
			if err != nil {
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

	// 等待所有goroutine完成
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
