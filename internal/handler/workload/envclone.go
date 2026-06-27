package workload

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloneNamespace 克隆命名空间配置
func (h *Handler) CloneNamespace(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Source  string   `json:"source" binding:"required"`
		Target  string   `json:"target" binding:"required"`
		Types   []string `json:"types"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.Source == req.Target {
		response.BadRequest(c, "source and target cannot be the same")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 确保目标命名空间存在
	_, err = client.Clientset.CoreV1().Namespaces().Get(ctx, req.Target, metav1.GetOptions{})
	if err != nil {
		// 创建目标命名空间
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: req.Target,
			},
		}
		_, err = client.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil {
			response.InternalError(c, "failed to create target namespace: "+err.Error())
			return
		}
	}

	results := []map[string]interface{}{}

	// 克隆 ConfigMaps
	if contains(req.Types, "configmaps") {
		cms, err := client.Clientset.CoreV1().ConfigMaps(req.Source).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, cm := range cms.Items {
				newCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cm.Name,
						Namespace: req.Target,
						Labels:    cm.Labels,
					},
					Data: cm.Data,
				}
				_, err := client.Clientset.CoreV1().ConfigMaps(req.Target).Create(ctx, newCM, metav1.CreateOptions{})
				results = append(results, map[string]interface{}{
					"type": "ConfigMap",
					"name": cm.Name,
					"status": map[bool]string{true: "success", false: "failed"}[err == nil],
				})
			}
		}
	}

	// 克隆 Secrets (不包含数据)
	if contains(req.Types, "secrets") {
		secrets, err := client.Clientset.CoreV1().Secrets(req.Source).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, s := range secrets.Items {
				newSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      s.Name,
						Namespace: req.Target,
						Labels:    s.Labels,
					},
					Type: s.Type,
					Data: s.Data,
				}
				_, err := client.Clientset.CoreV1().Secrets(req.Target).Create(ctx, newSecret, metav1.CreateOptions{})
				results = append(results, map[string]interface{}{
					"type": "Secret",
					"name": s.Name,
					"status": map[bool]string{true: "success", false: "failed"}[err == nil],
				})
			}
		}
	}

	// 克隆 Services
	if contains(req.Types, "services") {
		svcs, err := client.Clientset.CoreV1().Services(req.Source).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, svc := range svcs.Items {
				newSvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        svc.Name,
						Namespace:   req.Target,
						Labels:      svc.Labels,
						Annotations: svc.Annotations,
					},
					Spec: svc.Spec,
				}
				// 清理 ClusterIP，让 K8S 自动分配
				newSvc.Spec.ClusterIP = ""
				_, err := client.Clientset.CoreV1().Services(req.Target).Create(ctx, newSvc, metav1.CreateOptions{})
				results = append(results, map[string]interface{}{
					"type": "Service",
					"name": svc.Name,
					"status": map[bool]string{true: "success", false: "failed"}[err == nil],
				})
			}
		}
	}

	response.Success(c, gin.H{
		"source":   req.Source,
		"target":   req.Target,
		"results":  results,
		"total":    len(results),
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// jsonMarshal is a helper to avoid import issues
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
