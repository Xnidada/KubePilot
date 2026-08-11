package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// StagedActionParams stores the executable payload for a pending AI action.
type StagedActionParams struct {
	Action      string            `json:"action"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Image       string            `json:"image,omitempty"`
	Replicas    int32             `json:"replicas,omitempty"`
	Ports       []int32           `json:"ports,omitempty"`
	ServiceType string            `json:"service_type,omitempty"`
	Port        int32             `json:"port,omitempty"`
	TargetPort  int32             `json:"target_port,omitempty"`
	NodePort    int32             `json:"node_port,omitempty"`
	Selector    map[string]string `json:"selector,omitempty"`
}

// DryRunStagedAction returns a field-level impact preview.
// Create paths use server-side DryRunAll when possible.
func (s *Service) DryRunStagedAction(ctx context.Context, clusterID uint, params StagedActionParams) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", fmt.Errorf("cluster not connected: %w", err)
	}

	switch params.Action {
	case "create_deployment":
		if strings.TrimSpace(params.Image) == "" {
			return "", fmt.Errorf("image is required for create_deployment")
		}
		if _, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("deployment %s/%s already exists", params.Namespace, params.Name)
		}
		replicas := params.Replicas
		if replicas <= 0 {
			replicas = 1
		}
		dep := buildDeploymentObject(params.Namespace, params.Name, params.Image, replicas, params.Ports)
		_, err := client.Clientset.AppsV1().Deployments(params.Namespace).Create(ctx, dep, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})
		if err != nil {
			return "", fmt.Errorf("server dry-run failed: %w", err)
		}
		return fmt.Sprintf(`[dry-run] CREATE Deployment %s/%s
diff:
  + apiVersion: apps/v1
  + kind: Deployment
  + metadata.name: %s
  + metadata.namespace: %s
  + spec.replicas: %d
  + spec.template.spec.containers[0].image: %s
  + spec.template.spec.containers[0].ports: %v
server_dry_run: ok`, params.Namespace, params.Name, params.Name, params.Namespace, replicas, params.Image, params.Ports), nil

	case "create_service":
		if len(params.Selector) == 0 {
			return "", fmt.Errorf("selector is required for create_service")
		}
		if params.Port <= 0 {
			return "", fmt.Errorf("port is required for create_service")
		}
		if _, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("service %s/%s already exists", params.Namespace, params.Name)
		}
		svc := buildServiceObject(params)
		_, err := client.Clientset.CoreV1().Services(params.Namespace).Create(ctx, svc, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})
		if err != nil {
			return "", fmt.Errorf("server dry-run failed: %w", err)
		}
		tp := params.TargetPort
		if tp <= 0 {
			tp = params.Port
		}
		nodePortLine := "  + spec.ports[0].nodePort: (auto — NOT SET; external port unknown)"
		if params.NodePort > 0 {
			nodePortLine = fmt.Sprintf("  + spec.ports[0].nodePort: %d", params.NodePort)
		}
		return fmt.Sprintf(`[dry-run] CREATE Service %s/%s
diff:
  + kind: Service
  + metadata.name: %s
  + metadata.namespace: %s
  + spec.type: %s
  + spec.selector: %v
  + spec.ports[0].port: %d
  + spec.ports[0].targetPort: %d
%s
server_dry_run: ok`, params.Namespace, params.Name, params.Name, params.Namespace, params.ServiceType, params.Selector, params.Port, tp, nodePortLine), nil

	case "delete_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		img := ""
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			img = deploy.Spec.Template.Spec.Containers[0].Image
		}
		age := time.Since(deploy.CreationTimestamp.Time).Truncate(time.Second)
		return fmt.Sprintf(`[dry-run] DELETE Deployment %s/%s
impact:
  - replicas: %d (ready=%d available=%d)
  - image: %s
  - labels: %v
  - age: %s
  - will cascade delete owned ReplicaSets/Pods`,
			params.Namespace, params.Name, replicas, deploy.Status.ReadyReplicas, deploy.Status.AvailableReplicas, img, deploy.Labels, age), nil

	case "delete_service":
		svc, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("service %s/%s not found", params.Namespace, params.Name)
		}
		return fmt.Sprintf(`[dry-run] DELETE Service %s/%s
impact:
  - type: %s
  - clusterIP: %s
  - selector: %v
  - ports: %v`, params.Namespace, params.Name, svc.Spec.Type, svc.Spec.ClusterIP, svc.Spec.Selector, svc.Spec.Ports), nil

	case "delete_pod":
		pod, err := client.Clientset.CoreV1().Pods(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("pod %s/%s not found", params.Namespace, params.Name)
		}
		owners := make([]string, 0, len(pod.OwnerReferences))
		for _, o := range pod.OwnerReferences {
			owners = append(owners, fmt.Sprintf("%s/%s", o.Kind, o.Name))
		}
		return fmt.Sprintf(`[dry-run] DELETE Pod %s/%s
impact:
  - phase: %s
  - node: %s
  - restarts: see containerStatuses
  - owners: %v
  - labels: %v
note: if owned by ReplicaSet/Deployment it may be recreated`,
			params.Namespace, params.Name, pod.Status.Phase, pod.Spec.NodeName, owners, pod.Labels), nil

	case "scale_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		current := int32(0)
		if deploy.Spec.Replicas != nil {
			current = *deploy.Spec.Replicas
		}
		delta := params.Replicas - current
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		return fmt.Sprintf(`[dry-run] SCALE Deployment %s/%s
diff:
  - spec.replicas: %d
  + spec.replicas: %d  (%s%d)
status_snapshot:
  - readyReplicas: %d
  - availableReplicas: %d
  - updatedReplicas: %d
  - unavailableReplicas: %d`,
			params.Namespace, params.Name, current, params.Replicas, sign, delta,
			deploy.Status.ReadyReplicas, deploy.Status.AvailableReplicas, deploy.Status.UpdatedReplicas, deploy.Status.UnavailableReplicas), nil

	default:
		return "", fmt.Errorf("unsupported action: %s", params.Action)
	}
}

func buildDeploymentObject(namespace, name, image string, replicas int32, ports []int32) *appsv1.Deployment {
	containerPorts := make([]corev1.ContainerPort, 0, len(ports))
	for _, p := range ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{ContainerPort: p})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: namespace,
			Labels: map[string]string{"app": name},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: name, Image: image, Ports: containerPorts}},
				},
			},
		},
	}
}

func buildServiceObject(params StagedActionParams) *corev1.Service {
	st := params.ServiceType
	if params.NodePort > 0 {
		st = string(corev1.ServiceTypeNodePort)
	}
	if st == "" {
		st = string(corev1.ServiceTypeClusterIP)
	}
	tp := params.TargetPort
	if tp <= 0 {
		tp = params.Port
	}
	port := corev1.ServicePort{
		Name: "http", Port: params.Port, TargetPort: intstr.FromInt(int(tp)), Protocol: corev1.ProtocolTCP,
	}
	if params.NodePort > 0 && st == string(corev1.ServiceTypeNodePort) {
		port.NodePort = params.NodePort
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(st),
			Selector: params.Selector,
			Ports:    []corev1.ServicePort{port},
		},
	}
}

func (s *Service) ExecuteStagedAction(ctx context.Context, action *model.AgentAction) (*ExecuteResult, error) {
	var params StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return nil, fmt.Errorf("invalid staged parameters: %w", err)
	}

	switch params.Action {
	case "create_deployment":
		return s.ExecuteCreateDeployment(ctx, action.ClusterID, params.Namespace, params.Name, params.Image, params.Replicas, params.Ports)
	case "create_service":
		return s.ExecuteCreateService(ctx, action.ClusterID, params.Namespace, params.Name, params.ServiceType, params.Selector, params.Port, params.TargetPort, params.NodePort)
	case "delete_deployment":
		return s.ExecuteDeleteDeployment(ctx, action.ClusterID, params.Namespace, params.Name)
	case "delete_service":
		return s.ExecuteDeleteService(ctx, action.ClusterID, params.Namespace, params.Name)
	case "delete_pod":
		return s.ExecuteDeletePod(ctx, action.ClusterID, params.Namespace, params.Name)
	case "scale_deployment":
		return s.ExecuteScaleDeployment(ctx, action.ClusterID, params.Namespace, params.Name, params.Replicas)
	default:
		return nil, fmt.Errorf("unsupported action: %s", params.Action)
	}
}
