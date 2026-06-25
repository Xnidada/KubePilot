package workload

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
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

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var obj runtime.Object

	// 根据资源类型获取对象
	switch resourceType {
	case "deployments":
		deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "deployment not found")
			return
		}
		deploy.ManagedFields = nil // 清理 managedFields
		obj = deploy
	case "statefulsets":
		sts, err := client.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "statefulset not found")
			return
		}
		sts.ManagedFields = nil
		obj = sts
	case "daemonsets":
		ds, err := client.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "daemonset not found")
			return
		}
		ds.ManagedFields = nil
		obj = ds
	case "replicasets":
		rs, err := client.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "replicaset not found")
			return
		}
		rs.ManagedFields = nil
		obj = rs
	case "pods":
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "pod not found")
			return
		}
		pod.ManagedFields = nil
		obj = pod
	case "services":
		svc, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "service not found")
			return
		}
		svc.ManagedFields = nil
		obj = svc
	case "configmaps":
		cm, err := client.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "configmap not found")
			return
		}
		cm.ManagedFields = nil
		obj = cm
	case "secrets":
		secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "secret not found")
			return
		}
		secret.ManagedFields = nil
		// Base64 编码 secret data
		for k, v := range secret.Data {
			secret.StringData[k] = string(v)
		}
		secret.Data = nil
		obj = secret
	case "ingresses":
		ing, err := client.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "ingress not found")
			return
		}
		ing.ManagedFields = nil
		obj = ing
	case "namespaces":
		ns, err := client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "namespace not found")
			return
		}
		ns.ManagedFields = nil
		obj = ns
	case "pv":
		pv, err := client.Clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "pv not found")
			return
		}
		pv.ManagedFields = nil
		obj = pv
	case "pvc":
		pvc, err := client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "pvc not found")
			return
		}
		pvc.ManagedFields = nil
		obj = pvc
	case "jobs":
		job, err := client.Clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "job not found")
			return
		}
		job.ManagedFields = nil
		obj = job
	case "cronjobs":
		cj, err := client.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			response.NotFound(c, "cronjob not found")
			return
		}
		cj.ManagedFields = nil
		obj = cj
	default:
		response.BadRequest(c, "unsupported resource type: "+resourceType)
		return
	}

	// 转换为 YAML
	yamlSerializer := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	var buf bytes.Buffer
	if err := yamlSerializer.Encode(obj, &buf); err != nil {
		response.InternalError(c, "failed to encode yaml: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"resource":  resourceType,
		"name":      name,
		"namespace": namespace,
		"yaml":      buf.String(),
	})
}

// ApplyResourceYAML 通过YAML创建/更新资源
func (h *Handler) ApplyResourceYAML(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	if h.kubectlExecutor == nil {
		response.InternalError(c, "kubectl executor not initialized")
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

	if h.kubectlExecutor == nil {
		response.InternalError(c, "kubectl executor not initialized")
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

	if h.kubectlExecutor == nil {
		response.InternalError(c, "kubectl executor not initialized")
		return
	}

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
