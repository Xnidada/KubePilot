package workload

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ListPodFiles 列出Pod内文件
func (h *Handler) ListPodFiles(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")
	containerName := c.Query("container")
	path := c.DefaultQuery("path", "/")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 如果没有指定容器，使用第一个容器
	if containerName == "" {
		ctx := context.Background()
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "pod not found")
			return
		}
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
		}
	}

	// 尝试多种方式列出文件
	var output string
	var success bool

	// 尝试 ls -la
	output, err = execCommandInPod(client, namespace, podName, containerName, []string{"ls", "-la", path})
	if err == nil {
		success = true
	}

	// 尝试 /bin/ls
	if !success {
		output, err = execCommandInPod(client, namespace, podName, containerName, []string{"/bin/ls", "-la", path})
		if err == nil {
			success = true
		}
	}

	// 尝试 find
	if !success {
		output, err = execCommandInPod(client, namespace, podName, containerName, []string{"find", path, "-maxdepth", "1"})
		if err == nil {
			success = true
		}
	}

	if !success {
		response.BadRequest(c, "此容器使用精简镜像，不支持文件管理功能。请使用包含完整工具的镜像（如 alpine、ubuntu 等）")
		return
	}

	response.Success(c, gin.H{
		"path":      path,
		"output":    output,
		"pod":       podName,
		"container": containerName,
	})
}

// ReadPodFile 读取Pod文件内容
func (h *Handler) ReadPodFile(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")
	containerName := c.Query("container")
	filePath := c.Query("path")

	if filePath == "" {
		response.BadRequest(c, "file path is required")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 执行 cat 命令读取文件
	output, err := execCommandInPod(client, namespace, podName, containerName, []string{"cat", filePath})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"path":    filePath,
		"content": output,
	})
}

// WritePodFile 写入Pod文件
func (h *Handler) WritePodFile(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")

	var req struct {
		Container string `json:"container"`
		Path      string `json:"path" binding:"required"`
		Content   string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 使用 tee 命令写入文件（通过 stdin 传递内容）
	// 这种方式不需要 shell 的 heredoc 语法
	cmd := []string{"tee", req.Path}
	_, err = execCommandWithInput(client, namespace, podName, req.Container, cmd, req.Content)
	if err != nil {
		// 如果 tee 失败，尝试使用 sh -c
		cmd := fmt.Sprintf("cat > %s << 'KUBOPILOT_EOF'\n%s\nKUBOPILOT_EOF", req.Path, req.Content)
		_, err = execCommandInPod(client, namespace, podName, req.Container, []string{"sh", "-c", cmd})
		if err != nil {
			response.InternalError(c, "写入文件失败，容器可能不支持文件操作: "+err.Error())
			return
		}
	}

	response.SuccessWithMessage(c, "file written successfully", nil)
}

// DeletePodFile 删除Pod文件
func (h *Handler) DeletePodFile(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")
	containerName := c.Query("container")
	filePath := c.Query("path")

	if filePath == "" {
		response.BadRequest(c, "file path is required")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 执行 rm 命令删除文件
	_, err = execCommandInPod(client, namespace, podName, containerName, []string{"rm", "-f", filePath})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "file deleted successfully", nil)
}

// execCommandInPod 在Pod中执行命令
func execCommandInPod(client *k8s.ClusterClient, namespace, podName, containerName string, command []string) (string, error) {
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %w", stderr.String(), err)
		}
		return "", err
	}

	return stdout.String(), nil
}

// execCommandWithInput 在Pod中执行命令并传递stdin
func execCommandWithInput(client *k8s.ClusterClient, namespace, podName, containerName string, command []string, input string) (string, error) {
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	stdin := bytes.NewBufferString(input)

	err = executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %w", stderr.String(), err)
		}
		return "", err
	}

	return stdout.String(), nil
}

// DownloadPodFile 下载Pod文件
func (h *Handler) DownloadPodFile(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	podName := c.Param("name")
	containerName := c.Query("container")
	filePath := c.Query("path")

	if filePath == "" {
		response.BadRequest(c, "file path is required")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 执行 cat 命令读取文件
	output, err := execCommandInPod(client, namespace, podName, containerName, []string{"cat", filePath})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 设置下载响应头
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	c.Header("Content-Type", "application/octet-stream")
	c.Data(200, "application/octet-stream", []byte(output))
}
