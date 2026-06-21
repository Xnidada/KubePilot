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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// nodeShellPodName 生成固定的 Pod 名称
func nodeShellPodName(nodeName string) string {
	return fmt.Sprintf("node-shell-%s", nodeName)
}

// nodeShellLabel 用于标识 node-shell Pod
const nodeShellLabel = "kubepilot/node-shell"

// lastUsedAnnotation 记录最后使用时间
const lastUsedAnnotation = "kubepilot/last-used"

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

	ctx := context.Background()
	podName := nodeShellPodName(nodeName)

	// 检查是否已存在 node-shell Pod
	existingPod, err := client.Clientset.CoreV1().Pods("default").Get(ctx, podName, metav1.GetOptions{})
	if err == nil {
		// Pod 存在，检查状态
		switch existingPod.Status.Phase {
		case corev1.PodRunning:
			// Pod 正在运行，更新最后使用时间并复用
			updateLastUsed(client, podName)
			ws.WriteMessage(websocket.TextMessage, []byte("正在连接到现有终端...\r\n"))
		case corev1.PodPending:
			// Pod 还在启动中，等待
			ws.WriteMessage(websocket.TextMessage, []byte("等待终端启动...\r\n"))
		default:
			// Pod 状态异常，删除后重建
			ws.WriteMessage(websocket.TextMessage, []byte("清理旧终端，重新启动...\r\n"))
			client.Clientset.CoreV1().Pods("default").Delete(ctx, podName, metav1.DeleteOptions{})
			time.Sleep(1 * time.Second)
			existingPod = nil
		}
	} else if !errors.IsNotFound(err) {
		// 获取 Pod 出错（非不存在）
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\r\n", err)))
		ws.Close()
		return
	} else {
		// Pod 不存在，创建新的
		existingPod = nil
	}

	// 如果 Pod 不存在，创建新的
	if existingPod == nil {
		ws.WriteMessage(websocket.TextMessage, []byte("正在启动节点终端...\r\n"))

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: "default",
				Labels: map[string]string{
					nodeShellLabel: nodeName,
				},
				Annotations: map[string]string{
					lastUsedAnnotation: time.Now().Format(time.RFC3339),
				},
			},
			Spec: corev1.PodSpec{
				NodeName:      nodeName,
				HostPID:       true,
				HostNetwork:   true,
				RestartPolicy: corev1.RestartPolicyAlways,
				Containers: []corev1.Container{
					{
						Name:  "shell",
						Image: "alpine:latest",
						Command: []string{
							"sh",
							"-c",
							// 安装 nsenter 和 bash，然后保持运行
							"apk add --no-cache util-linux bash && sleep infinity",
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

		_, err = client.Clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error creating shell pod: %v\r\n", err)))
			ws.Close()
			return
		}
	}

	// 等待 Pod 运行
	if existingPod == nil || existingPod.Status.Phase != corev1.PodRunning {
		ws.WriteMessage(websocket.TextMessage, []byte("等待终端就绪...\r\n"))
		running := false
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			p, err := client.Clientset.CoreV1().Pods("default").Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if p.Status.Phase == corev1.PodRunning {
				// 检查容器是否就绪
				if len(p.Status.ContainerStatuses) > 0 && p.Status.ContainerStatuses[0].Ready {
					running = true
					break
				}
			}
			if p.Status.Phase == corev1.PodFailed {
				ws.WriteMessage(websocket.TextMessage, []byte("终端启动失败\r\n"))
				client.Clientset.CoreV1().Pods("default").Delete(ctx, podName, metav1.DeleteOptions{})
				ws.Close()
				return
			}
		}
		if !running {
			ws.WriteMessage(websocket.TextMessage, []byte("终端启动超时\r\n"))
			ws.Close()
			return
		}
	}

	// 更新最后使用时间
	updateLastUsed(client, podName)

	// 执行 exec - 通过 nsenter 进入节点命名空间
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace("default").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "shell",
			Command:   []string{"nsenter", "-t", "1", "-m", "-u", "-i", "-n", "--", "/bin/bash"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", req.URL())
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\r\n", err)))
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

	// 断开连接后不删除 Pod，保留复用
	logger.Info("node shell disconnected", zap.String("node", nodeName))
}

// updateLastUsed 更新 Pod 的最后使用时间
func updateLastUsed(client *k8s.ClusterClient, podName string) {
	ctx := context.Background()
	pod, err := client.Clientset.CoreV1().Pods("default").Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[lastUsedAnnotation] = time.Now().Format(time.RFC3339)
	client.Clientset.CoreV1().Pods("default").Update(ctx, pod, metav1.UpdateOptions{})
}

// CleanupNodeShellPods 清理长时间未使用的 node-shell Pod
// 建议通过定时任务调用此函数，例如每 30 分钟执行一次
func CleanupNodeShellPods(client *k8s.ClusterClient, maxIdleDuration time.Duration) {
	ctx := context.Background()

	// 获取所有 node-shell Pod
	pods, err := client.Clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
		LabelSelector: nodeShellLabel,
	})
	if err != nil {
		logger.Error("failed to list node-shell pods", zap.Error(err))
		return
	}

	now := time.Now()
	for _, pod := range pods.Items {
		// 检查最后使用时间
		lastUsedStr, ok := pod.Annotations[lastUsedAnnotation]
		if !ok {
			// 没有最后使用时间标记，删除
			client.Clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
			logger.Info("cleaned up node-shell pod without last-used annotation", zap.String("pod", pod.Name))
			continue
		}

		lastUsed, err := time.Parse(time.RFC3339, lastUsedStr)
		if err != nil {
			// 时间格式错误，删除
			client.Clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
			logger.Info("cleaned up node-shell pod with invalid annotation", zap.String("pod", pod.Name))
			continue
		}

		// 检查是否超过最大空闲时间
		if now.Sub(lastUsed) > maxIdleDuration {
			client.Clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
			logger.Info("cleaned up idle node-shell pod",
				zap.String("pod", pod.Name),
				zap.Duration("idle", now.Sub(lastUsed)),
			)
		}
	}
}

// cleanupPod 清理临时Pod（保留兼容性）
func cleanupPod(client *k8s.ClusterClient, podName string) {
	ctx := context.Background()
	client.Clientset.CoreV1().Pods("default").Delete(ctx, podName, metav1.DeleteOptions{})
}

// boolPtr 返回bool指针
func boolPtr(b bool) *bool {
	return &b
}
