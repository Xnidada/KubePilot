package k8s

import (
	"fmt"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
)

// ClusterDBAdapter 数据库适配器
type ClusterDBAdapter struct {
	encryptKey string
}

// NewClusterDBAdapter 创建数据库适配器
func NewClusterDBAdapter(encryptKey string) *ClusterDBAdapter {
	return &ClusterDBAdapter{encryptKey: encryptKey}
}

// QueryClusterKubeconfig 从数据库查询集群的kubeconfig
func (a *ClusterDBAdapter) QueryClusterKubeconfig(clusterID uint) (string, error) {
	var cluster model.Cluster
	if err := model.DB.First(&cluster, clusterID).Error; err != nil {
		return "", fmt.Errorf("cluster not found: %w", err)
	}

	// 解密kubeconfig
	decrypted, err := crypto.Decrypt(cluster.Kubeconfig, a.encryptKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt kubeconfig: %w", err)
	}

	return decrypted, nil
}
