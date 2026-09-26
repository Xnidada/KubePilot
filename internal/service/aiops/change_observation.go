package aiops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductionAutoRollbackSupported limits automatic execution to changes with
// a safe, narrowly scoped compensation. Destructive/arbitrary YAML needs a
// separate runbook rather than a pretend rollback.
func ProductionAutoRollbackSupported(action string) bool {
	switch action {
	case "create_deployment", "scale_deployment", "update_deployment":
		return true
	default:
		return false
	}
}

func (s *Service) DeploymentPrecondition(ctx context.Context, clusterID uint, params StagedActionParams) (string, int64, error) {
	if !ProductionAutoRollbackSupported(params.Action) {
		return "", 0, nil
	}
	cluster, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", 0, err
	}
	current, err := cluster.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
	if params.Action == "create_deployment" {
		if apierrors.IsNotFound(err) {
			return "", 0, nil
		}
		if err == nil {
			return "", 0, fmt.Errorf("deployment already exists; preview again")
		}
		return "", 0, err
	}
	if err != nil {
		return "", 0, err
	}
	return string(current.UID), current.Generation, nil
}

func (s *Service) VerifyDeploymentPrecondition(ctx context.Context, action *model.AgentAction) error {
	var params StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return err
	}
	if !ProductionAutoRollbackSupported(params.Action) {
		return fmt.Errorf("unsafe production action")
	}
	if params.Action != "create_deployment" && (action.ResourceUID == "" || action.BaseGeneration == 0) {
		return fmt.Errorf("change lacks a resource snapshot; preview again")
	}
	uid, generation, err := s.DeploymentPrecondition(ctx, action.ClusterID, params)
	if err != nil {
		return err
	}
	if uid != action.ResourceUID || generation != action.BaseGeneration {
		return fmt.Errorf("deployment changed after preview; preview and approve again")
	}
	return nil
}

// ExecuteObservedDeploymentChange waits for rollout health and compensates on failure.
func (s *Service) ExecuteObservedDeploymentChange(ctx context.Context, action *model.AgentAction) (*ExecuteResult, string, string, error) {
	var params StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return nil, "", "", err
	}
	if !ProductionAutoRollbackSupported(params.Action) {
		return nil, "", "", fmt.Errorf("production automation does not support safe rollback for %s", params.Action)
	}
	if err := s.VerifyDeploymentPrecondition(ctx, action); err != nil {
		return nil, "", "", err
	}
	cluster, err := k8s.Manager.GetClient(action.ClusterID)
	if err != nil {
		return nil, "", "", err
	}
	deployments := cluster.Clientset.AppsV1().Deployments(params.Namespace)
	var before *appsv1.Deployment
	if params.Action != "create_deployment" {
		before, err = deployments.Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return nil, "", "", err
		}
	} else if _, err = deployments.Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
		return nil, "", "", fmt.Errorf("deployment already exists; preview again")
	} else if !apierrors.IsNotFound(err) {
		return nil, "", "", err
	}
	result, err := s.ExecuteStagedActionWithRetry(ctx, action)
	if err != nil || result == nil || !result.Success {
		return result, "execution not confirmed; inspect live state", "not attempted after ambiguous execution", err
	}
	after, err := deployments.Get(ctx, params.Name, metav1.GetOptions{})
	if err != nil {
		return result, "unable to fetch deployment after execution", "manual verification required", err
	}
	uid, generation := after.UID, after.Generation
	observeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var observation string
	for {
		current, getErr := deployments.Get(observeCtx, params.Name, metav1.GetOptions{})
		if getErr == nil {
			if current.UID != uid || current.Generation != generation {
				observation = "deployment changed concurrently during observation"
				break
			}
			if deploymentReady(current) {
				return result, fmt.Sprintf("ready: generation=%d replicas=%d", generation, current.Status.ReadyReplicas), "", nil
			}
			for _, condition := range current.Status.Conditions {
				if condition.Type == appsv1.DeploymentProgressing && condition.Status == "False" && condition.Reason == "ProgressDeadlineExceeded" {
					observation = "deployment exceeded progress deadline"
					break
				}
			}
		} else {
			observation = fmt.Sprintf("deployment observation failed: %v", getErr)
		}
		if observation != "" {
			break
		}
		select {
		case <-observeCtx.Done():
			observation = "deployment did not become ready within 60 seconds"
		case <-time.After(2 * time.Second):
		}
		if observation != "" {
			break
		}
	}
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer rollbackCancel()
	current, err := deployments.Get(rollbackCtx, params.Name, metav1.GetOptions{})
	if err != nil {
		return result, observation, "manual rollback required: resource unavailable", errors.New(observation)
	}
	if current.UID != uid || current.Generation != generation {
		return result, observation, "manual rollback required: resource changed concurrently", errors.New(observation)
	}
	rollbackGeneration := int64(0)
	if before == nil {
		err = deployments.Delete(rollbackCtx, params.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
	} else {
		current.Spec = *before.Spec.DeepCopy()
		var restored *appsv1.Deployment
		restored, err = deployments.Update(rollbackCtx, current, metav1.UpdateOptions{})
		if err == nil {
			rollbackGeneration = restored.Generation
		}
	}
	if err != nil {
		return result, observation, "automatic rollback failed: " + err.Error(), errors.New(observation)
	}
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer verifyCancel()
	for {
		observed, getErr := deployments.Get(verifyCtx, params.Name, metav1.GetOptions{})
		if before == nil && apierrors.IsNotFound(getErr) {
			return result, observation, "rollback verified: created deployment removed", errors.New(observation)
		}
		if before != nil && getErr == nil && observed.UID == uid && observed.Generation == rollbackGeneration && deploymentReady(observed) {
			return result, observation, "rollback verified: previous deployment spec ready", errors.New(observation)
		}
		if getErr == nil && (observed.UID != uid || (before != nil && observed.Generation > rollbackGeneration)) {
			return result, observation, "rollback submitted but resource changed concurrently; manual verification required", errors.New(observation)
		}
		select {
		case <-verifyCtx.Done():
			return result, observation, "rollback submitted; verification timed out", errors.New(observation)
		case <-time.After(2 * time.Second):
		}
	}
}

func deploymentReady(deployment *appsv1.Deployment) bool {
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return false
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	return deployment.Status.ReadyReplicas >= desired && deployment.Status.AvailableReplicas >= desired && deployment.Status.Replicas == desired
}
