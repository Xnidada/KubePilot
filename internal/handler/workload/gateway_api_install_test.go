package workload

import (
	"context"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestGatewayInstallPreflightNeverOverwritesExistingResources(t *testing.T) {
	for _, test := range []struct {
		name      string
		crd       string
		namespace bool
		blocked   bool
	}{
		{name: "empty cluster"},
		{name: "existing Gateway API CRD", crd: "gateways.gateway.networking.k8s.io", blocked: true},
		{name: "existing Envoy CRD", crd: "envoyproxies.gateway.envoyproxy.io", blocked: true},
		{name: "existing namespace", namespace: true, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var objects []runtime.Object
			if test.crd != "" {
				objects = append(objects, &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
					"metadata": map[string]interface{}{"name": test.crd},
				}})
			}
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
				map[schema.GroupVersionResource]string{gatewayCRDGVR: "CustomResourceDefinitionList"}, objects...)
			client := kubernetesfake.NewSimpleClientset()
			if test.namespace {
				if _, err := client.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: gatewayInstallerNamespace},
				}, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			reason, err := gatewayInstallPreflight(context.Background(), client, dynamicClient)
			if err != nil {
				t.Fatal(err)
			}
			if got := reason != ""; got != test.blocked {
				t.Fatalf("blocked = %v, want %v (reason: %s)", got, test.blocked, reason)
			}
		})
	}
}

func TestGatewayEnvoyControllerStatus(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset()
	if got := gatewayEnvoyControllerStatus(context.Background(), client); got != "absent" {
		t.Fatalf("missing deployment status = %q", got)
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "envoy-gateway", Namespace: gatewayInstallerNamespace, Generation: 2}}
	if _, err := client.AppsV1().Deployments(gatewayInstallerNamespace).Create(context.Background(), deployment, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := gatewayEnvoyControllerStatus(context.Background(), client); got != "not_ready" {
		t.Fatalf("unavailable deployment status = %q", got)
	}
	deployment.Status.AvailableReplicas = 1
	deployment.Status.ObservedGeneration = 2
	if _, err := client.AppsV1().Deployments(gatewayInstallerNamespace).UpdateStatus(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := gatewayEnvoyControllerStatus(context.Background(), client); got != "ready" {
		t.Fatalf("available deployment status = %q", got)
	}
}

func TestPinnedGatewayManifestVerifiesDigest(t *testing.T) {
	manifest, err := loadPinnedGatewayManifest(gatewayInstallerSHA256)
	if err != nil || len(manifest) < 1_000_000 || !strings.Contains(string(manifest), "kind: Deployment") {
		t.Fatalf("embedded manifest rejected: length %d, %v", len(manifest), err)
	}
	if _, err := loadPinnedGatewayManifest(strings.Repeat("0", 64)); err == nil {
		t.Fatal("changed manifest was accepted")
	}
}

func TestSupportedGatewayKubernetesVersion(t *testing.T) {
	for _, test := range []struct {
		major, minor string
		want         bool
	}{
		{"1", "32", false}, {"1", "33", true}, {"1", "34+", true}, {"1", "36", true}, {"1", "37", false}, {"2", "34", false},
	} {
		if got := supportedGatewayKubernetesVersion(test.major, test.minor); got != test.want {
			t.Fatalf("version %s.%s support = %v, want %v", test.major, test.minor, got, test.want)
		}
	}
}

func TestGatewayKubectlCompatible(t *testing.T) {
	for _, test := range []struct {
		server, client string
		want           bool
	}{
		{"33", "34", true}, {"34", "35", true}, {"36", "35", true}, {"36", "34", false}, {"34", "36", false},
	} {
		if got := gatewayKubectlCompatible("1", test.server, "1", test.client); got != test.want {
			t.Fatalf("kubectl %s, server %s: got %v, want %v", test.client, test.server, got, test.want)
		}
	}
}

func TestGatewayControllerWaitAndLogCap(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy-gateway", Namespace: gatewayInstallerNamespace, Generation: 2},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, ObservedGeneration: 2},
	})
	if ready, reason := waitGatewayController(context.Background(), client, func(string) {}); !ready || reason != "" {
		t.Fatalf("available controller not detected: %v, %s", ready, reason)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	logs := []string{}
	if ready, _ := waitGatewayController(cancelled, kubernetesfake.NewSimpleClientset(), func(line string) { logs = append(logs, line) }); ready || len(logs) == 0 {
		t.Fatalf("pending controller must emit progress: ready=%v, logs=%v", ready, logs)
	}
	if got := gatewayInstallLogText(strings.Repeat("x", 20<<10)); len(got) > (16<<10)+40 || !strings.Contains(got, "已截断") {
		t.Fatalf("install log not capped: %d bytes", len(got))
	}
}

type fakeGatewayInstallExecutor struct {
	args    []string
	success bool
	output  string
	errMsg  string
}

func (f *fakeGatewayInstallExecutor) ExecuteKubectl(_ context.Context, _ uint, args []string) (bool, string, string, error) {
	f.args = args
	return f.success, f.output, f.errMsg, nil
}

func (*fakeGatewayInstallExecutor) ExecuteKubectlApply(context.Context, uint, string) (bool, string, string, error) {
	panic("unexpected ExecuteKubectlApply")
}

func (*fakeGatewayInstallExecutor) ExecuteKubectlDelete(context.Context, uint, string) (bool, string, string, error) {
	panic("unexpected ExecuteKubectlDelete")
}

func TestRunGatewayInstallOutcome(t *testing.T) {
	readyClient := kubernetesfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy-gateway", Namespace: gatewayInstallerNamespace, Generation: 1},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, ObservedGeneration: 1},
	})
	for _, test := range []struct {
		name        string
		success     bool
		output      string
		errMsg      string
		wantStatus  string
		wantMessage string
	}{
		{name: "ready", success: true, output: "deployment.apps/envoy-gateway configured", wantStatus: "ready", wantMessage: "deployment.apps/envoy-gateway configured"},
		{name: "apply failed", errMsg: "forbidden: create customresourcedefinitions", wantStatus: "failed", wantMessage: "forbidden: create customresourcedefinitions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := &fakeGatewayInstallExecutor{success: test.success, output: test.output, errMsg: test.errMsg}
			h := NewHandler()
			h.SetKubectlExecutor(executor)
			var logs []string
			status, message := h.runGatewayInstall(context.Background(), 7, readyClient, "/tmp/test-gateway.yaml", func(_, line string) { logs = append(logs, line) })
			if got := executor.args; !reflect.DeepEqual(got, []string{"apply", "--server-side", "-f", "/tmp/test-gateway.yaml"}) {
				t.Fatalf("kubectl args = %v", got)
			}
			if status != test.wantStatus || !strings.Contains(message+strings.Join(logs, "\n"), test.wantMessage) {
				t.Fatalf("status=%s message=%s logs=%v", status, message, logs)
			}
		})
	}
}

type fakeStreamingGatewayExecutor struct{ fakeGatewayInstallExecutor }

func (f *fakeStreamingGatewayExecutor) ExecuteKubectlStream(_ context.Context, _ uint, args []string, emit func(string, string)) (bool, error) {
	f.args = args
	emit("info", "deployment.apps/envoy-gateway configured")
	return true, nil
}

func TestRunGatewayInstallUsesLiveOutput(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy-gateway", Namespace: gatewayInstallerNamespace, Generation: 1},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, ObservedGeneration: 1},
	})
	h := NewHandler()
	executor := &fakeStreamingGatewayExecutor{}
	h.SetKubectlExecutor(executor)
	var logs []string
	status, _ := h.runGatewayInstall(context.Background(), 7, client, "/tmp/test-gateway.yaml", func(_, line string) { logs = append(logs, line) })
	if status != "ready" || !strings.Contains(strings.Join(logs, "\n"), "deployment.apps/envoy-gateway configured") {
		t.Fatalf("status=%s logs=%v", status, logs)
	}
}
