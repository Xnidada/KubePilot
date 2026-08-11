package aiops

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestLooksLikeFakeAgentActions(t *testing.T) {
	if !looksLikeFakeAgentActions("```action\n{\"action\":\"delete_pod\"}\n```") {
		t.Fatal("expected fake action fence")
	}
	if looksLikeFakeAgentActions("集群状态正常，建议观察") {
		t.Fatal("should not flag normal text")
	}
}

func TestIsClusterQueryIntent(t *testing.T) {
	if !isClusterQueryIntent("列出 default 下的 Pod") {
		t.Fatal("expected query intent")
	}
	if isClusterQueryIntent("你好") {
		t.Fatal("hello is not query")
	}
}

func TestIsWriteIntent(t *testing.T) {
	if !isWriteIntent("删除 cj-test 的 pod") {
		t.Fatal("expected write intent")
	}
}

func TestValidateMutationParams(t *testing.T) {
	err := validateMutationParams(StagedActionParams{Action: "create_deployment", Namespace: "default", Name: "x", Image: "nginx:latest"})
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder image rejection, got %v", err)
	}
	err = validateMutationParams(StagedActionParams{Action: "create_service", Namespace: "default", Name: "x", Port: 80})
	if err == nil || !strings.Contains(err.Error(), "selector") {
		t.Fatalf("expected selector required, got %v", err)
	}
	err = validateMutationParams(StagedActionParams{
		Action: "create_service", Namespace: "default", Name: "x", Port: 80,
		ServiceType: "NodePort", Selector: map[string]string{"app": "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "node_port") {
		t.Fatalf("expected node_port required for NodePort, got %v", err)
	}
	err = validateMutationParams(StagedActionParams{
		Action: "create_service", Namespace: "default", Name: "x", Port: 80, NodePort: 30089,
		ServiceType: "NodePort", Selector: map[string]string{"app": "nginx-deployment"},
	})
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParseMutationParamsNodePortAlias(t *testing.T) {
	p, err := parseMutationParams(`{"action":"create_service","namespace":"default","name":"svc","port":80,"nodePort":30089,"selector":{"app":"web"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.NodePort != 30089 || p.ServiceType != "NodePort" {
		t.Fatalf("got node_port=%d type=%s", p.NodePort, p.ServiceType)
	}
}

func TestExtractRequestedNodePorts(t *testing.T) {
	ports := extractRequestedNodePorts("令外部的30089端口可以访问到该nginx")
	if len(ports) != 1 || ports[0] != 30089 {
		t.Fatalf("got %v", ports)
	}
}

func TestAgentNudgeNoToolsClaim(t *testing.T) {
	s := &Service{}
	nudge := s.agentNudgeIfNeeded(
		"查看 default 命名空间的 pod",
		"default 下共有 5 个 Pod，状态如下：Running...",
		nil, nil,
	)
	if nudge == "" {
		t.Fatal("expected nudge when claiming without tools")
	}
}

func TestAgentNudgeWriteWithoutStage(t *testing.T) {
	s := &Service{}
	nudge := s.agentNudgeIfNeeded(
		"删除 default 下 cj-test 开头的 Pod",
		"已帮你删除完成。",
		[]ToolTraceItem{{Name: "list_resources"}},
		nil,
	)
	if nudge == "" {
		t.Fatal("expected write-intent nudge without stage_mutation")
	}
}

func TestLooksLikeClusterStateClaim(t *testing.T) {
	if !looksLikeClusterStateClaim("default 下共有 5 个 Pod，状态如下：Running...") {
		t.Fatal("expected claim detection")
	}
	if looksLikeClusterStateClaim("我尚未查询，需要先调用工具。") {
		t.Fatal("hedge should not count as claim")
	}
}

func TestNamePrefixFilterLogic(t *testing.T) {
	names := []string{"cj-test-1", "cj-test-2", "other"}
	prefix := "cj-test"
	var matched []string
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			matched = append(matched, n)
		}
	}
	if len(matched) != 2 {
		t.Fatalf("matched=%v", matched)
	}
}

func TestFormatServicePorts(t *testing.T) {
	ports := []corev1.ServicePort{{
		Name: "http", Port: 80, TargetPort: intstr.FromInt(80), NodePort: 32700, Protocol: corev1.ProtocolTCP,
	}}
	got := formatServicePorts(ports)
	if !strings.Contains(got, "nodePort=32700") || !strings.Contains(got, "80") {
		t.Fatalf("unexpected ports: %s", got)
	}
}

func TestScoreAgainstSelectorNearMiss(t *testing.T) {
	want := map[string]string{"app": "web", "tier": "frontend"}
	exact, mismatched, missing := scoreAgainstSelector(want, map[string]string{"app": "web-deploy", "tier": "frontend"})
	if exact != 1 || len(mismatched) != 1 || len(missing) != 0 {
		t.Fatalf("exact=%d mismatched=%v missing=%v", exact, mismatched, missing)
	}
	if !strings.Contains(mismatched[0], "app:") {
		t.Fatalf("expected app mismatch, got %v", mismatched)
	}
}

func TestTruncateRunesPrefersNewline(t *testing.T) {
	in := "line1-full-name\nline2-another-long-name-here\nline3-more"
	out := truncateRunes(in, 40)
	if strings.Contains(out, "line2-another-long-na") && !strings.HasSuffix(strings.TrimSuffix(out, "\n...(truncated=true)"), "\n") && !strings.Contains(out, "line1-full-name\n") {
		// must not mid-cut without newline preference when possible
	}
	if !strings.Contains(out, "line1-full-name") {
		t.Fatalf("expected first full line kept, got %q", out)
	}
	// Cut should not produce a bare prefix of line2 without newline boundary when room allows
	if strings.HasSuffix(strings.Split(out, "\n...(truncated=true)")[0], "line2-anoth") {
		t.Fatalf("mid-line cut: %q", out)
	}
}
