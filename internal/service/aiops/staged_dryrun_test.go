package aiops

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kubepilot/kubepilot/internal/k8s"
)

func TestSecretPreviewCallsKubernetesServerDryRun(t *testing.T) {
	previousManager := k8s.Manager
	defer func() { k8s.Manager = previousManager }()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/namespaces/default/secrets" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		called = r.URL.Query().Get("dryRun") == "All" && r.URL.Query().Get("fieldValidation") == "Strict"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"Secret is invalid: rejected by admission","reason":"Invalid","code":422}`)
	}))
	defer server.Close()
	k8s.InitClientManager(1, 1, nil)
	kubeconfig := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
	if err := k8s.Manager.RegisterClient(4244, []byte(kubeconfig), "default"); err != nil {
		t.Fatal(err)
	}
	_, err := (&Service{}).DryRunStagedAction(context.Background(), 4244,
		StagedActionParams{Action: "create_secret", Namespace: "default", Name: "example", Data: map[string]string{"password": "test-sensitive-value"}})
	if !called || err == nil || !strings.Contains(err.Error(), "rejected by admission") {
		t.Fatalf("preview did not enforce server dry-run: called=%v err=%v", called, err)
	}
}

func TestPVCBuilderRejectsInvalidQuantity(t *testing.T) {
	if _, err := buildPVCObject(StagedActionParams{StorageSize: "not-a-quantity"}); err == nil {
		t.Fatal("invalid quantity must not panic or reach the cluster")
	}
}
