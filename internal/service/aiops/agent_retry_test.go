package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRetryAgentToolOnlyTransientFailure(t *testing.T) {
	count := 0
	result := retryAgentTool(context.Background(), "get_resource", func() toolExecResult {
		count++
		if count < 3 {
			return toolExecResult{Content: "connection reset by peer", IsError: true}
		}
		return toolExecResult{Content: "ok"}
	})
	if result.IsError || count != 3 {
		t.Fatalf("expected two retries then success, count=%d result=%+v", count, result)
	}
	count = 0
	result = retryAgentTool(context.Background(), "get_resource", func() toolExecResult {
		count++
		return toolExecResult{Content: "field spec.foo is invalid", IsError: true}
	})
	if !result.IsError || count != 1 {
		t.Fatalf("validation error must not be retried, count=%d", count)
	}
}

func TestYAMLRejectsMissingIdentity(t *testing.T) {
	_, err := parseYAMLResources(&k8s.ClusterClient{}, StagedActionParams{
		Action: "apply_yaml", Namespace: "default", YAML: "apiVersion: v1\nmetadata:\n  name: test\n",
	})
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected actionable YAML identity error, got %v", err)
	}
}

func TestYAMLServerDryRunReportsSchemaError(t *testing.T) {
	previousManager := k8s.Manager
	defer func() { k8s.Manager = previousManager }()
	var receivedDryRun bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"v1","resources":[{"name":"configmaps","singularName":"configmap","namespaced":true,"kind":"ConfigMap","verbs":["get","create","update"]}]}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/namespaces/default/configmaps":
			receivedDryRun = r.URL.Query().Get("dryRun") == "All" && r.URL.Query().Get("fieldValidation") == "Strict"
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"ConfigMap is invalid: strict decoding error: unknown field spec.badField","reason":"Invalid","code":422}`)
		default:
			t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	k8s.InitClientManager(1, 1, nil)
	kubeconfig := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
	if err := k8s.Manager.RegisterClient(4242, []byte(kubeconfig), "default"); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	_, err := s.dryRunApplyYAML(context.Background(), 4242, StagedActionParams{
		Action: "apply_yaml", Namespace: "default",
		YAML: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: bad-config\n  namespace: default\nspec:\n  badField: true\n",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") || !receivedDryRun {
		t.Fatalf("expected strict server dry-run error, got err=%v dry_run=%v", err, receivedDryRun)
	}
}

func TestObservedActionOutcomeAndSubset(t *testing.T) {
	before := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "web", "resourceVersion": "1"}, "spec": map[string]any{"replicas": int64(1)}}}
	after := before.DeepCopy()
	after.SetResourceVersion("2")
	_ = unstructured.SetNestedField(after.Object, int64(3), "spec", "replicas")
	if !actionOutcomeObserved(StagedActionParams{Action: "scale_deployment", Replicas: 3}, before, after, nil) {
		t.Fatal("scale outcome should be observed")
	}
	if unchangedResource(before, after) {
		t.Fatal("resourceVersion changed")
	}
	if !desiredFieldsMatch(map[string]any{"spec": map[string]any{"replicas": int64(3)}}, after.Object) {
		t.Fatal("desired YAML subset should match")
	}
}

func TestConfirmedWriteChecksBeforeRetry(t *testing.T) {
	for _, appliedOnError := range []bool{false, true} {
		t.Run(fmt.Sprintf("applied_on_error_%v", appliedOnError), func(t *testing.T) {
			previousManager := k8s.Manager
			defer func() { k8s.Manager = previousManager }()
			var present atomic.Bool
			var posts, gets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/v1/namespaces/default/configmaps/retry-test" &&
					r.URL.Path != "/api/v1/namespaces/default/configmaps" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				switch r.Method {
				case "GET":
					gets.Add(1)
					if !present.Load() {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","reason":"NotFound","code":404}`)
						return
					}
					fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"retry-test","namespace":"default","uid":"test-uid","resourceVersion":"2"},"data":{"a":"v"}}`)
				case "POST":
					attempt := posts.Add(1)
					if attempt == 1 {
						if appliedOnError {
							present.Store(true)
						}
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"service unavailable","reason":"ServiceUnavailable","code":503}`)
						return
					}
					present.Store(true)
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"retry-test","namespace":"default","resourceVersion":"2"},"data":{"a":"v"}}`)
				default:
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			k8s.InitClientManager(20, 20, nil)
			kubeconfig := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
			if err := k8s.Manager.RegisterClient(4243, []byte(kubeconfig), "default"); err != nil {
				t.Fatal(err)
			}
			params := StagedActionParams{Action: "create_configmap", Namespace: "default", Name: "retry-test", Data: map[string]string{"a": "v"}}
			paramJSON, _ := json.Marshal(params)
			s := &Service{}
			result, err := s.ExecuteStagedActionWithRetry(context.Background(), &model.AgentAction{ClusterID: 4243, Parameters: string(paramJSON)})
			if err != nil || result == nil || !result.Success {
				t.Fatalf("unexpected result=%+v err=%v", result, err)
			}
			wantPosts := int32(2)
			if appliedOnError {
				wantPosts = 1
			}
			if posts.Load() != wantPosts || gets.Load() < 2 {
				t.Fatalf("expected guarded retry posts=%d, got posts=%d gets=%d", wantPosts, posts.Load(), gets.Load())
			}
		})
	}
}

type interruptedStreamClient struct{ calls int }

func (c *interruptedStreamClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (c *interruptedStreamClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	c.calls++
	ch := make(chan llm.StreamChunk, 2)
	if c.calls == 1 {
		ch <- llm.StreamChunk{Content: "partial"}
		ch <- llm.StreamChunk{Error: "LLM stream ended before completion"}
	} else {
		ch <- llm.StreamChunk{Content: "complete"}
		ch <- llm.StreamChunk{Done: true}
	}
	close(ch)
	return ch, nil
}

func TestInterruptedFinalStreamDoesNotEmitPartial(t *testing.T) {
	previousManager := k8s.Manager
	defer func() { k8s.Manager = previousManager }()
	k8s.InitClientManager(1, 1, nil)
	client := &interruptedStreamClient{}
	s := &Service{llmClient: client}
	var deltas string
	out, err := s.streamFinalViaLLM(context.Background(), 1, 1, 1, "query", &agentLoopResult{
		Trace: []ToolTraceItem{{Name: "get_resource", Result: "found"}},
	}, func(ev AgentStreamEvent) {
		if ev.Type == "content_delta" {
			deltas += ev.Delta
		}
	})
	if err != nil || client.calls != 2 || out != "complete" || deltas != "complete" {
		t.Fatalf("expected complete retry without duplicate partial; calls=%d out=%q deltas=%q err=%v", client.calls, out, deltas, err)
	}
}
