package k8s

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
)

// KubectlExecutor kubectl命令执行器
type KubectlExecutor struct {
	encryptKey string
}

// ExecuteKubectlStream emits each stdout/stderr line while kubectl is running.
// It is used for durable installer logs; existing callers keep the buffered API.
func (e *KubectlExecutor) ExecuteKubectlStream(ctx context.Context, clusterID uint, args []string, emit func(string, string)) (bool, error) {
	kubeconfig, err := e.getKubeconfig(clusterID)
	if err != nil {
		return false, err
	}
	file, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return false, err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return false, err
	}
	if _, err := file.WriteString(kubeconfig); err != nil {
		file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", file.Name()}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	var wg sync.WaitGroup
	var readErr error
	var readMu sync.Mutex
	for _, source := range []struct {
		name   string
		reader io.Reader
	}{{"info", stdout}, {"error", stderr}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(source.reader)
			scanner.Buffer(make([]byte, 4096), 128<<10)
			for scanner.Scan() {
				emit(source.name, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				emit("error", "kubectl output read failed: "+err.Error())
				readMu.Lock()
				readErr = err
				readMu.Unlock()
			}
		}()
	}
	wg.Wait()
	err = cmd.Wait()
	if err == nil {
		err = readErr
	}
	return err == nil, err
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
