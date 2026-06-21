package k8s

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
)

// KubectlExecutor kubectl命令执行器
type KubectlExecutor struct {
	encryptKey string
}

// NewKubectlExecutor 创建执行器
func NewKubectlExecutor(encryptKey string) *KubectlExecutor {
	return &KubectlExecutor{encryptKey: encryptKey}
}

// ExecuteKubectl 执行kubectl命令
func (e *KubectlExecutor) ExecuteKubectl(ctx context.Context, clusterID uint, args []string) (bool, string, string, error) {
	kubeconfig, err := e.getKubeconfig(clusterID)
	if err != nil {
		return false, "", "", err
	}

	// 写入临时kubeconfig
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kubeconfig-%d-%d.yaml", clusterID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(kubeconfig), 0600); err != nil {
		return false, "", "", fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	defer os.Remove(tmpFile)

	cmdArgs := append([]string{"--kubeconfig", tmpFile}, args...)
	cmd := exec.CommandContext(ctx, "kubectl", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	output := stdout.String()
	errMsg := ""

	if err != nil {
		errMsg = stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return false, output, errMsg, nil
	}

	return true, output, "", nil
}

// ExecuteKubectlApply 执行kubectl apply
func (e *KubectlExecutor) ExecuteKubectlApply(ctx context.Context, clusterID uint, yamlContent string) (bool, string, string, error) {
	tmpYAML := filepath.Join(os.TempDir(), fmt.Sprintf("apply-%d-%d.yaml", clusterID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpYAML, []byte(yamlContent), 0644); err != nil {
		return false, "", "", fmt.Errorf("failed to write YAML: %w", err)
	}
	defer os.Remove(tmpYAML)

	return e.ExecuteKubectl(ctx, clusterID, []string{"apply", "-f", tmpYAML})
}

// ExecuteKubectlDelete 执行kubectl delete
func (e *KubectlExecutor) ExecuteKubectlDelete(ctx context.Context, clusterID uint, yamlContent string) (bool, string, string, error) {
	tmpYAML := filepath.Join(os.TempDir(), fmt.Sprintf("delete-%d-%d.yaml", clusterID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpYAML, []byte(yamlContent), 0644); err != nil {
		return false, "", "", fmt.Errorf("failed to write YAML: %w", err)
	}
	defer os.Remove(tmpYAML)

	return e.ExecuteKubectl(ctx, clusterID, []string{"delete", "-f", tmpYAML})
}

// getKubeconfig 获取kubeconfig
func (e *KubectlExecutor) getKubeconfig(clusterID uint) (string, error) {
	var cluster model.Cluster
	if err := model.DB.First(&cluster, clusterID).Error; err != nil {
		return "", fmt.Errorf("cluster not found: %w", err)
	}

	// 解密kubeconfig
	decrypted, err := crypto.Decrypt(cluster.Kubeconfig, e.encryptKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt kubeconfig: %w", err)
	}

	return decrypted, nil
}
