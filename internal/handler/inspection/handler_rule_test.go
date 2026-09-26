package inspection

import (
	"context"
	"strings"
	"testing"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInspectionRuleErrorNeverAcceptsUnimplementedChecks(t *testing.T) {
	for _, resource := range []string{"node", "pod", "deployment", "service"} {
		if got := inspectionRuleError(&model.InspectionRule{Resource: resource, CheckType: "status"}); got != "" {
			t.Fatalf("supported %s rule rejected: %s", resource, got)
		}
	}
	for _, rule := range []model.InspectionRule{
		{Resource: "custom", CheckType: "custom", Script: "return true"},
		{Resource: "pod", CheckType: "resource"},
		{Resource: "pod", CheckType: "status", Threshold: "80"},
		{Resource: "pod", CheckType: "status", Condition: ">"},
		{Resource: "pod", CheckType: "status", Script: "return true"},
		{Resource: "unknown", CheckType: "status"},
	} {
		if got := inspectionRuleError(&rule); !strings.Contains(got, "未执行检查") {
			t.Fatalf("unsupported rule %#v did not explain failure: %q", rule, got)
		}
	}
}

func TestInspectionResultsDoNotFalselyPass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []model.InspectionResult
		status  string
		failed  int
		warning int
	}{
		{"empty", nil, "failed", 0, 0},
		{"failed result", []model.InspectionResult{{Status: "pass"}, {Status: "fail"}}, "failed", 1, 0},
		{"warning only", []model.InspectionResult{{Status: "warn"}}, "completed", 0, 1},
		{"unknown result", []model.InspectionResult{{Status: "unknown"}}, "failed", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, failed, warning, _ := summarizeInspectionResults(tc.results)
			if status != tc.status || failed != tc.failed || warning != tc.warning {
				t.Fatalf("got status=%s failed=%d warning=%d", status, failed, warning)
			}
		})
	}
}

func TestInspectionChecksDoNotOverstateHealth(t *testing.T) {
	replicas := int32(1)
	client := &k8s.ClusterClient{Clientset: fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: false, RestartCount: 6}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "gated", Namespace: "default"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "default"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "not-available", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 0},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "rolling", Namespace: "default", Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 0, ObservedGeneration: 2},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "default", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1, ObservedGeneration: 1},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "unverified", Namespace: "default"},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1"},
		},
	)}
	h := &InspectionHandler{}
	rule := &model.InspectionRule{}
	pods := map[string]string{}
	for _, result := range h.checkPods(context.Background(), client, rule) {
		pods[result.ResourceName] = result.Status
	}
	if len(pods) != 3 || pods["not-ready"] != "fail" || pods["gated"] != "fail" || pods["healthy"] != "pass" {
		t.Fatalf("Pod readiness misclassified: %#v", pods)
	}
	deployments := map[string]string{}
	for _, result := range h.checkDeployments(context.Background(), client, rule) {
		deployments[result.ResourceName] = result.Status
	}
	if len(deployments) != 3 || deployments["not-available"] != "fail" || deployments["rolling"] != "fail" || deployments["healthy"] != "pass" {
		t.Fatalf("Deployment readiness misclassified: %#v", deployments)
	}
	if got := h.checkServices(context.Background(), client, rule); len(got) != 1 || got[0].Status != "warn" {
		t.Fatalf("Service with unverified backend must warn: %#v", got)
	}
}
