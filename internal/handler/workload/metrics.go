package workload

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// roundTo2 保留两位小数
func roundTo2(f float64) float64 {
	return math.Round(f*100) / 100
}

// GetPodMetrics 获取Pod资源使用指标
func (h *Handler) GetPodMetrics(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 尝试从 metrics API 获取数据
	var podMetrics []PodMetricInfo

	if client.MetricsClient != nil {
		metrics, err := client.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if err == nil && len(metrics.Items) > 0 {
			// 成功获取到metrics数据
			for _, m := range metrics.Items {
				podMetric := PodMetricInfo{
					Name:      m.Name,
					Namespace: m.Namespace,
					Containers: make([]ContainerMetricInfo, 0),
				}

				var totalCPU, totalMemory int64
				for _, c := range m.Containers {
					cpu := c.Usage.Cpu().MilliValue()
					mem := c.Usage.Memory().Value() / (1024 * 1024) // Convert to Mi
					totalCPU += cpu
					totalMemory += mem

					podMetric.Containers = append(podMetric.Containers, ContainerMetricInfo{
						Name:        c.Name,
						CPUMillis:   cpu,
						MemoryMi:    mem,
					})
				}
				podMetric.TotalCPUMillis = totalCPU
				podMetric.TotalMemoryMi = totalMemory

				podMetrics = append(podMetrics, podMetric)
			}

			response.Success(c, podMetrics)
			return
		}
	}

	// 如果metrics API不可用，返回Pod的requests/limits
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	for _, pod := range pods.Items {
		podMetric := PodMetricInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Node:      pod.Spec.NodeName,
			Containers: make([]ContainerMetricInfo, 0),
		}

		var totalCPURequest, totalCPULimit, totalMemRequest, totalMemLimit int64
		for _, c := range pod.Spec.Containers {
			cpuReq := c.Resources.Requests.Cpu().MilliValue()
			cpuLim := c.Resources.Limits.Cpu().MilliValue()
			memReq := c.Resources.Requests.Memory().Value() / (1024 * 1024)
			memLim := c.Resources.Limits.Memory().Value() / (1024 * 1024)

			totalCPURequest += cpuReq
			totalCPULimit += cpuLim
			totalMemRequest += memReq
			totalMemLimit += memLim

			podMetric.Containers = append(podMetric.Containers, ContainerMetricInfo{
				Name:           c.Name,
				Image:          c.Image,
				CPURequestM:    cpuReq,
				CPULimitM:      cpuLim,
				MemoryRequestMi: memReq,
				MemoryLimitMi:  memLim,
			})
		}

		podMetric.CPURequestM = totalCPURequest
		podMetric.CPULimitM = totalCPULimit
		podMetric.MemoryRequestMi = totalMemRequest
		podMetric.MemoryLimitMi = totalMemLimit

		podMetrics = append(podMetrics, podMetric)
	}

	response.Success(c, podMetrics)
}

// GetDeploymentMetrics 获取Deployment资源使用概览
func (h *Handler) GetDeploymentMetrics(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取所有Deployments
	deployments, err := client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取所有ReplicaSets
	allReplicaSets, err := client.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取所有Pods的metrics
	var podMetricsMap map[string]*metricsv1beta1.PodMetrics
	if client.MetricsClient != nil {
		podMetrics, err := client.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			podMetricsMap = make(map[string]*metricsv1beta1.PodMetrics)
			for _, pm := range podMetrics.Items {
				podMetricsMap[pm.Name] = &pm
			}
		}
	}

	// 获取所有Pods
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 构建ReplicaSet名称到Deployment名称的映射
	rsToDeploy := make(map[string]string)
	for _, rs := range allReplicaSets.Items {
		for _, ownerRef := range rs.OwnerReferences {
			if ownerRef.Kind == "Deployment" {
				rsToDeploy[rs.Name] = ownerRef.Name
				break
			}
		}
	}

	// 构建Pod名称到ReplicaSet名称的映射
	podToRS := make(map[string]string)
	for _, pod := range pods.Items {
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" {
				podToRS[pod.Name] = ownerRef.Name
				break
			}
		}
	}

	// 构建Deployment指标
	result := make([]DeploymentMetricInfo, 0)
	for _, deploy := range deployments.Items {
		deployMetric := DeploymentMetricInfo{
			Name:      deploy.Name,
			Namespace: deploy.Namespace,
			Replicas:  int32(0),
			Ready:     deploy.Status.ReadyReplicas,
			Available: deploy.Status.AvailableReplicas,
		}
		if deploy.Spec.Replicas != nil {
			deployMetric.Replicas = *deploy.Spec.Replicas
		}

		// 计算该Deployment下所有Pod的资源使用
		var totalCPURequest, totalCPULimit, totalMemRequest, totalMemLimit int64
		var totalCPUUsage, totalMemUsage int64
		var podCount int

		for _, pod := range pods.Items {
			// 通过ReplicaSet检查Pod是否属于该Deployment
			rsName, ok := podToRS[pod.Name]
			if !ok {
				continue
			}
			deployName, ok := rsToDeploy[rsName]
			if !ok || deployName != deploy.Name {
				continue
			}

			podCount++

			// 计算requests/limits
			for _, c := range pod.Spec.Containers {
				totalCPURequest += c.Resources.Requests.Cpu().MilliValue()
				totalCPULimit += c.Resources.Limits.Cpu().MilliValue()
				totalMemRequest += c.Resources.Requests.Memory().Value() / (1024 * 1024)
				totalMemLimit += c.Resources.Limits.Memory().Value() / (1024 * 1024)
			}

			// 计算实际使用
			if podMetricsMap != nil {
				if pm, ok := podMetricsMap[pod.Name]; ok {
					for _, c := range pm.Containers {
						totalCPUUsage += c.Usage.Cpu().MilliValue()
						totalMemUsage += c.Usage.Memory().Value() / (1024 * 1024)
					}
				}
			}
		}

		deployMetric.PodCount = podCount
		deployMetric.CPURequestM = totalCPURequest
		deployMetric.CPULimitM = totalCPULimit
		deployMetric.MemoryRequestMi = totalMemRequest
		deployMetric.MemoryLimitMi = totalMemLimit
		deployMetric.CPUUsageM = totalCPUUsage
		deployMetric.MemoryUsageMi = totalMemUsage

		// 计算使用率
		if totalCPURequest > 0 {
			deployMetric.CPUUsagePercent = roundTo2(float64(totalCPUUsage) / float64(totalCPURequest) * 100)
		}
		if totalMemRequest > 0 {
			deployMetric.MemoryUsagePercent = roundTo2(float64(totalMemUsage) / float64(totalMemRequest) * 100)
		}

		result = append(result, deployMetric)
	}

	response.Success(c, result)
}

// GetNodeMetrics 获取节点资源使用指标
func (h *Handler) GetNodeMetrics(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取节点信息
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	result := make([]NodeMetricInfo, 0)
	for _, node := range nodes.Items {
		nodeMetric := NodeMetricInfo{
			Name: node.Name,
			IP:   getNodeIP(node.Status.Addresses),
		}

		// 获取节点容量
		cpuCapacity := node.Status.Capacity.Cpu().MilliValue()
		memCapacity := node.Status.Capacity.Memory().Value() / (1024 * 1024)
		podCapacity := node.Status.Capacity.Pods().Value()

		nodeMetric.CPUCapacityM = cpuCapacity
		nodeMetric.MemoryCapacityMi = memCapacity
		nodeMetric.PodCapacity = int(podCapacity)

		// 获取节点状态
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				nodeMetric.Ready = cond.Status == "True"
			}
		}

		// 计算已分配的资源
		pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
		})
		if err == nil {
			var cpuAllocated, memAllocated int64
			for _, pod := range pods.Items {
				for _, c := range pod.Spec.Containers {
					cpuAllocated += c.Resources.Requests.Cpu().MilliValue()
					memAllocated += c.Resources.Requests.Memory().Value() / (1024 * 1024)
				}
			}
			nodeMetric.CPUAllocatedM = cpuAllocated
			nodeMetric.MemoryAllocatedMi = memAllocated
			nodeMetric.PodCount = len(pods.Items)

			if cpuCapacity > 0 {
				nodeMetric.CPUAllocatedPercent = roundTo2(float64(cpuAllocated) / float64(cpuCapacity) * 100)
			}
			if memCapacity > 0 {
				nodeMetric.MemoryAllocatedPercent = roundTo2(float64(memAllocated) / float64(memCapacity) * 100)
			}
		}

		// 尝试获取实际使用
		if client.MetricsClient != nil {
			nodeMetrics, err := client.MetricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
			if err == nil {
				nodeMetric.CPUUsageM = nodeMetrics.Usage.Cpu().MilliValue()
				nodeMetric.MemoryUsageMi = nodeMetrics.Usage.Memory().Value() / (1024 * 1024)
				if cpuCapacity > 0 {
					nodeMetric.CPUUsagePercent = roundTo2(float64(nodeMetric.CPUUsageM) / float64(cpuCapacity) * 100)
				}
				if memCapacity > 0 {
					nodeMetric.MemoryUsagePercent = roundTo2(float64(nodeMetric.MemoryUsageMi) / float64(memCapacity) * 100)
				}
			}
		}

		result = append(result, nodeMetric)
	}

	response.Success(c, result)
}

// GetClusterOverview 获取集群资源概览
func (h *Handler) GetClusterOverview(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取节点信息
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取所有Pods
	pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取Deployments
	deployments, err := client.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 计算集群资源
	var totalCPUCapacity, totalMemCapacity int64
	var totalCPUAllocated, totalMemAllocated int64
	var totalCPUUsage, totalMemUsage int64

	for _, node := range nodes.Items {
		totalCPUCapacity += node.Status.Capacity.Cpu().MilliValue()
		totalMemCapacity += node.Status.Capacity.Memory().Value() / (1024 * 1024)
	}

	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			totalCPUAllocated += c.Resources.Requests.Cpu().MilliValue()
			totalMemAllocated += c.Resources.Requests.Memory().Value() / (1024 * 1024)
		}
	}

	// 尝试获取实际使用
	if client.MetricsClient != nil {
		nodeMetrics, err := client.MetricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, nm := range nodeMetrics.Items {
				totalCPUUsage += nm.Usage.Cpu().MilliValue()
				totalMemUsage += nm.Usage.Memory().Value() / (1024 * 1024)
			}
		}
	}

	// 统计Pod状态
	var running, pending, succeeded, failed int
	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case "Running":
			running++
		case "Pending":
			pending++
		case "Succeeded":
			succeeded++
		case "Failed":
			failed++
		}
	}

	overview := ClusterOverview{
		NodeCount:        len(nodes.Items),
		DeploymentCount:  len(deployments.Items),
		PodCount:         len(pods.Items),
		PodRunning:       running,
		PodPending:       pending,
		PodSucceeded:     succeeded,
		PodFailed:        failed,
		CPUCapacityM:     totalCPUCapacity,
		MemoryCapacityMi: totalMemCapacity,
		CPUAllocatedM:    totalCPUAllocated,
		MemoryAllocatedMi: totalMemAllocated,
		CPUUsageM:        totalCPUUsage,
		MemoryUsageMi:    totalMemUsage,
	}

	if totalCPUCapacity > 0 {
		overview.CPUAllocatedPercent = roundTo2(float64(totalCPUAllocated) / float64(totalCPUCapacity) * 100)
		overview.CPUUsagePercent = roundTo2(float64(totalCPUUsage) / float64(totalCPUCapacity) * 100)
	}
	if totalMemCapacity > 0 {
		overview.MemoryAllocatedPercent = roundTo2(float64(totalMemAllocated) / float64(totalMemCapacity) * 100)
		overview.MemoryUsagePercent = roundTo2(float64(totalMemUsage) / float64(totalMemCapacity) * 100)
	}

	response.Success(c, overview)
}

// 辅助函数
func getNodeIP(addresses []corev1.NodeAddress) string {
	for _, addr := range addresses {
		if addr.Type == "InternalIP" {
			return addr.Address
		}
	}
	return ""
}

// 数据结构定义
type PodMetricInfo struct {
	Name            string               `json:"name"`
	Namespace       string               `json:"namespace"`
	Status          string               `json:"status,omitempty"`
	Node            string               `json:"node,omitempty"`
	TotalCPUMillis  int64                `json:"total_cpu_millis,omitempty"`
	TotalMemoryMi   int64                `json:"total_memory_mi,omitempty"`
	CPURequestM     int64                `json:"cpu_request_m,omitempty"`
	CPULimitM       int64                `json:"cpu_limit_m,omitempty"`
	MemoryRequestMi int64                `json:"memory_request_mi,omitempty"`
	MemoryLimitMi   int64                `json:"memory_limit_mi,omitempty"`
	Containers      []ContainerMetricInfo `json:"containers"`
}

type ContainerMetricInfo struct {
	Name            string `json:"name"`
	Image           string `json:"image,omitempty"`
	CPUMillis       int64  `json:"cpu_millis,omitempty"`
	MemoryMi        int64  `json:"memory_mi,omitempty"`
	CPURequestM     int64  `json:"cpu_request_m,omitempty"`
	CPULimitM       int64  `json:"cpu_limit_m,omitempty"`
	MemoryRequestMi int64  `json:"memory_request_mi,omitempty"`
	MemoryLimitMi   int64  `json:"memory_limit_mi,omitempty"`
}

type DeploymentMetricInfo struct {
	Name                 string  `json:"name"`
	Namespace            string  `json:"namespace"`
	Replicas             int32   `json:"replicas"`
	Ready                int32   `json:"ready"`
	Available            int32   `json:"available"`
	PodCount             int     `json:"pod_count"`
	CPURequestM          int64   `json:"cpu_request_m"`
	CPULimitM            int64   `json:"cpu_limit_m"`
	MemoryRequestMi      int64   `json:"memory_request_mi"`
	MemoryLimitMi        int64   `json:"memory_limit_mi"`
	CPUUsageM            int64   `json:"cpu_usage_m"`
	MemoryUsageMi        int64   `json:"memory_usage_mi"`
	CPUUsagePercent      float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent   float64 `json:"memory_usage_percent"`
}

type NodeMetricInfo struct {
	Name                    string  `json:"name"`
	IP                      string  `json:"ip"`
	Ready                   bool    `json:"ready"`
	CPUCapacityM            int64   `json:"cpu_capacity_m"`
	MemoryCapacityMi        int64   `json:"memory_capacity_mi"`
	PodCapacity             int     `json:"pod_capacity"`
	PodCount                int     `json:"pod_count"`
	CPUAllocatedM           int64   `json:"cpu_allocated_m"`
	MemoryAllocatedMi       int64   `json:"memory_allocated_mi"`
	CPUAllocatedPercent     float64 `json:"cpu_allocated_percent"`
	MemoryAllocatedPercent  float64 `json:"memory_allocated_percent"`
	CPUUsageM               int64   `json:"cpu_usage_m"`
	MemoryUsageMi           int64   `json:"memory_usage_mi"`
	CPUUsagePercent         float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent      float64 `json:"memory_usage_percent"`
}

type ClusterOverview struct {
	NodeCount              int     `json:"node_count"`
	DeploymentCount        int     `json:"deployment_count"`
	PodCount               int     `json:"pod_count"`
	PodRunning             int     `json:"pod_running"`
	PodPending             int     `json:"pod_pending"`
	PodSucceeded           int     `json:"pod_succeeded"`
	PodFailed              int     `json:"pod_failed"`
	CPUCapacityM           int64   `json:"cpu_capacity_m"`
	MemoryCapacityMi       int64   `json:"memory_capacity_mi"`
	CPUAllocatedM          int64   `json:"cpu_allocated_m"`
	MemoryAllocatedMi      int64   `json:"memory_allocated_mi"`
	CPUAllocatedPercent    float64 `json:"cpu_allocated_percent"`
	MemoryAllocatedPercent float64 `json:"memory_allocated_percent"`
	CPUUsageM              int64   `json:"cpu_usage_m"`
	MemoryUsageMi          int64   `json:"memory_usage_mi"`
	CPUUsagePercent        float64 `json:"cpu_usage_percent"`
	MemoryUsagePercent     float64 `json:"memory_usage_percent"`
}
