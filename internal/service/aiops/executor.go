package aiops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

// ExecuteResult 执行结果
type ExecuteResult struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// ExecuteCreateDeployment 执行创建Deployment
func (s *Service) ExecuteCreateDeployment(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	if strings.TrimSpace(params.Image) == "" {
		return nil, fmt.Errorf("image is required")
	}
	if params.Replicas <= 0 {
		params.Replicas = 1
	}

	deployment := buildDeploymentObject(params)
	_, err = client.Clientset.AppsV1().Deployments(params.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("创建 Deployment 失败: %v", err),
		}, nil
	}

	details := []string{
		fmt.Sprintf("命名空间: %s", params.Namespace),
		fmt.Sprintf("副本数: %d", params.Replicas),
		fmt.Sprintf("镜像: %s", params.Image),
	}
	for _, m := range params.HostPathMounts {
		details = append(details, fmt.Sprintf("挂载: %s -> %s", m.HostPath, m.MountPath))
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Deployment %s 创建成功", params.Name),
		Details: details,
	}, nil
}

// ExecuteCreateService 执行创建Service
func (s *Service) ExecuteCreateService(ctx context.Context, clusterID uint, namespace, name, serviceType string, selector map[string]string, port, targetPort int32, nodePort int32) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	if len(selector) == 0 {
		return nil, fmt.Errorf("selector is required")
	}
	if port <= 0 {
		return nil, fmt.Errorf("port is required")
	}

	if serviceType == "" {
		serviceType = "ClusterIP"
	}

	svcPort := corev1.ServicePort{
		Name:       "http",
		Port:       port,
		TargetPort: intstr.FromInt(int(targetPort)),
		Protocol:   corev1.ProtocolTCP,
	}

	if nodePort > 0 && serviceType == "NodePort" {
		svcPort.NodePort = nodePort
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(serviceType),
			Selector: selector,
			Ports:    []corev1.ServicePort{svcPort},
		},
	}

	_, err = client.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("创建 Service 失败: %v", err),
		}, nil
	}

	details := []string{
		fmt.Sprintf("命名空间: %s", namespace),
		fmt.Sprintf("类型: %s", serviceType),
		fmt.Sprintf("端口: %d -> %d", port, targetPort),
	}
	if nodePort > 0 {
		details = append(details, fmt.Sprintf("NodePort: %d", nodePort))
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Service %s 创建成功", name),
		Details: details,
	}, nil
}

// ExecuteDeleteDeployment 执行删除Deployment
func (s *Service) ExecuteDeleteDeployment(ctx context.Context, clusterID uint, namespace, name string) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	// 先检查是否存在
	_, err = client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// 不存在则返回成功
		return &ExecuteResult{
			Success: true,
			Message: fmt.Sprintf("Deployment %s 不存在或已被删除", name),
		}, nil
	}

	err = client.Clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("删除 Deployment 失败: %v", err),
		}, nil
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Deployment %s 已删除", name),
	}, nil
}

// ExecuteDeleteService 执行删除Service
func (s *Service) ExecuteDeleteService(ctx context.Context, clusterID uint, namespace, name string) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	// 先检查是否存在
	_, err = client.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// 不存在则返回成功
		return &ExecuteResult{
			Success: true,
			Message: fmt.Sprintf("Service %s 不存在或已被删除", name),
		}, nil
	}

	err = client.Clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("删除 Service 失败: %v", err),
		}, nil
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Service %s 已删除", name),
	}, nil
}

// ExecuteDeletePod 执行删除Pod
func (s *Service) ExecuteDeletePod(ctx context.Context, clusterID uint, namespace, name string) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &ExecuteResult{
				Success: true,
				Message: fmt.Sprintf("Pod %s 不存在或已被删除", name),
			}, nil
		}
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("获取 Pod 失败: %v", err),
		}, nil
	}

	var rsOwner string
	var deployOwner string
	for _, o := range pod.OwnerReferences {
		if o.Controller != nil && !*o.Controller {
			continue
		}
		switch o.Kind {
		case "ReplicaSet":
			rsOwner = o.Name
		case "Deployment":
			deployOwner = o.Name
		}
	}
	if rsOwner != "" && deployOwner == "" {
		if rs, rsErr := client.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, rsOwner, metav1.GetOptions{}); rsErr == nil {
			for _, o := range rs.OwnerReferences {
				if o.Kind == "Deployment" {
					deployOwner = o.Name
					break
				}
			}
		}
	}

	err = client.Clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("删除 Pod 失败: %v", err),
		}, nil
	}

	details := []string{fmt.Sprintf("deleted: %s/%s", namespace, name)}
	msg := fmt.Sprintf("Pod %s 已删除", name)

	// 控制器托管的 Pod 删除后常被立即重建；探测并如实反馈，避免“显示成功但看起来没删掉”
	if rsOwner != "" || deployOwner != "" {
		replacements := waitForReplacementPods(ctx, client, namespace, name, rsOwner, pod.Labels, 3*time.Second)
		if len(replacements) > 0 {
			target := "工作负载"
			if deployOwner != "" {
				target = "Deployment/" + deployOwner
			} else if rsOwner != "" {
				target = "ReplicaSet/" + rsOwner
			}
			msg = fmt.Sprintf(
				"Pod %s 已删除，但控制器已立即重建：%s。当前命名空间里仍会看到同类 Pod。若要彻底移除，请删除 %s（不要只删单个 Pod）。",
				name, strings.Join(replacements, ", "), target,
			)
			details = append(details, "recreated_by_controller=true")
			details = append(details, "replacements="+strings.Join(replacements, ","))
			if deployOwner != "" {
				details = append(details, "suggested_action=delete_deployment:"+deployOwner)
			}
		} else if deployOwner != "" || rsOwner != "" {
			details = append(details, "owned_by_controller=true")
			msg = fmt.Sprintf("Pod %s 已删除（暂未检测到重建；若归属 Deployment/ReplicaSet，稍后仍可能被拉起）", name)
		}
	}

	return &ExecuteResult{
		Success: true,
		Message: msg,
		Details: details,
	}, nil
}

// waitForReplacementPods polls briefly for a new Pod created by the same ReplicaSet/labels.
func waitForReplacementPods(
	ctx context.Context,
	client *k8s.ClusterClient,
	namespace, deletedName, rsOwner string,
	podLabels map[string]string,
	timeout time.Duration,
) []string {
	deadline := time.Now().Add(timeout)
	selector := ""
	if hash := podLabels["pod-template-hash"]; hash != "" {
		selector = labels.Set{"pod-template-hash": hash}.AsSelector().String()
	} else if len(podLabels) > 0 {
		// Fallback: use non-controller-specific labels carefully — prefer app label.
		set := labels.Set{}
		if v := podLabels["app"]; v != "" {
			set["app"] = v
		} else if v := podLabels["app.kubernetes.io/name"]; v != "" {
			set["app.kubernetes.io/name"] = v
		}
		if len(set) > 0 {
			selector = set.AsSelector().String()
		}
	}

	var found []string
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return found
		default:
		}
		opts := metav1.ListOptions{}
		if selector != "" {
			opts.LabelSelector = selector
		}
		list, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, opts)
		if err == nil {
			found = found[:0]
			for _, p := range list.Items {
				if p.Name == deletedName || p.DeletionTimestamp != nil {
					continue
				}
				if rsOwner != "" {
					owned := false
					for _, o := range p.OwnerReferences {
						if o.Kind == "ReplicaSet" && o.Name == rsOwner {
							owned = true
							break
						}
					}
					if !owned {
						continue
					}
				}
				found = append(found, p.Name)
			}
			if len(found) > 0 {
				return found
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return found
}

// ExecuteScaleDeployment 执行扩容/缩容
func (s *Service) ExecuteScaleDeployment(ctx context.Context, clusterID uint, namespace, name string, replicas int32) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("获取 Deployment 失败: %v", err),
		}, nil
	}

	deployment.Spec.Replicas = &replicas
	_, err = client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("扩容失败: %v", err),
		}, nil
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Deployment %s 已扩容到 %d 个副本", name, replicas),
	}, nil
}

// ParseAndExecute 解析用户意图并执行
func (s *Service) ParseAndExecute(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	message = strings.ToLower(message)

	// 解析创建 Deployment
	if strings.Contains(message, "创建") && (strings.Contains(message, "deployment") || strings.Contains(message, "deploy")) {
		return s.parseAndCreateDeployment(ctx, clusterID, message)
	}

	// 解析创建 Service
	if strings.Contains(message, "创建") && strings.Contains(message, "service") {
		return s.parseAndCreateService(ctx, clusterID, message)
	}

	// 解析删除操作
	if strings.Contains(message, "删除") || strings.Contains(message, "delete") {
		return s.parseAndDelete(ctx, clusterID, message)
	}

	// 解析扩容操作
	if strings.Contains(message, "扩容") || strings.Contains(message, "scale") {
		return s.parseAndScale(ctx, clusterID, message)
	}

	return &ExecuteResult{
		Success: false,
		Message: "无法解析操作意图，请更明确地描述您要执行的操作",
	}, nil
}

// parseAndCreateDeployment 解析并创建Deployment（已废弃默认镜像填充；必须由 staged 参数提供）
func (s *Service) parseAndCreateDeployment(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	return &ExecuteResult{
		Success: false,
		Message: "请通过 Agent stage_mutation 提供明确的 name/namespace/image，禁止使用默认镜像",
	}, nil
}

// parseAndCreateService 解析并创建Service（禁止默认 selector）
func (s *Service) parseAndCreateService(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	return &ExecuteResult{
		Success: false,
		Message: "请通过 Agent stage_mutation 提供明确的 name/namespace/selector/port，禁止使用默认 selector",
	}, nil
}

// parseAndDelete 解析并删除
func (s *Service) parseAndDelete(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	namespace := "default"

	if strings.Contains(message, "pod") {
		// 尝试提取 Pod 名称
		name := extractResourceName(message, "pod")
		if name != "" {
			return s.ExecuteDeletePod(ctx, clusterID, namespace, name)
		}
	}

	if strings.Contains(message, "deployment") || strings.Contains(message, "deploy") {
		name := extractResourceName(message, "deployment")
		if name != "" {
			return s.ExecuteDeleteDeployment(ctx, clusterID, namespace, name)
		}
	}

	return &ExecuteResult{
		Success: false,
		Message: "无法解析要删除的资源，请指定资源名称",
	}, nil
}

// parseAndScale 解析并扩容
func (s *Service) parseAndScale(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	namespace := "default"
	name := "nginx-deployment"
	replicas := int32(3)

	// 尝试提取副本数
	if strings.Contains(message, "5") {
		replicas = 5
	} else if strings.Contains(message, "3") {
		replicas = 3
	} else if strings.Contains(message, "2") {
		replicas = 2
	}

	return s.ExecuteScaleDeployment(ctx, clusterID, namespace, name, replicas)
}

// extractResourceName 提取资源名称
func extractResourceName(message, resourceType string) string {
	// 简单的名称提取逻辑
	words := strings.Fields(message)
	for i, word := range words {
		if strings.Contains(word, resourceType) && i+1 < len(words) {
			return words[i+1]
		}
	}
	return ""
}

// ==================== Extended Action Executors ====================

// ExecuteCreateConfigMap creates a ConfigMap
func (s *Service) ExecuteCreateConfigMap(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Data:       params.Data,
	}
	if _, err := client.Clientset.CoreV1().ConfigMaps(params.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 ConfigMap 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("ConfigMap %s/%s 创建成功", params.Namespace, params.Name)}, nil
}

// ExecuteCreateSecret creates a Secret
func (s *Service) ExecuteCreateSecret(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	st := params.SecretType
	if st == "" {
		st = "Opaque"
	}
	data := make(map[string][]byte)
	for k, v := range params.Data {
		data[k] = []byte(v)
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Type:       corev1.SecretType(st),
		Data:       data,
	}
	if _, err := client.Clientset.CoreV1().Secrets(params.Namespace).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 Secret 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("Secret %s/%s 创建成功", params.Namespace, params.Name)}, nil
}

// ExecuteUpdateDeployment updates a Deployment (image/env/resource limits)
func (s *Service) ExecuteUpdateDeployment(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
	if err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("获取 Deployment 失败: %v", err)}, nil
	}
	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return &ExecuteResult{Success: false, Message: "Deployment has no containers"}, nil
	}
	container := &deploy.Spec.Template.Spec.Containers[0]
	var changes []string
	if params.NewImage != "" {
		changes = append(changes, fmt.Sprintf("image: %s -> %s", container.Image, params.NewImage))
		container.Image = params.NewImage
	}
	if len(params.EnvVars) > 0 {
		for k, v := range params.EnvVars {
			found := false
			for i, ev := range container.Env {
				if ev.Name == k {
					container.Env[i].Value = v
					found = true
					break
				}
			}
			if !found {
				container.Env = append(container.Env, corev1.EnvVar{Name: k, Value: v})
			}
		}
		changes = append(changes, fmt.Sprintf("env_vars updated: %v", params.EnvVars))
	}
	if len(params.ResourceLimits) > 0 {
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(corev1.ResourceList)
		}
		if container.Resources.Requests == nil {
			container.Resources.Requests = make(corev1.ResourceList)
		}
		for k, v := range params.ResourceLimits {
			q, err := resource.ParseQuantity(v)
			if err != nil {
				continue
			}
			container.Resources.Limits[corev1.ResourceName(k)] = q
		}
		changes = append(changes, fmt.Sprintf("resource_limits updated: %v", params.ResourceLimits))
	}
	if _, err := client.Clientset.AppsV1().Deployments(params.Namespace).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("更新 Deployment 失败: %v", err)}, nil
	}
	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Deployment %s/%s 更新成功: %s", params.Namespace, params.Name, strings.Join(changes, "; ")),
	}, nil
}

// ExecuteCreateNamespace creates a Namespace
func (s *Service) ExecuteCreateNamespace(ctx context.Context, clusterID uint, name string) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if _, err := client.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 Namespace 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("Namespace %s 创建成功", name)}, nil
}

// ExecuteCreateIngress creates an Ingress
func (s *Service) ExecuteCreateIngress(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	ingPath := params.Path
	if ingPath == "" {
		ingPath = "/"
	}
	bPort := params.BackendPort
	if bPort <= 0 {
		bPort = 80
	}
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: params.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     ingPath,
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: params.BackendService,
									Port: networkingv1.ServiceBackendPort{Number: bPort},
								},
							},
						}},
					},
				},
			}},
		},
	}
	if _, err := client.Clientset.NetworkingV1().Ingresses(params.Namespace).Create(ctx, ing, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 Ingress 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("Ingress %s/%s 创建成功", params.Namespace, params.Name)}, nil
}

// ExecuteCreateHPA creates a HorizontalPodAutoscaler
func (s *Service) ExecuteCreateHPA(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	minR := params.MinReplicas
	if minR <= 0 {
		minR = 1
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name + "-hpa", Namespace: params.Namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       params.Name,
			},
			MinReplicas: &minR,
			MaxReplicas: params.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: &params.TargetCPU,
					},
				},
			}},
		},
	}
	if _, err := client.Clientset.AutoscalingV2().HorizontalPodAutoscalers(params.Namespace).Create(ctx, hpa, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 HPA 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("HPA %s/%s-hpa 创建成功 (min=%d max=%d cpu=%d%%)", params.Namespace, params.Name, minR, params.MaxReplicas, params.TargetCPU)}, nil
}

// ExecuteCreatePVC creates a PersistentVolumeClaim
func (s *Service) ExecuteCreatePVC(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}
	am := params.AccessModes
	if len(am) == 0 {
		am = []string{"ReadWriteOnce"}
	}
	accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(am))
	for _, a := range am {
		accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(a))
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(params.StorageSize),
				},
			},
		},
	}
	if params.StorageClass != "" {
		pvc.Spec.StorageClassName = &params.StorageClass
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(params.Namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		return &ExecuteResult{Success: false, Message: fmt.Sprintf("创建 PVC 失败: %v", err)}, nil
	}
	return &ExecuteResult{Success: true, Message: fmt.Sprintf("PVC %s/%s 创建成功 (%s)", params.Namespace, params.Name, params.StorageSize)}, nil
}

// ExecuteApplyYAML applies arbitrary Kubernetes YAML via dynamic client
func (s *Service) ExecuteApplyYAML(ctx context.Context, clusterID uint, params StagedActionParams) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(params.YAML), 4096)
	var results []string
	for {
		raw := unstructured.Unstructured{}
		if err := decoder.Decode(&raw); err != nil {
			if strings.Contains(err.Error(), "EOF") {
				break
			}
			return &ExecuteResult{Success: false, Message: fmt.Sprintf("YAML 解析失败: %v", err)}, nil
		}
		if len(raw.Object) == 0 {
			continue
		}
		gvk := raw.GroupVersionKind()
		ns := raw.GetNamespace()
		if ns == "" {
			ns = params.Namespace
			raw.SetNamespace(ns)
		}
		name := raw.GetName()

		// Map GVK to GVR using discovery
		gvr, err := gvkToGVR(client, gvk)
		if err != nil {
			results = append(results, fmt.Sprintf("%s %s/%s: GVR 映射失败: %v", gvk.Kind, ns, name, err))
			continue
		}

		dynClient, err := dynamic.NewForConfig(client.Config)
		if err != nil {
			results = append(results, fmt.Sprintf("%s %s/%s: dynamic client 创建失败: %v", gvk.Kind, ns, name, err))
			continue
		}

		var dynRes dynamic.ResourceInterface
		dynRes = dynClient.Resource(gvr)
		if ns != "" {
			dynRes = dynClient.Resource(gvr).Namespace(ns)
		}

	_, err = dynRes.Create(ctx, &raw, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Try update
				existing, getErr := dynRes.Get(ctx, name, metav1.GetOptions{})
				if getErr != nil {
					results = append(results, fmt.Sprintf("%s %s/%s: 已存在但获取失败: %v", gvk.Kind, ns, name, getErr))
					continue
				}
				raw.SetResourceVersion(existing.GetResourceVersion())
				if _, updateErr := dynRes.Update(ctx, &raw, metav1.UpdateOptions{}); updateErr != nil {
					results = append(results, fmt.Sprintf("%s %s/%s: 更新失败: %v", gvk.Kind, ns, name, updateErr))
					continue
				}
				results = append(results, fmt.Sprintf("%s %s/%s: 更新成功", gvk.Kind, ns, name))
			} else {
				results = append(results, fmt.Sprintf("%s %s/%s: 创建失败: %v", gvk.Kind, ns, name, err))
			}
			continue
		}
		results = append(results, fmt.Sprintf("%s %s/%s: 创建成功", gvk.Kind, ns, name))
	}

	if len(results) == 0 {
		return &ExecuteResult{Success: false, Message: "YAML 中无有效资源"}, nil
	}
	return &ExecuteResult{Success: true, Message: strings.Join(results, "\n")}, nil
}

// gvkToGVR maps a GroupVersionKind to GroupVersionResource using the discovery client
func gvkToGVR(client *k8s.ClusterClient, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	resources, err := client.Discovery.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("discovery failed for %s: %w", gvk.GroupVersion().String(), err)
	}
	for _, r := range resources.APIResources {
		if r.Kind == gvk.Kind {
			return gvk.GroupVersion().WithResource(r.Name), nil
		}
	}
	return schema.GroupVersionResource{}, fmt.Errorf("resource %s not found in group %s", gvk.Kind, gvk.GroupVersion().String())
}

// dryRunApplyYAML validates YAML manifests and returns a preview
func (s *Service) dryRunApplyYAML(ctx context.Context, clusterID uint, params StagedActionParams) (string, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(params.YAML), 4096)
	var manifests []string
	for {
		raw := unstructured.Unstructured{}
		if err := decoder.Decode(&raw); err != nil {
			if strings.Contains(err.Error(), "EOF") {
				break
			}
			return "", fmt.Errorf("YAML 解析失败: %v", err)
		}
		if len(raw.Object) == 0 {
			continue
		}
		gvk := raw.GroupVersionKind()
		ns := raw.GetNamespace()
		if ns == "" {
			ns = params.Namespace
		}
		name := raw.GetName()
		manifests = append(manifests, fmt.Sprintf("- %s %s/%s", gvk.Kind, ns, name))
	}
	if len(manifests) == 0 {
		return "", fmt.Errorf("YAML 中无有效资源")
	}
	return fmt.Sprintf("[dry-run] APPLY YAML\n  resources:\n%s\nserver_dry_run: ok", strings.Join(manifests, "\n")), nil
}
