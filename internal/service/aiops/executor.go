package aiops

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubepilot/kubepilot/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ExecuteResult 执行结果
type ExecuteResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Details []string `json:"details,omitempty"`
}

// ExecuteCreateDeployment 执行创建Deployment
func (s *Service) ExecuteCreateDeployment(ctx context.Context, clusterID uint, namespace, name, image string, replicas int32, ports []int32) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
	}

	if replicas == 0 {
		replicas = 1
	}

	// 构建容器端口
	containerPorts := make([]corev1.ContainerPort, 0)
	for _, p := range ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: p,
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							Ports: containerPorts,
						},
					},
				},
			},
		},
	}

	_, err = client.Clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("创建 Deployment 失败: %v", err),
		}, nil
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Deployment %s 创建成功", name),
		Details: []string{
			fmt.Sprintf("命名空间: %s", namespace),
			fmt.Sprintf("副本数: %d", replicas),
			fmt.Sprintf("镜像: %s", image),
		},
	}, nil
}

// ExecuteCreateService 执行创建Service
func (s *Service) ExecuteCreateService(ctx context.Context, clusterID uint, namespace, name, serviceType string, selector map[string]string, port, targetPort int32, nodePort int32) (*ExecuteResult, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster not connected: %w", err)
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

	// 先检查是否存在
	_, err = client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// 不存在则返回成功
		return &ExecuteResult{
			Success: true,
			Message: fmt.Sprintf("Pod %s 不存在或已被删除", name),
		}, nil
	}

	err = client.Clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("删除 Pod 失败: %v", err),
		}, nil
	}

	return &ExecuteResult{
		Success: true,
		Message: fmt.Sprintf("Pod %s 已删除", name),
	}, nil
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

// parseAndCreateDeployment 解析并创建Deployment
func (s *Service) parseAndCreateDeployment(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	// 默认值
	name := "nginx-deployment"
	namespace := "default"
	image := "nginx:latest"
	replicas := int32(1)
	ports := []int32{80}

	// 尝试解析名称
	if strings.Contains(message, "nginx") {
		name = "nginx-deployment"
		image = "nginx:latest"
	} else if strings.Contains(message, "redis") {
		name = "redis-deployment"
		image = "redis:latest"
	} else if strings.Contains(message, "mysql") {
		name = "mysql-deployment"
		image = "mysql:latest"
	}

	// 尝试解析副本数
	if strings.Contains(message, "2个") || strings.Contains(message, "2副本") || strings.Contains(message, "两个") {
		replicas = 2
	} else if strings.Contains(message, "3个") || strings.Contains(message, "3副本") || strings.Contains(message, "三个") {
		replicas = 3
	}

	return s.ExecuteCreateDeployment(ctx, clusterID, namespace, name, image, replicas, ports)
}

// parseAndCreateService 解析并创建Service
func (s *Service) parseAndCreateService(ctx context.Context, clusterID uint, message string) (*ExecuteResult, error) {
	name := "nginx-service"
	namespace := "default"
	serviceType := "NodePort"
	selector := map[string]string{"app": "nginx-deployment"}
	port := int32(80)
	targetPort := int32(80)
	nodePort := int32(30080)

	// 尝试解析 NodePort
	if strings.Contains(message, "30080") {
		nodePort = 30080
	} else if strings.Contains(message, "30081") {
		nodePort = 30081
	}

	return s.ExecuteCreateService(ctx, clusterID, namespace, name, serviceType, selector, port, targetPort, nodePort)
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
