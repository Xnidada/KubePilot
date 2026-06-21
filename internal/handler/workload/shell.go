package workload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

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

// NodeShell 节点终端WebSocket连接
func (h *Handler) NodeShell(c *gin.Context) {
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
	nodeName := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 升级WebSocket连接
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("failed to upgrade websocket", zap.Error(err))
		return
	}
	defer ws.Close()

	// 创建一个特权Pod来访问节点
	podName := fmt.Sprintf("node-shell-%s-%d", nodeName, time.Now().UnixNano())
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
			Labels: map[string]string{
				"app": "node-shell",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			HostPID:       true,
			HostNetwork:   true,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "shell",
					Image: "alpine:latest",
					Command: []string{
						"nsenter",
						"-t", "1",
						"-m",
						"-u",
						"-i",
						"-n",
						"--",
						"/bin/bash",
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: boolPtr(true),
					},
					TTY:   true,
					Stdin: true,
				},
			},
			Tolerations: []corev1.Toleration{
				{
					Effect:   corev1.TaintEffectNoSchedule,
					Operator: corev1.TolerationOpExists,
				},
				{
					Effect:   corev1.TaintEffectNoExecute,
					Operator: corev1.TolerationOpExists,
				},
			},
		},
	}

	ctx := context.Background()

	// 创建Pod
	_, err = client.Clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error creating shell pod: %v\r\n", err)))
		ws.Close()
		return
	}

	// 等待Pod运行
	ws.WriteMessage(websocket.TextMessage, []byte("正在启动节点终端...\r\n"))
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		p, err := client.Clientset.CoreV1().Pods("default").Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if p.Status.Phase == corev1.PodRunning {
			break
		}
		if p.Status.Phase == corev1.PodFailed {
			ws.WriteMessage(websocket.TextMessage, []byte("终端启动失败\r\n"))
			cleanupPod(client, podName)
			ws.Close()
			return
		}
	}

	// 执行exec
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace("default").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "shell",
			Command:   []string{"/bin/bash"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", req.URL())
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\r\n", err)))
		cleanupPod(client, podName)
		ws.Close()
		return
	}

	// 创建流
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

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

	// 执行命令
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdinReader,
		Stdout: stdoutWriter,
		Tty:    true,
	})

	if err != nil {
		logger.Error("stream error", zap.Error(err))
	}

	stdinWriter.Close()
	stdoutWriter.Close()
	wg.Wait()

	// 清理Pod
	cleanupPod(client, podName)
}


// cleanupPod 清理临时Pod
func cleanupPod(client *k8s.ClusterClient, podName string) {
	ctx := context.Background()
	client.Clientset.CoreV1().Pods("default").Delete(ctx, podName, metav1.DeleteOptions{})
}

// boolPtr 返回bool指针
func boolPtr(b bool) *bool {
	return &b
}
