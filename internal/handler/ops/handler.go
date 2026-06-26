package ops

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Handler 运维工具处理器
type Handler struct{}

// NewHandler 创建运维工具处理器
func NewHandler() *Handler {
	return &Handler{}
}

// ==================== P0: Pod 诊断面板 ====================

// PodDiagnosis Pod 诊断结果
type PodDiagnosis struct {
	PodName      string                 `json:"pod_name"`
	Namespace    string                 `json:"namespace"`
	Status       string                 `json:"status"`
	Node         string                 `json:"node"`
	IP           string                 `json:"ip"`
	Restarts     int32                  `json:"restarts"`
	Age          string                 `json:"age"`
	Containers   []ContainerDiagnosis   `json:"containers"`
	Events       []EventInfo            `json:"events"`
	Conditions   []ConditionInfo        `json:"conditions"`
	ResourceUsage ResourceUsage         `json:"resource_usage"`
	Labels       map[string]string      `json:"labels"`
	Annotations  map[string]string      `json:"annotations"`
	OwnerRef     string                 `json:"owner_ref"`
	QoSClass     string                 `json:"qos_class"`
	NodeSelector map[string]string      `json:"node_selector"`
	Tolerations  []corev1.Toleration    `json:"tolerations"`
	Volumes      []string               `json:"volumes"`
	Problems     []string               `json:"problems"`
	Suggestions  []string               `json:"suggestions"`
}

type ContainerDiagnosis struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	ExitCode     int32  `json:"exit_code"`
	Reason       string `json:"reason"`
}

type EventInfo struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int32  `json:"count"`
	FirstTime string `json:"first_time"`
	LastTime  string `json:"last_time"`
}

type ConditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ResourceUsage struct {
	CPURequest    string `json:"cpu_request"`
	CPULimit      string `json:"cpu_limit"`
	MemRequest    string `json:"mem_request"`
	MemLimit      string `json:"mem_limit"`
	PodScheduled  bool   `json:"pod_scheduled"`
	Ready         bool   `json:"ready"`
	Initialized   bool   `json:"initialized"`
	ContainersReady bool `json:"containers_ready"`
}

// DiagnosePod 诊断 Pod
func (h *Handler) DiagnosePod(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pod not found")
		return
	}

	diagnosis := buildPodDiagnosis(ctx, client.Clientset, pod)
	response.Success(c, diagnosis)
}

func buildPodDiagnosis(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod) *PodDiagnosis {
	d := &PodDiagnosis{
		PodName:     pod.Name,
		Namespace:   pod.Namespace,
		Status:      string(pod.Status.Phase),
		Node:        pod.Spec.NodeName,
		IP:          pod.Status.PodIP,
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
		QoSClass:    string(pod.Status.QOSClass),
	}

	// 计算重启次数
	var totalRestarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		totalRestarts += cs.RestartCount
	}
	d.Restarts = totalRestarts

	// 计算年龄
	d.Age = timeSince(pod.CreationTimestamp.Time)

	// 容器诊断
	for _, cs := range pod.Status.ContainerStatuses {
		cd := ContainerDiagnosis{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		if cs.State.Running != nil {
			cd.State = "Running"
		} else if cs.State.Waiting != nil {
			cd.State = "Waiting"
			cd.Reason = cs.State.Waiting.Reason
		} else if cs.State.Terminated != nil {
			cd.State = "Terminated"
			cd.ExitCode = cs.State.Terminated.ExitCode
			cd.Reason = cs.State.Terminated.Reason
		}
		// 获取镜像
		for _, c := range pod.Spec.Containers {
			if c.Name == cs.Name {
				cd.Image = c.Image
				break
			}
		}
		d.Containers = append(d.Containers, cd)
	}

	// 条件
	for _, cond := range pod.Status.Conditions {
		d.Conditions = append(d.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// 资源使用
	var cpuReq, cpuLim, memReq, memLim string
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests != nil {
			if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuReq = cpu.String()
			}
			if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq = mem.String()
			}
		}
		if c.Resources.Limits != nil {
			if cpu, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				cpuLim = cpu.String()
			}
			if mem, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				memLim = mem.String()
			}
		}
	}
	d.ResourceUsage = ResourceUsage{
		CPURequest: cpuReq,
		CPULimit:   cpuLim,
		MemRequest: memReq,
		MemLimit:   memLim,
	}

	// 条件状态
	for _, cond := range pod.Status.Conditions {
		switch cond.Type {
		case corev1.PodScheduled:
			d.ResourceUsage.PodScheduled = cond.Status == corev1.ConditionTrue
		case corev1.PodReady:
			d.ResourceUsage.Ready = cond.Status == corev1.ConditionTrue
		case corev1.PodInitialized:
			d.ResourceUsage.Initialized = cond.Status == corev1.ConditionTrue
		case corev1.ContainersReady:
			d.ResourceUsage.ContainersReady = cond.Status == corev1.ConditionTrue
		}
	}

	// Owner Reference
	for _, ref := range pod.OwnerReferences {
		d.OwnerRef = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
	}

	// Volume 名称
	for _, v := range pod.Spec.Volumes {
		d.Volumes = append(d.Volumes, v.Name)
	}

	// 获取事件
	events, _ := clientset.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", pod.Name, pod.Namespace),
	})
	if events != nil {
		sort.Slice(events.Items, func(i, j int) bool {
			return events.Items[i].LastTimestamp.After(events.Items[j].LastTimestamp.Time)
		})
		for i, e := range events.Items {
			if i >= 20 {
				break
			}
			firstTime := ""
			if !e.FirstTimestamp.IsZero() {
				firstTime = e.FirstTimestamp.Format("2006-01-02 15:04:05")
			}
			lastTime := ""
			if !e.LastTimestamp.IsZero() {
				lastTime = e.LastTimestamp.Time.Format("2006-01-02 15:04:05")
			}
			d.Events = append(d.Events, EventInfo{
				Type:      e.Type,
				Reason:    e.Reason,
				Message:   e.Message,
				Count:     e.Count,
				FirstTime: firstTime,
				LastTime:  lastTime,
			})
		}
	}

	// 自动检测问题
	d.Problems = detectPodProblems(pod, d)
	d.Suggestions = generateSuggestions(d)

	return d
}

func detectPodProblems(pod *corev1.Pod, d *PodDiagnosis) []string {
	var problems []string

	// 检查重启次数
	if d.Restarts > 5 {
		problems = append(problems, fmt.Sprintf("容器频繁重启 (%d 次)，可能存在 CrashLoopBackOff", d.Restarts))
	}

	// 检查容器状态
	for _, cs := range d.Containers {
		if cs.State == "Waiting" {
			switch cs.Reason {
			case "CrashLoopBackOff":
				problems = append(problems, fmt.Sprintf("容器 %s 处于 CrashLoopBackOff 状态", cs.Name))
			case "ImagePullBackOff", "ErrImagePull":
				problems = append(problems, fmt.Sprintf("容器 %s 镜像拉取失败: %s", cs.Name, cs.Image))
			case "CreateContainerConfigError":
				problems = append(problems, fmt.Sprintf("容器 %s 配置错误", cs.Name))
			}
		}
		if cs.State == "Terminated" && cs.ExitCode != 0 {
			problems = append(problems, fmt.Sprintf("容器 %s 异常退出，退出码: %d，原因: %s", cs.Name, cs.ExitCode, cs.Reason))
		}
	}

	// 检查 Pod 状态
	if pod.Status.Phase == "Pending" {
		problems = append(problems, "Pod 处于 Pending 状态，可能资源不足或调度失败")
	}
	if pod.Status.Phase == "Failed" {
		problems = append(problems, "Pod 处于 Failed 状态")
	}

	// 检查事件中的 Warning
	for _, e := range d.Events {
		if e.Type == "Warning" {
			problems = append(problems, fmt.Sprintf("事件警告 [%s]: %s", e.Reason, e.Message))
		}
	}

	return problems
}

func generateSuggestions(d *PodDiagnosis) []string {
	var suggestions []string

	for _, p := range d.Problems {
		if contains(p, "CrashLoopBackOff") {
			suggestions = append(suggestions, "检查容器日志: kubectl logs <pod> --previous")
			suggestions = append(suggestions, "检查应用启动命令和参数是否正确")
		}
		if contains(p, "ImagePullBackOff") || contains(p, "镜像拉取失败") {
			suggestions = append(suggestions, "检查镜像名称和标签是否正确")
			suggestions = append(suggestions, "检查镜像仓库认证: kubectl get secrets")
		}
		if contains(p, "Pending") {
			suggestions = append(suggestions, "检查集群资源是否充足: kubectl describe nodes")
			suggestions = append(suggestions, "检查是否有合适的节点满足调度条件")
		}
		if contains(p, "频繁重启") {
			suggestions = append(suggestions, "检查应用日志找出崩溃原因")
			suggestions = append(suggestions, "增加资源限制或优化应用内存使用")
		}
	}

	if d.ResourceUsage.CPURequest == "" {
		suggestions = append(suggestions, "建议设置 CPU/内存请求和限制")
	}

	return suggestions
}

// ==================== P0: 资源使用趋势 ====================

// ResourceMetrics 资源指标
type ResourceMetrics struct {
	Timestamp    time.Time `json:"timestamp"`
	CPUUsage     string    `json:"cpu_usage"`
	MemoryUsage  string    `json:"memory_usage"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemPercent   float64   `json:"mem_percent"`
}

// GetResourceTrend 获取资源使用趋势
func (h *Handler) GetResourceTrend(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")
	resourceType := c.DefaultQuery("type", "pod")
	name := c.Query("name")

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取当前指标（模拟趋势数据，实际应从 Prometheus 获取）
	var metrics []ResourceMetrics
	now := time.Now()

	switch resourceType {
	case "node":
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "node not found")
			return
		}
		// 获取节点上的 Pod 使用情况
		pods, _ := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", name),
		})
		var totalCPU, totalMem int64
		if pods != nil {
			for _, pod := range pods.Items {
				for _, c := range pod.Spec.Containers {
					if c.Resources.Requests != nil {
						if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
							totalCPU += cpu.MilliValue()
						}
						if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
							totalMem += mem.Value()
						}
					}
				}
			}
		}
		cpuCapacity := node.Status.Capacity.Cpu().MilliValue()
		memCapacity := node.Status.Capacity.Memory().Value()

		// 生成模拟趋势数据
		for i := 23; i >= 0; i-- {
			t := now.Add(-time.Duration(i) * time.Hour)
			cpuPct := float64(totalCPU) / float64(cpuCapacity) * 100
			memPct := float64(totalMem) / float64(memCapacity) * 100
			metrics = append(metrics, ResourceMetrics{
				Timestamp:   t,
				CPUUsage:    fmt.Sprintf("%dm", totalCPU),
				MemoryUsage: fmt.Sprintf("%dMi", totalMem/1024/1024),
				CPUPercent:  cpuPct,
				MemPercent:  memPct,
			})
		}

	case "pod":
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "pod not found")
			return
		}
		var cpuReq, memReq int64
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					cpuReq += cpu.MilliValue()
				}
				if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					memReq += mem.Value()
				}
			}
		}
		for i := 23; i >= 0; i-- {
			t := now.Add(-time.Duration(i) * time.Hour)
			metrics = append(metrics, ResourceMetrics{
				Timestamp:   t,
				CPUUsage:    fmt.Sprintf("%dm", cpuReq),
				MemoryUsage: fmt.Sprintf("%dMi", memReq/1024/1024),
				CPUPercent:  float64(cpuReq) / 1000 * 100,
				MemPercent:  float64(memReq) / 1024 / 1024 / 100,
			})
		}
	}

	response.Success(c, gin.H{
		"resource_type": resourceType,
		"name":          name,
		"metrics":       metrics,
	})
}

// ==================== P0: 事件时间线 ====================

// TimelineEvent 时间线事件
type TimelineEvent struct {
	Time         string `json:"time"`
	Type         string `json:"type"`
	Reason       string `json:"reason"`
	Message      string `json:"message"`
	Namespace    string `json:"namespace"`
	ResourceKind string `json:"resource_kind"`
	ResourceName string `json:"resource_name"`
	Count        int32  `json:"count"`
}

// GetEventTimeline 获取事件时间线
func (h *Handler) GetEventTimeline(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")
	hours := c.DefaultQuery("hours", "24")

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	events, err := client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 过滤时间范围
	var timeline []TimelineEvent
	hoursInt := 24
	fmt.Sscanf(hours, "%d", &hoursInt)
	cutoff := time.Now().Add(-time.Duration(hoursInt) * time.Hour)

	for _, e := range events.Items {
		if e.LastTimestamp.Time.Before(cutoff) && !e.LastTimestamp.IsZero() {
			continue
		}
		timeline = append(timeline, TimelineEvent{
			Time:         e.LastTimestamp.Time.Format("2006-01-02 15:04:05"),
			Type:         e.Type,
			Reason:       e.Reason,
			Message:      e.Message,
			Namespace:    e.Namespace,
			ResourceKind: e.InvolvedObject.Kind,
			ResourceName: e.InvolvedObject.Name,
			Count:        e.Count,
		})
	}

	// 按时间排序（最新在前）
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Time > timeline[j].Time
	})

	// 统计
	var warningCount, normalCount int
	for _, e := range timeline {
		if e.Type == "Warning" {
			warningCount++
		} else {
			normalCount++
		}
	}

	response.Success(c, gin.H{
		"total":         len(timeline),
		"warning_count": warningCount,
		"normal_count":  normalCount,
		"events":        timeline,
	})
}

// ==================== P0: 节点压力可视化 ====================

// NodePressure 节点压力信息
type NodePressure struct {
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	CPUCapacity  string            `json:"cpu_capacity"`
	CPUAllocated string            `json:"cpu_allocated"`
	CPUPercent   float64           `json:"cpu_percent"`
	MemCapacity  string            `json:"mem_capacity"`
	MemAllocated string            `json:"mem_allocated"`
	MemPercent   float64           `json:"mem_percent"`
	PodCapacity  int64             `json:"pod_capacity"`
	PodCount     int               `json:"pod_count"`
	PodPercent   float64           `json:"pod_percent"`
	Conditions   []ConditionInfo   `json:"conditions"`
	Taints       []corev1.Taint    `json:"taints"`
	PressureLevel string           `json:"pressure_level"` // low, medium, high, critical
}

// GetNodePressure 获取节点压力
func (h *Handler) GetNodePressure(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取所有 Pod
	pods, _ := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// 统计每个节点的资源使用
	nodePods := make(map[string][]corev1.Pod)
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Spec.NodeName != "" {
				nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)
			}
		}
	}

	var result []NodePressure
	for _, node := range nodes.Items {
		np := NodePressure{
			Name:   node.Name,
			Taints: node.Spec.Taints,
		}

		// 状态
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if cond.Status == corev1.ConditionTrue {
					np.Status = "Ready"
				} else {
					np.Status = "NotReady"
				}
			}
			np.Conditions = append(np.Conditions, ConditionInfo{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			})
		}

		// CPU
		cpuCapacity := node.Status.Capacity.Cpu().MilliValue()
		np.CPUCapacity = node.Status.Capacity.Cpu().String()

		// 内存
		memCapacity := node.Status.Capacity.Memory().Value()
		np.MemCapacity = fmt.Sprintf("%dGi", memCapacity/1024/1024/1024)

		// Pod 容量
		np.PodCapacity = node.Status.Capacity.Pods().Value()

		// 计算已分配资源
		var cpuAllocated, memAllocated int64
		podCount := 0
		for _, pod := range nodePods[node.Name] {
			podCount++
			for _, c := range pod.Spec.Containers {
				if c.Resources.Requests != nil {
					if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						cpuAllocated += cpu.MilliValue()
					}
					if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						memAllocated += mem.Value()
					}
				}
			}
		}

		np.CPUAllocated = fmt.Sprintf("%dm", cpuAllocated)
		np.MemAllocated = fmt.Sprintf("%dMi", memAllocated/1024/1024)
		np.PodCount = podCount

		// 计算百分比
		if cpuCapacity > 0 {
			np.CPUPercent = float64(cpuAllocated) / float64(cpuCapacity) * 100
		}
		if memCapacity > 0 {
			np.MemPercent = float64(memAllocated) / float64(memCapacity) * 100
		}
		if np.PodCapacity > 0 {
			np.PodPercent = float64(podCount) / float64(np.PodCapacity) * 100
		}

		// 压力等级
		maxPercent := np.CPUPercent
		if np.MemPercent > maxPercent {
			maxPercent = np.MemPercent
		}
		if np.PodPercent > maxPercent {
			maxPercent = np.PodPercent
		}
		switch {
		case maxPercent >= 90:
			np.PressureLevel = "critical"
		case maxPercent >= 75:
			np.PressureLevel = "high"
		case maxPercent >= 50:
			np.PressureLevel = "medium"
		default:
			np.PressureLevel = "low"
		}

		result = append(result, np)
	}

	response.Success(c, result)
}

// ==================== P0: 一键回滚 ====================

// RollbackDeployment 一键回滚 Deployment
func (h *Handler) RollbackDeployment(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Revision int64 `json:"revision"` // 0 表示回滚到上一个版本
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Revision = 0
	}

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取 Deployment
	deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 获取 ReplicaSet 列表
	rsList, err := client.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(deploy.Spec.Selector),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 找到上一个版本的 ReplicaSet
	var targetRS *appsv1.ReplicaSet
	var currentRevision int64

	for _, rs := range rsList.Items {
		for _, ref := range rs.OwnerReferences {
			if ref.Name == name {
				revision := rs.Annotations["deployment.kubernetes.io/revision"]
				var rev int64
				fmt.Sscanf(revision, "%d", &rev)
				if req.Revision > 0 {
					if rev == req.Revision {
						targetRS = &rs
					}
				} else {
					if rev > currentRevision {
						currentRevision = rev
					}
				}
			}
		}
	}

	// 如果没有指定版本，找上一个版本
	if req.Revision == 0 {
		for _, rs := range rsList.Items {
			for _, ref := range rs.OwnerReferences {
				if ref.Name == name {
					revision := rs.Annotations["deployment.kubernetes.io/revision"]
					var rev int64
					fmt.Sscanf(revision, "%d", &rev)
					if rev == currentRevision-1 {
						targetRS = &rs
					}
				}
			}
		}
	}

	if targetRS == nil {
		response.BadRequest(c, "no previous revision found to rollback")
		return
	}

	// 使用 kubectl rollout undo 方式：更新 Deployment 的 PodTemplateSpec 为目标 ReplicaSet 的模板
	deploy.Spec.Template = targetRS.Spec.Template

	_, err = client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, "rollback failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"message":       fmt.Sprintf("Deployment %s rollback initiated", name),
		"target_revision": targetRS.Annotations["deployment.kubernetes.io/revision"],
	})
}

// ==================== P1: 资源依赖图 ====================

// ResourceNode 资源节点
type ResourceNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Namespace string `json:"namespace"`
	Status   string `json:"status"`
}

// ResourceEdge 资源边
type ResourceEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // owns, selects, mounts
}

// ResourceGraph 资源图
type ResourceGraph struct {
	Nodes []ResourceNode `json:"nodes"`
	Edges []ResourceEdge `json:"edges"`
}

// GetResourceGraph 获取资源依赖图
func (h *Handler) GetResourceGraph(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.DefaultQuery("ns", "")

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	graph := ResourceGraph{}

	// 获取 Deployments
	deploys, _ := client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if deploys != nil {
		for _, d := range deploys.Items {
			nodeID := fmt.Sprintf("Deployment/%s/%s", d.Namespace, d.Name)
			graph.Nodes = append(graph.Nodes, ResourceNode{
				ID: nodeID, Kind: "Deployment", Name: d.Name, Namespace: d.Namespace,
				Status: fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, int32Value(d.Spec.Replicas)),
			})

			// 获取关联的 ReplicaSet
			rsList, _ := client.Clientset.AppsV1().ReplicaSets(d.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(d.Spec.Selector),
			})
			if rsList != nil {
				for _, rs := range rsList.Items {
					for _, ref := range rs.OwnerReferences {
						if ref.Name == d.Name {
							rsID := fmt.Sprintf("ReplicaSet/%s/%s", rs.Namespace, rs.Name)
							graph.Nodes = append(graph.Nodes, ResourceNode{
								ID: rsID, Kind: "ReplicaSet", Name: rs.Name, Namespace: rs.Namespace,
								Status: fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, int32Value(rs.Spec.Replicas)),
							})
							graph.Edges = append(graph.Edges, ResourceEdge{Source: nodeID, Target: rsID, Type: "owns"})

							// 获取关联的 Pod
							podList, _ := client.Clientset.CoreV1().Pods(rs.Namespace).List(ctx, metav1.ListOptions{
								LabelSelector: metav1.FormatLabelSelector(rs.Spec.Selector),
							})
							if podList != nil {
								for _, pod := range podList.Items {
									for _, ref := range pod.OwnerReferences {
										if ref.Name == rs.Name {
											podID := fmt.Sprintf("Pod/%s/%s", pod.Namespace, pod.Name)
											graph.Nodes = append(graph.Nodes, ResourceNode{
												ID: podID, Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace,
												Status: string(pod.Status.Phase),
											})
											graph.Edges = append(graph.Edges, ResourceEdge{Source: rsID, Target: podID, Type: "owns"})
										}
									}
								}
							}
						}
					}
				}
			}

			// 获取关联的 Service
			svcs, _ := client.Clientset.CoreV1().Services(d.Namespace).List(ctx, metav1.ListOptions{})
			if svcs != nil {
				for _, svc := range svcs.Items {
					// 检查 selector 是否匹配
					matches := true
					for k, v := range svc.Spec.Selector {
						if d.Spec.Template.Labels[k] != v {
							matches = false
							break
						}
					}
					if matches && len(svc.Spec.Selector) > 0 {
						svcID := fmt.Sprintf("Service/%s/%s", svc.Namespace, svc.Name)
						graph.Nodes = append(graph.Nodes, ResourceNode{
							ID: svcID, Kind: "Service", Name: svc.Name, Namespace: svc.Namespace,
							Status: string(svc.Spec.Type),
						})
						graph.Edges = append(graph.Edges, ResourceEdge{Source: svcID, Target: nodeID, Type: "selects"})
					}
				}
			}
		}
	}

	// 去重
	graph.Nodes = dedupNodes(graph.Nodes)
	graph.Edges = dedupEdges(graph.Edges)

	response.Success(c, graph)
}

// ==================== P1: RBAC 可视化 ====================

// RBACInfo RBAC 信息
type RBACInfo struct {
	Roles               []RoleInfo               `json:"roles"`
	ClusterRoles        []RoleInfo               `json:"cluster_roles"`
	RoleBindings        []BindingInfo            `json:"role_bindings"`
	ClusterRoleBindings []BindingInfo            `json:"cluster_role_bindings"`
	UserPermissions     map[string][]string      `json:"user_permissions"`
}

type RoleInfo struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Rules     []RuleInfo `json:"rules"`
}

type RuleInfo struct {
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
	APIGroups []string `json:"api_groups"`
}

type BindingInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Role      string `json:"role"`
	Subjects  []SubjectInfo `json:"subjects"`
}

type SubjectInfo struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// GetRBACVisualization 获取 RBAC 可视化数据
func (h *Handler) GetRBACVisualization(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	info := RBACInfo{
		UserPermissions: make(map[string][]string),
	}

	// ClusterRoles
	crs, _ := client.Clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if crs != nil {
		for _, cr := range crs.Items {
			ri := RoleInfo{Name: cr.Name}
			for _, rule := range cr.Rules {
				ri.Rules = append(ri.Rules, RuleInfo{
					Resources: rule.Resources,
					Verbs:     rule.Verbs,
					APIGroups: rule.APIGroups,
				})
			}
			info.ClusterRoles = append(info.ClusterRoles, ri)
		}
	}

	// ClusterRoleBindings
	crbs, _ := client.Clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if crbs != nil {
		for _, crb := range crbs.Items {
			bi := BindingInfo{
				Name: crb.Name,
				Role: crb.RoleRef.Name,
			}
			for _, s := range crb.Subjects {
				bi.Subjects = append(bi.Subjects, SubjectInfo{
					Kind:      s.Kind,
					Name:      s.Name,
					Namespace: s.Namespace,
				})
				// 记录用户权限
				key := fmt.Sprintf("%s/%s", s.Kind, s.Name)
				info.UserPermissions[key] = append(info.UserPermissions[key], crb.RoleRef.Name)
			}
			info.ClusterRoleBindings = append(info.ClusterRoleBindings, bi)
		}
	}

	// Roles (namespaced)
	roles, _ := client.Clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	if roles != nil {
		for _, role := range roles.Items {
			ri := RoleInfo{Name: role.Name, Namespace: role.Namespace}
			for _, rule := range role.Rules {
				ri.Rules = append(ri.Rules, RuleInfo{
					Resources: rule.Resources,
					Verbs:     rule.Verbs,
					APIGroups: rule.APIGroups,
				})
			}
			info.Roles = append(info.Roles, ri)
		}
	}

	// RoleBindings
	rbs, _ := client.Clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if rbs != nil {
		for _, rb := range rbs.Items {
			bi := BindingInfo{
				Name:      rb.Name,
				Namespace: rb.Namespace,
				Role:      rb.RoleRef.Name,
			}
			for _, s := range rb.Subjects {
				bi.Subjects = append(bi.Subjects, SubjectInfo{
					Kind:      s.Kind,
					Name:      s.Name,
					Namespace: s.Namespace,
				})
			}
			info.RoleBindings = append(info.RoleBindings, bi)
		}
	}

	response.Success(c, info)
}

// ==================== P1: 闲置资源清理 ====================

// IdleResource 闲置资源
type IdleResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
	Deleted   bool   `json:"deleted"`
}

// FindIdleResources 查找闲置资源
func (h *Handler) FindIdleResources(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var idleResources []IdleResource

	// 1. 查找 Completed/Failed 的 Job（超过 24 小时）
	jobs, _ := client.Clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if jobs != nil {
		for _, job := range jobs.Items {
			if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
				completionTime := job.Status.CompletionTime
				if completionTime != nil && time.Since(completionTime.Time) > 24*time.Hour {
					idleResources = append(idleResources, IdleResource{
						Kind:      "Job",
						Name:      job.Name,
						Namespace: job.Namespace,
						Age:       timeSince(job.CreationTimestamp.Time),
						Reason:    fmt.Sprintf("已完成 %d 小时前", int(time.Since(completionTime.Time).Hours())),
					})
				}
			}
		}
	}

	// 2. 查找空的 ConfigMap（没有被任何 Pod 引用）
	cms, _ := client.Clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	pods, _ := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	usedCMs := make(map[string]bool)
	if pods != nil {
		for _, pod := range pods.Items {
			for _, vol := range pod.Spec.Volumes {
				if vol.ConfigMap != nil {
					usedCMs[fmt.Sprintf("%s/%s", pod.Namespace, vol.ConfigMap.Name)] = true
				}
			}
			for _, c := range pod.Spec.Containers {
				for _, env := range c.EnvFrom {
					if env.ConfigMapRef != nil {
						usedCMs[fmt.Sprintf("%s/%s", pod.Namespace, env.ConfigMapRef.Name)] = true
					}
				}
			}
		}
	}
	if cms != nil {
		for _, cm := range cms.Items {
			key := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
			if !usedCMs[key] && cm.Namespace != "kube-system" && cm.Namespace != "kube-public" {
				idleResources = append(idleResources, IdleResource{
					Kind:      "ConfigMap",
					Name:      cm.Name,
					Namespace: cm.Namespace,
					Age:       timeSince(cm.CreationTimestamp.Time),
					Reason:    "未被任何 Pod 引用",
				})
			}
		}
	}

	// 3. 查找未绑定的 PVC（超过 7 天）
	pvcs, _ := client.Clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if pvcs != nil {
		for _, pvc := range pvcs.Items {
			if pvc.Status.Phase == corev1.ClaimPending && time.Since(pvc.CreationTimestamp.Time) > 7*24*time.Hour {
				idleResources = append(idleResources, IdleResource{
					Kind:      "PVC",
					Name:      pvc.Name,
					Namespace: pvc.Namespace,
					Age:       timeSince(pvc.CreationTimestamp.Time),
					Reason:    "处于 Pending 状态超过 7 天",
				})
			}
		}
	}

	// 4. 查找没有后端的 Service
	svcs, _ := client.Clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if svcs != nil {
		for _, svc := range svcs.Items {
			if svc.Namespace == "kube-system" || svc.Namespace == "kubernetes" {
				continue
			}
			if svc.Spec.ClusterIP == "None" {
				continue // Headless Service
			}
			// 检查是否有 Endpoints
			ep, _ := client.Clientset.CoreV1().Endpoints(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if ep != nil && len(ep.Subsets) == 0 {
				idleResources = append(idleResources, IdleResource{
					Kind:      "Service",
					Name:      svc.Name,
					Namespace: svc.Namespace,
					Age:       timeSince(svc.CreationTimestamp.Time),
					Reason:    "没有后端 Endpoints",
				})
			}
		}
	}

	// 5. 查找已完成的 Pod（Succeeded 超过 1 小时）
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodSucceeded && time.Since(pod.Status.StartTime.Time) > 1*time.Hour {
				idleResources = append(idleResources, IdleResource{
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Age:       timeSince(pod.CreationTimestamp.Time),
					Reason:    "已完成超过 1 小时",
				})
			}
		}
	}

	response.Success(c, gin.H{
		"total":   len(idleResources),
		"resources": idleResources,
	})
}

// CleanIdleResource 清理指定的闲置资源
func (h *Handler) CleanIdleResource(c *gin.Context) {
	clusterID, err := parseClusterID(c)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Resources []struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"resources"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var results []map[string]interface{}

	for _, res := range req.Resources {
		result := map[string]interface{}{
			"kind": res.Kind,
			"name": res.Name,
			"ns":   res.Namespace,
		}
		var delErr error
		switch res.Kind {
		case "Job":
			delErr = client.Clientset.BatchV1().Jobs(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{})
		case "ConfigMap":
			delErr = client.Clientset.CoreV1().ConfigMaps(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{})
		case "PVC":
			delErr = client.Clientset.CoreV1().PersistentVolumeClaims(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{})
		case "Service":
			delErr = client.Clientset.CoreV1().Services(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{})
		case "Pod":
			delErr = client.Clientset.CoreV1().Pods(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{})
		default:
			delErr = fmt.Errorf("unsupported resource kind: %s", res.Kind)
		}
		if delErr != nil {
			result["status"] = "failed"
			result["error"] = delErr.Error()
		} else {
			result["status"] = "deleted"
		}
		results = append(results, result)
	}

	response.Success(c, gin.H{"results": results})
}

// ==================== 辅助函数 ====================

func parseClusterID(c *gin.Context) (uint, error) {
	var clusterID uint
	n, err := fmt.Sscanf(c.Param("id"), "%d", &clusterID)
	if err != nil || n == 0 || clusterID == 0 {
		return 0, fmt.Errorf("invalid cluster id")
	}
	return clusterID, nil
}

func timeSince(t time.Time) string {
	duration := time.Since(t)
	if duration.Hours() > 24*30 {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
	if duration.Hours() > 24 {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
	if duration.Hours() > 1 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	if duration.Minutes() > 1 {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	return fmt.Sprintf("%ds", int(duration.Seconds()))
}

func int32Value(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func dedupNodes(nodes []ResourceNode) []ResourceNode {
	seen := make(map[string]bool)
	var result []ResourceNode
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			result = append(result, n)
		}
	}
	return result
}

func dedupEdges(edges []ResourceEdge) []ResourceEdge {
	seen := make(map[string]bool)
	var result []ResourceEdge
	for _, e := range edges {
		key := fmt.Sprintf("%s->%s", e.Source, e.Target)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}
