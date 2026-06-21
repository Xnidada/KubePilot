package workload

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ==================== ConfigMap ====================

// ListConfigMaps 获取ConfigMap列表
func (h *Handler) ListConfigMaps(c *gin.Context) {
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
	var configMaps *corev1.ConfigMapList
	if namespace == "" {
		configMaps, err = client.Clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	} else {
		configMaps, err = client.Clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type ConfigMapInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Keys      []string `json:"keys"`
		DataCount int      `json:"data_count"`
		Age       string   `json:"age"`
	}

	result := make([]ConfigMapInfo, 0, len(configMaps.Items))
	for _, cm := range configMaps.Items {
		keys := make([]string, 0)
		for k := range cm.Data {
			keys = append(keys, k)
		}

		// 检查是否处于Terminating状态
		status := "Active"
		if cm.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, ConfigMapInfo{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Status:    status,
			Keys:      keys,
			DataCount: len(cm.Data),
			Age:       timeSince(cm.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// GetConfigMap 获取ConfigMap详情
func (h *Handler) GetConfigMap(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	cm, err := client.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "configmap not found")
		return
	}

	response.Success(c, cm)
}

// CreateConfigMap 创建ConfigMap
func (h *Handler) CreateConfigMap(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string            `json:"namespace" binding:"required"`
		Name      string            `json:"name" binding:"required"`
		Data      map[string]string `json:"data"`
		Labels    map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels:    req.Labels,
		},
		Data: req.Data,
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().ConfigMaps(req.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateConfigMap 更新ConfigMap
func (h *Handler) UpdateConfigMap(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Data   map[string]string `json:"data"`
		Labels map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	cm, err := client.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "configmap not found")
		return
	}

	if req.Data != nil {
		cm.Data = req.Data
	}
	if req.Labels != nil {
		cm.Labels = req.Labels
	}

	_, err = client.Clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "configmap updated", nil)
}

// DeleteConfigMap 删除ConfigMap
func (h *Handler) DeleteConfigMap(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "configmap deleted", nil)
}

// ==================== Secret ====================

// ListSecrets 获取Secret列表
func (h *Handler) ListSecrets(c *gin.Context) {
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
	var secrets *corev1.SecretList
	if namespace == "" {
		secrets, err = client.Clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	} else {
		secrets, err = client.Clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type SecretInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Type      string   `json:"type"`
		Keys      []string `json:"keys"`
		DataCount int      `json:"data_count"`
		Age       string   `json:"age"`
	}

	result := make([]SecretInfo, 0, len(secrets.Items))
	for _, s := range secrets.Items {
		keys := make([]string, 0)
		for k := range s.Data {
			keys = append(keys, k)
		}

		// 检查是否处于Terminating状态
		status := "Active"
		if s.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, SecretInfo{
			Name:      s.Name,
			Namespace: s.Namespace,
			Status:    status,
			Type:      string(s.Type),
			Keys:      keys,
			DataCount: len(s.Data),
			Age:       timeSince(s.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// GetSecret 获取Secret详情
func (h *Handler) GetSecret(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "secret not found")
		return
	}

	// 返回时将data转为string（base64编码）
	result := map[string]interface{}{
		"name":      secret.Name,
		"namespace": secret.Namespace,
		"type":      string(secret.Type),
		"data":      secret.Data,
		"labels":    secret.Labels,
		"age":       timeSince(secret.CreationTimestamp.Time),
	}

	response.Success(c, result)
}

// CreateSecret 创建Secret
func (h *Handler) CreateSecret(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string            `json:"namespace" binding:"required"`
		Name      string            `json:"name" binding:"required"`
		Type      string            `json:"type"`
		Data      map[string]string `json:"data"` // base64 encoded
		Labels    map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.Type == "" {
		req.Type = "Opaque"
	}

	// 转换string data为bytes
	data := make(map[string][]byte)
	for k, v := range req.Data {
		data[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels:    req.Labels,
		},
		Type: corev1.SecretType(req.Type),
		Data: data,
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().Secrets(req.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateSecret 更新Secret
func (h *Handler) UpdateSecret(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Data   map[string]string `json:"data"`
		Labels map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "secret not found")
		return
	}

	if req.Data != nil {
		data := make(map[string][]byte)
		for k, v := range req.Data {
			data[k] = []byte(v)
		}
		secret.Data = data
	}
	if req.Labels != nil {
		secret.Labels = req.Labels
	}

	_, err = client.Clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "secret updated", nil)
}

// DeleteSecret 删除Secret
func (h *Handler) DeleteSecret(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "secret deleted", nil)
}

// ==================== Ingress ====================

// ListIngresses 获取Ingress列表
func (h *Handler) ListIngresses(c *gin.Context) {
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
	var ingresses *networkingv1.IngressList
	if namespace == "" {
		ingresses, err = client.Clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	} else {
		ingresses, err = client.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type IngressInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		ClassName string   `json:"class_name"`
		Hosts     []string `json:"hosts"`
		Paths     []string `json:"paths"`
		Address   string   `json:"address"`
		TLS       bool     `json:"tls"`
		Age       string   `json:"age"`
	}

	result := make([]IngressInfo, 0, len(ingresses.Items))
	for _, ing := range ingresses.Items {
		hosts := make([]string, 0)
		paths := make([]string, 0)
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hosts = append(hosts, rule.Host)
			}
			if rule.HTTP != nil {
				for _, p := range rule.HTTP.Paths {
					paths = append(paths, p.Path)
				}
			}
		}

		className := ""
		if ing.Spec.IngressClassName != nil {
			className = *ing.Spec.IngressClassName
		}

		address := ""
		if len(ing.Status.LoadBalancer.Ingress) > 0 {
			address = ing.Status.LoadBalancer.Ingress[0].IP
		}

		// 检查是否处于Terminating状态
		status := "Active"
		if ing.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, IngressInfo{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			Status:    status,
			ClassName: className,
			Hosts:     hosts,
			Paths:     paths,
			Address:   address,
			TLS:       len(ing.Spec.TLS) > 0,
			Age:       timeSince(ing.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// GetIngress 获取Ingress详情
func (h *Handler) GetIngress(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	ing, err := client.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "ingress not found")
		return
	}

	response.Success(c, ing)
}

// CreateIngress 创建Ingress
func (h *Handler) CreateIngress(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string            `json:"namespace" binding:"required"`
		Name      string            `json:"name" binding:"required"`
		ClassName string            `json:"class_name"`
		Host      string            `json:"host" binding:"required"`
		Paths     []struct {
			Path     string `json:"path"`
			PathType string `json:"path_type"`
			Service  string `json:"service" binding:"required"`
			Port     int32  `json:"port" binding:"required"`
		} `json:"paths" binding:"required"`
		TLSSecretName string            `json:"tls_secret"`
		Annotations   map[string]string `json:"annotations"`
		Labels        map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	pathType := networkingv1.PathTypePrefix
	ingressPaths := make([]networkingv1.HTTPIngressPath, 0)
	for _, p := range req.Paths {
		pt := networkingv1.PathType(p.PathType)
		if pt == "" {
			pt = pathType
		}
		ingressPaths = append(ingressPaths, networkingv1.HTTPIngressPath{
			Path:     p.Path,
			PathType: &pt,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: p.Service,
					Port: networkingv1.ServiceBackendPort{
						Number: p.Port,
					},
				},
			},
		})
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   req.Namespace,
			Labels:      req.Labels,
			Annotations: req.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &req.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: req.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: ingressPaths,
						},
					},
				},
			},
		},
	}

	if req.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{req.Host},
				SecretName: req.TLSSecretName,
			},
		}
	}

	ctx := context.Background()
	result, err := client.Clientset.NetworkingV1().Ingresses(req.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateIngress 更新Ingress
func (h *Handler) UpdateIngress(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Host  string `json:"host"`
		Paths []struct {
			Path     string `json:"path"`
			PathType string `json:"path_type"`
			Service  string `json:"service"`
			Port     int32  `json:"port"`
		} `json:"paths"`
		TLSSecretName string            `json:"tls_secret"`
		Annotations   map[string]string `json:"annotations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	ing, err := client.Clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "ingress not found")
		return
	}

	if req.Host != "" || req.Paths != nil {
		pathType := networkingv1.PathTypePrefix
		ingressPaths := make([]networkingv1.HTTPIngressPath, 0)
		for _, p := range req.Paths {
			pt := networkingv1.PathType(p.PathType)
			if pt == "" {
				pt = pathType
			}
			ingressPaths = append(ingressPaths, networkingv1.HTTPIngressPath{
				Path:     p.Path,
				PathType: &pt,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: p.Service,
						Port: networkingv1.ServiceBackendPort{
							Number: p.Port,
						},
					},
				},
			})
		}

		host := req.Host
		if host == "" && len(ing.Spec.Rules) > 0 {
			host = ing.Spec.Rules[0].Host
		}

		ing.Spec.Rules = []networkingv1.IngressRule{
			{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: ingressPaths,
					},
				},
			},
		}
	}

	if req.TLSSecretName != "" {
		host := ""
		if len(ing.Spec.Rules) > 0 {
			host = ing.Spec.Rules[0].Host
		}
		ing.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{host},
				SecretName: req.TLSSecretName,
			},
		}
	}

	if req.Annotations != nil {
		ing.Annotations = req.Annotations
	}

	_, err = client.Clientset.NetworkingV1().Ingresses(namespace).Update(ctx, ing, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "ingress updated", nil)
}

// DeleteIngress 删除Ingress
func (h *Handler) DeleteIngress(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "ingress deleted", nil)
}

// ==================== Namespace ====================

// CreateNamespace 创建命名空间
func (h *Handler) CreateNamespace(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Name   string            `json:"name" binding:"required"`
		Labels map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   req.Name,
			Labels: req.Labels,
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// DeleteNamespace 删除命名空间
func (h *Handler) DeleteNamespace(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	// 不允许删除系统命名空间
	systemNamespaces := []string{"default", "kube-system", "kube-public", "kube-node-lease"}
	for _, ns := range systemNamespaces {
		if name == ns {
			response.BadRequest(c, "cannot delete system namespace")
			return
		}
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "namespace deleted", nil)
}

// UpdateNamespace 更新命名空间
func (h *Handler) UpdateNamespace(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	var req struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	ns, err := client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "namespace not found")
		return
	}

	// 更新标签
	if req.Labels != nil {
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		for k, v := range req.Labels {
			ns.Labels[k] = v
		}
	}

	// 更新注解
	if req.Annotations != nil {
		if ns.Annotations == nil {
			ns.Annotations = make(map[string]string)
		}
		for k, v := range req.Annotations {
			ns.Annotations[k] = v
		}
	}

	result, err := client.Clientset.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// GetNamespaceDetail 获取命名空间详情
func (h *Handler) GetNamespaceDetail(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	ns, err := client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "namespace not found")
		return
	}

	// 获取命名空间下的资源统计
	pods, _ := client.Clientset.CoreV1().Pods(name).List(ctx, metav1.ListOptions{})
	services, _ := client.Clientset.CoreV1().Services(name).List(ctx, metav1.ListOptions{})
	deployments, _ := client.Clientset.AppsV1().Deployments(name).List(ctx, metav1.ListOptions{})
	configMaps, _ := client.Clientset.CoreV1().ConfigMaps(name).List(ctx, metav1.ListOptions{})
	secrets, _ := client.Clientset.CoreV1().Secrets(name).List(ctx, metav1.ListOptions{})

	result := map[string]interface{}{
		"name":   ns.Name,
		"status": string(ns.Status.Phase),
		"labels": ns.Labels,
		"age":    timeSince(ns.CreationTimestamp.Time),
		"resources": map[string]int{
			"pods":        len(pods.Items),
			"services":    len(services.Items),
			"deployments": len(deployments.Items),
			"configmaps":  len(configMaps.Items),
			"secrets":     len(secrets.Items),
		},
	}

	response.Success(c, result)
}

// ==================== Resource Quota ====================

// GetResourceQuota 获取资源配额
func (h *Handler) GetResourceQuota(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	quotas, err := client.Clientset.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, quotas.Items)
}

// CreateResourceQuota 创建资源配额
func (h *Handler) CreateResourceQuota(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		CPU       string `json:"cpu"`
		Memory    string `json:"memory"`
		Pods      string `json:"pods"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	hard := corev1.ResourceList{}
	if req.CPU != "" {
		hard[corev1.ResourceRequestsCPU] = resource.MustParse(req.CPU)
	}
	if req.Memory != "" {
		hard[corev1.ResourceRequestsMemory] = resource.MustParse(req.Memory)
	}
	if req.Pods != "" {
		hard[corev1.ResourcePods] = resource.MustParse(req.Pods)
	}

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: hard,
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().ResourceQuotas(req.Namespace).Create(ctx, quota, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateResourceQuota 更新资源配额
func (h *Handler) UpdateResourceQuota(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	var req struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
		Pods   string `json:"pods"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取现有的 ResourceQuota
	// 需要知道 namespace，从 URL 或 query 获取
	namespace := c.Query("ns")
	if namespace == "" {
		namespace = "default"
	}

	quota, err := client.Clientset.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "resource quota not found")
		return
	}

	// 更新配额
	if quota.Spec.Hard == nil {
		quota.Spec.Hard = corev1.ResourceList{}
	}
	if req.CPU != "" {
		quota.Spec.Hard[corev1.ResourceRequestsCPU] = resource.MustParse(req.CPU)
	}
	if req.Memory != "" {
		quota.Spec.Hard[corev1.ResourceRequestsMemory] = resource.MustParse(req.Memory)
	}
	if req.Pods != "" {
		quota.Spec.Hard[corev1.ResourcePods] = resource.MustParse(req.Pods)
	}

	result, err := client.Clientset.CoreV1().ResourceQuotas(namespace).Update(ctx, quota, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// DeleteResourceQuota 删除资源配额
func (h *Handler) DeleteResourceQuota(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")
	namespace := c.Query("ns")
	if namespace == "" {
		namespace = "default"
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().ResourceQuotas(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "resource quota deleted", nil)
}
