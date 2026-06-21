package aiops

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

// KubectlResult kubectl执行结果
type KubectlResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// ExecuteKubectl 执行kubectl命令
func (s *Service) ExecuteKubectl(ctx context.Context, clusterID uint, args []string) (*KubectlResult, error) {
	// 获取kubeconfig
	kubeconfig, err := s.getKubeconfig(clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// 写入临时kubeconfig文件
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kubeconfig-%d-%d.yaml", clusterID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(kubeconfig), 0600); err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	defer os.Remove(tmpFile)

	// 构建kubectl命令
	cmdArgs := append([]string{"--kubeconfig", tmpFile}, args...)
	cmd := exec.CommandContext(ctx, "kubectl", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result := &KubectlResult{
		Output: stdout.String(),
	}

	if err != nil {
		result.Success = false
		result.Error = stderr.String()
		if result.Error == "" {
			result.Error = err.Error()
		}
	} else {
		result.Success = true
	}

	return result, nil
}

// ExecuteKubectlApply 执行kubectl apply
func (s *Service) ExecuteKubectlApply(ctx context.Context, clusterID uint, yamlContent string) (*KubectlResult, error) {
	// 写入临时YAML文件
	tmpYAML := filepath.Join(os.TempDir(), fmt.Sprintf("apply-%d-%d.yaml", clusterID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpYAML, []byte(yamlContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write YAML: %w", err)
	}
	defer os.Remove(tmpYAML)

	return s.ExecuteKubectl(ctx, clusterID, []string{"apply", "-f", tmpYAML})
}

// ExecuteKubectlDelete 执行kubectl delete
func (s *Service) ExecuteKubectlDelete(ctx context.Context, clusterID uint, resourceType, name, namespace string) (*KubectlResult, error) {
	args := []string{"delete", resourceType, name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return s.ExecuteKubectl(ctx, clusterID, args)
}

// getKubeconfig 获取集群的kubeconfig
func (s *Service) getKubeconfig(clusterID uint) (string, error) {
	var cluster model.Cluster
	if err := s.db.First(&cluster, clusterID).Error; err != nil {
		return "", fmt.Errorf("cluster not found: %w", err)
	}

	// 解密kubeconfig
	decrypted, err := crypto.Decrypt(cluster.Kubeconfig, s.encryptKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt kubeconfig: %w", err)
	}

	return decrypted, nil
}

// QueryWithKubectl 使用kubectl查询资源
func (s *Service) QueryWithKubectl(ctx context.Context, clusterID uint, queryType string) (*KubectlResult, error) {
	switch queryType {
	case "pods":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "pods", "-A", "-o", "wide"})
	case "deployments":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "deployments", "-A"})
	case "services":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "services", "-A"})
	case "nodes":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "nodes", "-o", "wide"})
	case "all":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "all", "-A"})
	case "events":
		return s.ExecuteKubectl(ctx, clusterID, []string{"get", "events", "-A", "--sort-by='.lastTimestamp'"})
	default:
		return nil, fmt.Errorf("unsupported query type: %s", queryType)
	}
}
