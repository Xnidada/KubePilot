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
		Source    string `json:"source" binding:"required"`
		Target    string `json:"target" binding:"required"`
		Resources []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"resources"`
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

	for _, res := range req.Resources {
		result := map[string]interface{}{
			"kind": res.Kind,
			"name": res.Name,
		}

		var cloneErr error

		switch res.Kind {
		case "Deployment":
			deploy, err := client.Clientset.AppsV1().Deployments(req.Source).Get(ctx, res.Name, metav1.GetOptions{})
			if err != nil {
				cloneErr = err
				break
			}
			deploy.Name = res.Name
			deploy.Namespace = req.Target
			deploy.ResourceVersion = ""
			deploy.UID = ""
			deploy.CreationTimestamp = metav1.Now()
			_, cloneErr = client.Clientset.AppsV1().Deployments(req.Target).Create(ctx, deploy, metav1.CreateOptions{})

		case "Service":
			svc, err := client.Clientset.CoreV1().Services(req.Source).Get(ctx, res.Name, metav1.GetOptions{})
			if err != nil {
				cloneErr = err
				break
			}
			svc.Name = res.Name
			svc.Namespace = req.Target
			svc.ResourceVersion = ""
			svc.UID = ""
			svc.CreationTimestamp = metav1.Now()
			svc.Spec.ClusterIP = ""
			_, cloneErr = client.Clientset.CoreV1().Services(req.Target).Create(ctx, svc, metav1.CreateOptions{})

		case "ConfigMap":
			cm, err := client.Clientset.CoreV1().ConfigMaps(req.Source).Get(ctx, res.Name, metav1.GetOptions{})
			if err != nil {
				cloneErr = err
				break
			}
			cm.Name = res.Name
			cm.Namespace = req.Target
			cm.ResourceVersion = ""
			cm.UID = ""
			cm.CreationTimestamp = metav1.Now()
			_, cloneErr = client.Clientset.CoreV1().ConfigMaps(req.Target).Create(ctx, cm, metav1.CreateOptions{})

		case "Secret":
			secret, err := client.Clientset.CoreV1().Secrets(req.Source).Get(ctx, res.Name, metav1.GetOptions{})
			if err != nil {
				cloneErr = err
				break
			}
			secret.Name = res.Name
			secret.Namespace = req.Target
			secret.ResourceVersion = ""
			secret.UID = ""
			secret.CreationTimestamp = metav1.Now()
			_, cloneErr = client.Clientset.CoreV1().Secrets(req.Target).Create(ctx, secret, metav1.CreateOptions{})

		default:
			result["status"] = "skipped"
			result["message"] = "unsupported resource type"
			results = append(results, result)
			continue
		}

		if cloneErr != nil {
			result["status"] = "failed"
			result["message"] = cloneErr.Error()
		} else {
			result["status"] = "success"
		}
		results = append(results, result)
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
