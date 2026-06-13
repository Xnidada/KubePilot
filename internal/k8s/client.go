package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type ClusterClient struct {
	Clientset     kubernetes.Interface
	MetricsClient metricsv.Interface
	Discovery     discovery.DiscoveryInterface
	RESTMapper    meta.RESTMapper
	Config        *rest.Config
	Namespace     string
}

type ClientManager struct {
	mu       sync.RWMutex
	clients  map[uint]*ClusterClient // clusterID -> client
	defaults *rest.Config
}

var Manager *ClientManager

func InitClientManager(qps float32, burst int) {
	Manager = &ClientManager{
		clients: make(map[uint]*ClusterClient),
		defaults: &rest.Config{
			QPS:   qps,
			Burst: burst,
		},
	}
}

func (cm *ClientManager) GetClient(clusterID uint) (*ClusterClient, error) {
	cm.mu.RLock()
	client, exists := cm.clients[clusterID]
	cm.mu.RUnlock()

	if exists {
		return client, nil
	}
	return nil, fmt.Errorf("client not found for cluster %d", clusterID)
}

func (cm *ClientManager) RegisterClient(clusterID uint, kubeconfig []byte, namespace string) error {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	config.QPS = cm.defaults.QPS
	config.Burst = cm.defaults.Burst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		logger.Warn("failed to create metrics client, metrics will be unavailable", zap.Error(err))
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	client := &ClusterClient{
		Clientset:     clientset,
		MetricsClient: metricsClient,
		Discovery:     discoveryClient,
		Config:        config,
		Namespace:     namespace,
	}

	cm.mu.Lock()
	cm.clients[clusterID] = client
	cm.mu.Unlock()

	return nil
}

func (cm *ClientManager) RemoveClient(clusterID uint) {
	cm.mu.Lock()
	delete(cm.clients, clusterID)
	cm.mu.Unlock()
}

func (cm *ClientManager) PingCluster(clusterID uint) error {
	client, err := cm.GetClient(clusterID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("cluster ping failed: %w", err)
	}

	_ = ctx
	return nil
}

func (cm *ClientManager) GetClusterInfo(clusterID uint) (*ClusterInfo, error) {
	client, err := cm.GetClient(clusterID)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	version, err := client.Clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	info := &ClusterInfo{
		Version:    version.GitVersion,
		NodeCount:  len(nodes.Items),
		Nodes:      make([]NodeInfo, 0, len(nodes.Items)),
	}

	var totalCPU, totalMemory resource.Quantity
	for _, node := range nodes.Items {
		totalCPU.Add(*node.Status.Capacity.Cpu())
		totalMemory.Add(*node.Status.Capacity.Memory())

		nodeInfo := NodeInfo{
			Name:        node.Name,
			IP:          getNodeAddress(node.Status.Addresses),
			CPUCapacity: node.Status.Capacity.Cpu().String(),
			MemCapacity: node.Status.Capacity.Memory().String(),
			OS:          node.Status.NodeInfo.OSImage,
			Kernel:      node.Status.NodeInfo.KernelVersion,
			ContainerRT: node.Status.NodeInfo.ContainerRuntimeVersion,
			KubeletVer:  node.Status.NodeInfo.KubeletVersion,
			Ready:       isNodeReady(node.Status.Conditions),
		}
		info.Nodes = append(info.Nodes, nodeInfo)
	}

	info.CPUCapacity = totalCPU.String()
	info.MemCapacity = totalMemory.String()

	return info, nil
}

func getNodeAddress(addresses []corev1.NodeAddress) string {
	for _, addr := range addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func isNodeReady(conditions []corev1.NodeCondition) bool {
	for _, cond := range conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

type ClusterInfo struct {
	Version     string     `json:"version"`
	NodeCount   int        `json:"node_count"`
	CPUCapacity string     `json:"cpu_capacity"`
	MemCapacity string     `json:"mem_capacity"`
	Nodes       []NodeInfo `json:"nodes"`
}

type NodeInfo struct {
	Name        string `json:"name"`
	IP          string `json:"ip"`
	CPUCapacity string `json:"cpu_capacity"`
	MemCapacity string `json:"mem_capacity"`
	OS          string `json:"os"`
	Kernel      string `json:"kernel"`
	ContainerRT string `json:"container_rt"`
	KubeletVer  string `json:"kubelet_ver"`
	Ready       bool   `json:"ready"`
}
