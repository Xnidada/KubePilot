package aiops

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
)

func TestDeploymentReadyAndProductionRollbackAllowlist(t *testing.T) {
	want := int32(2)
	d := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &want}}
	d.Generation = 3
	d.Status = appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, ReadyReplicas: 2, AvailableReplicas: 2}
	if deploymentReady(d) {
		t.Fatal("stale generation considered ready")
	}
	d.Status.ObservedGeneration = 3
	if !deploymentReady(d) {
		t.Fatal("ready deployment rejected")
	}
	if ProductionAutoRollbackSupported("delete_deployment") || !ProductionAutoRollbackSupported("scale_deployment") {
		t.Fatal("unsafe production rollback allowlist")
	}
}
