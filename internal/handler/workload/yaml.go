package workload

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetResourceYAML 获取资源的YAML格式
func (h *Handler) GetResourceYAML(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	resourceType := c.Param("type")
	namespace := c.Param("ns")
	name := c.Param("name")

	if h.kubectlExecutor == nil {
		response.InternalError(c, "kubectl executor not initialized")
		return
	}

	// 使用 kubectl get -o yaml 获取资源
	var args []string
	if namespace != "" {
		args = []string{"get", resourceType, name, "-n", namespace, "-o", "yaml"}
	} else {
		args = []string{"get", resourceType, name, "-o", "yaml"}
	}

	success, output, errMsg, err := h.kubectlExecutor.ExecuteKubectl(context.Background(), uint(clusterID), args)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if !success {
		response.InternalError(c, errMsg)
		return
	}

	response.Success(c, gin.H{
		"resource":  resourceType,
		"name":      name,
		"namespace": namespace,
		"yaml":      output,
	})
}

// ApplyResourceYAML 通过YAML创建/更新资源
func (h *Handler) ApplyResourceYAML(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		YAML string `json:"yaml" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	success, output, errMsg, err := h.kubectlExecutor.ExecuteKubectlApply(context.Background(), uint(clusterID), req.YAML)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if !success {
		response.BadRequest(c, errMsg)
		return
	}

	response.Success(c, gin.H{
		"message": "resource applied successfully",
		"output":  output,
	})
}

// DeleteResourceYAML 通过YAML删除资源
func (h *Handler) DeleteResourceYAML(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		YAML string `json:"yaml" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	success, output, errMsg, err := h.kubectlExecutor.ExecuteKubectlDelete(context.Background(), uint(clusterID), req.YAML)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if !success {
		response.BadRequest(c, errMsg)
		return
	}

	response.Success(c, gin.H{
		"message": "resource deleted successfully",
		"output":  output,
	})
}

// GetResourceEvents 获取资源相关事件
func (h *Handler) GetResourceEvents(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	resourceType := c.Param("type")
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 构建 field selector
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", name, getKindFromResourceType(resourceType))
	if namespace != "" {
		fieldSelector += ",involvedObject.namespace=" + namespace
	}

	events, err := client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type EventInfo struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Count     int32  `json:"count"`
		FirstTime string `json:"first_time"`
		LastTime  string `json:"last_time"`
	}

	result := make([]EventInfo, 0, len(events.Items))
	for _, e := range events.Items {
		firstTime := ""
		if !e.FirstTimestamp.IsZero() {
			firstTime = e.FirstTimestamp.Format("2006-01-02 15:04:05")
		}
		lastTime := ""
		if !e.LastTimestamp.IsZero() {
			lastTime = e.LastTimestamp.Format("2006-01-02 15:04:05")
		}
		result = append(result, EventInfo{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Count:     e.Count,
			FirstTime: firstTime,
			LastTime:  lastTime,
		})
	}

	response.Success(c, result)
}

// DescribeResource 获取资源的 describe 信息
func (h *Handler) DescribeResource(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	resourceType := c.Param("type")
	namespace := c.Param("ns")
	name := c.Param("name")

	var args []string
	if namespace != "" {
		args = []string{"describe", resourceType, name, "-n", namespace}
	} else {
		args = []string{"describe", resourceType, name}
	}

	success, output, errMsg, err := h.kubectlExecutor.ExecuteKubectl(context.Background(), uint(clusterID), args)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if !success {
		response.InternalError(c, errMsg)
		return
	}

	response.Success(c, gin.H{
		"resource": resourceType,
		"name":     name,
		"describe": output,
	})
}

// getKindFromResourceType 获取资源类型的 Kind
func getKindFromResourceType(resourceType string) string {
	kindMap := map[string]string{
		"pods":         "Pod",
		"deployments":  "Deployment",
		"services":     "Service",
		"configmaps":   "ConfigMap",
		"secrets":      "Secret",
		"ingresses":    "Ingress",
		"namespaces":   "Namespace",
		"nodes":        "Node",
		"pv":           "PersistentVolume",
		"pvc":          "PersistentVolumeClaim",
		"statefulsets": "StatefulSet",
		"daemonsets":   "DaemonSet",
		"jobs":         "Job",
		"cronjobs":     "CronJob",
		"replicasets":  "ReplicaSet",
	}
	if kind, ok := kindMap[resourceType]; ok {
		return kind
	}
	return resourceType
}
