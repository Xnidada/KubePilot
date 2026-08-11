package aiops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (s *Service) DryRunStagedAction(ctx context.Context, clusterID uint, params StagedActionParams) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", fmt.Errorf("cluster not connected: %w", err)
	}

	switch params.Action {
	case "create_deployment":
		_, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err == nil {
			return "", fmt.Errorf("deployment %s/%s already exists", params.Namespace, params.Name)
		}
		return fmt.Sprintf("[dry-run] will create Deployment %s/%s image=%s replicas=%d", params.Namespace, params.Name, params.Image, params.Replicas), nil
	case "create_service":
		_, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err == nil {
			return "", fmt.Errorf("service %s/%s already exists", params.Namespace, params.Name)
		}
		return fmt.Sprintf("[dry-run] will create Service %s/%s type=%s port=%d", params.Namespace, params.Name, params.ServiceType, params.Port), nil
	case "delete_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		return fmt.Sprintf("[dry-run] will delete Deployment %s/%s (replicas=%d)", params.Namespace, params.Name, replicas), nil
	case "delete_service":
		if _, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err != nil {
			return "", fmt.Errorf("service %s/%s not found", params.Namespace, params.Name)
		}
		return fmt.Sprintf("[dry-run] will delete Service %s/%s", params.Namespace, params.Name), nil
	case "delete_pod":
		if _, err := client.Clientset.CoreV1().Pods(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err != nil {
			return "", fmt.Errorf("pod %s/%s not found", params.Namespace, params.Name)
		}
		return fmt.Sprintf("[dry-run] will delete Pod %s/%s", params.Namespace, params.Name), nil
	case "scale_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		current := int32(0)
		if deploy.Spec.Replicas != nil {
			current = *deploy.Spec.Replicas
		}
		return fmt.Sprintf("[dry-run] will scale Deployment %s/%s from %d to %d", params.Namespace, params.Name, current, params.Replicas), nil
	default:
		return "", fmt.Errorf("unsupported action: %s", params.Action)
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
