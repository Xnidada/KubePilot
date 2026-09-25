package aiops

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
)

func TestValidMemoryContentRejectsCredentials(t *testing.T) {
	for _, content := range []string{
		"API_KEY=abc123", "密码是 dummy-example", "Authorization: Bearer abc123",
		"-----BEGIN PRIVATE KEY-----", "ghp_12345678901234567890", strings.Repeat("a", 801),
	} {
		if ValidMemoryContent(content) {
			t.Errorf("accepted sensitive memory %q", content)
		}
	}
	if !ValidMemoryContent("偏好：以后回答使用中文") {
		t.Fatal("rejected ordinary preference")
	}
}

func TestMemorySearchTermsFindsChineseBigrams(t *testing.T) {
	terms := memorySearchTerms("查看生产集群的 nginx Deployment")
	for _, want := range []string{"生产", "产集", "集群", "nginx", "deployment"} {
		if !slices.Contains(terms, want) {
			t.Errorf("missing search term %q from %#v", want, terms)
		}
	}
	if got := escapeLike("a_b%c"); got != `a\_b\%c` {
		t.Errorf("LIKE escape = %q", got)
	}
}

func TestVerifiedToolFactStoresOnlyStatus(t *testing.T) {
	trace := ToolTraceItem{
		Name: "get_resource", Args: `{"resource_type":"pod","namespace":"default","name":"nginx"}`,
		Result: `{"metadata":{"name":"nginx","namespace":"default"},"status":{"phase":"Running"},"spec":{"containers":[{"env":[{"name":"PASSWORD","value":"secret"}]}]}}`,
	}
	fact, ok := verifiedToolFact(trace)
	if !ok || fact.Text != "default/nginx Pod 阶段=Running" || strings.Contains(fact.Text, "secret") || fact.Source != "tool:get_resource:status_v1" {
		t.Fatalf("unexpected fact: %#v, ok=%v", fact, ok)
	}
	if !fact.ExpiresAt.After(time.Now()) {
		t.Fatal("fact TTL is not in the future")
	}
	trace.Name = "get_pod_logs"
	if _, ok := verifiedToolFact(trace); ok {
		t.Fatal("promoted raw logs to verified facts")
	}
}

func TestMemoryRelevancePrefersChineseMatch(t *testing.T) {
	terms := memorySearchTerms("生产集群")
	matched := model.AgentMemory{Content: "生产集群默认命名空间", Confidence: 0.8}
	unmatched := model.AgentMemory{Content: "测试环境", Confidence: 0.8}
	if memoryRelevance(matched, terms) <= memoryRelevance(unmatched, terms) {
		t.Fatal("matching Chinese memory did not rank higher")
	}
}

func TestActiveFactsRejectsLegacyRawToolOutput(t *testing.T) {
	until := time.Now().Add(time.Minute).Format(time.RFC3339)
	legacy := `[{"text":"secret payload","source":"tool:get_resource","confidence":0.9,"expires_at":"` + until + `"}]`
	if got := activeFacts(legacy); got != "无" {
		t.Fatalf("legacy raw fact was replayed: %q", got)
	}
}
