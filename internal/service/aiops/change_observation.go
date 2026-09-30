package aiops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
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

// Read, validate and update the same object. Never replace its resourceVersion
// with a fresh read after approval validation, including on a retry.
func executeApprovedDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, action *model.AgentAction, params StagedActionParams) (*appsv1.Deployment, *appsv1.Deployment, error) {
	before, err := deployments.Get(ctx, params.Name, metav1.GetOptions{})
	var desired *appsv1.Deployment
	if params.Action == "create_deployment" {
		if err == nil {
			return nil, nil, fmt.Errorf("deployment already exists; preview again")
		}
		if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
		before = nil
		if params.Replicas <= 0 {
			params.Replicas = 1
		}
		desired = buildDeploymentObject(params)
	} else {
		if err != nil {
			return nil, nil, err
		}
		if action.ResourceUID == "" || action.BaseGeneration == 0 || string(before.UID) != action.ResourceUID || before.Generation != action.BaseGeneration {
			return nil, nil, fmt.Errorf("deployment changed or snapshot missing; preview and approve again")
		}
		desired = before.DeepCopy()
		switch params.Action {
		case "scale_deployment":
			desired.Spec.Replicas = &params.Replicas
		case "update_deployment":
			if _, err := updateDeploymentObject(desired, params); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, fmt.Errorf("unsafe production action")
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		var after *appsv1.Deployment
		if before == nil {
			after, err = deployments.Create(ctx, desired.DeepCopy(), metav1.CreateOptions{FieldValidation: "Strict"})
		} else {
			after, err = deployments.Update(ctx, desired.DeepCopy(), metav1.UpdateOptions{FieldValidation: "Strict"})
		}
		if err == nil {
			return before, after, nil
		}
		if !isTransientAgentFailure(err.Error()) {
			return before, nil, err
		}
		current, checkErr := deployments.Get(ctx, params.Name, metav1.GetOptions{})
		if before == nil {
			if !apierrors.IsNotFound(checkErr) {
				return before, nil, fmt.Errorf("creation outcome uncertain; inspect live state before retrying: %w", err)
			}
		} else {
			if checkErr != nil {
				return before, nil, fmt.Errorf("cannot verify failed write: %w", checkErr)
			}
			if current.UID == before.UID && current.Generation == before.Generation+1 && reflect.DeepEqual(current.Spec, desired.Spec) {
				return before, current, nil
			}
			if current.UID != before.UID || current.ResourceVersion != before.ResourceVersion {
				return before, nil, fmt.Errorf("deployment changed after failed write; preview again: %w", err)
			}
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(250<<attempt) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return before, nil, ctx.Err()
		case <-timer.C:
		}
	}
	return before, nil, err
}

// ExecuteObservedDeploymentChange waits for rollout health and compensates on failure.
func (s *Service) ExecuteObservedDeploymentChange(ctx context.Context, action *model.AgentAction) (*ExecuteResult, string, string, error) {
	// The action has already been claimed durably by the handler. A browser
	// disconnect must not cancel this confirmed operation or cause a rollback.
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer stop()
	var params StagedActionParams
	opened, err := s.openActionParameters(action.Parameters)
	if err != nil {
		return nil, "", "", err
	}
	if err := json.Unmarshal(opened, &params); err != nil {
		return nil, "", "", err
	}
	if !ProductionAutoRollbackSupported(params.Action) {
		return nil, "", "", fmt.Errorf("production automation does not support safe rollback for %s", params.Action)
	}
	cluster, err := k8s.Manager.GetClient(action.ClusterID)
	if err != nil {
		return nil, "", "", err
	}
	deployments := cluster.Clientset.AppsV1().Deployments(params.Namespace)
	before, after, err := executeApprovedDeployment(ctx, deployments, action, params)
	if err != nil {
		return nil, "execution not confirmed; inspect live state", "not attempted after unconfirmed execution", err
	}
	result := &ExecuteResult{Success: true, Message: fmt.Sprintf("Deployment %s/%s 变更完成", params.Namespace, params.Name)}
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
	rollbackCtx, rollbackCancel := context.WithTimeout(ctx, 15*time.Second)
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
		err = deployments.Delete(rollbackCtx, params.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &current.ResourceVersion}})
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
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 45*time.Second)
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
	return deployment.Status.UpdatedReplicas == desired && deployment.Status.ReadyReplicas >= desired && deployment.Status.AvailableReplicas >= desired && deployment.Status.Replicas == desired
}
