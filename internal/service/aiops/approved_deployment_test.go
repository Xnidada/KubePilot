package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
)

func approvedDeploymentServer(t *testing.T, handler http.HandlerFunc) typedappsv1.DeploymentInterface {
	t.Helper()
	previous := k8s.Manager
	t.Cleanup(func() { k8s.Manager = previous })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	k8s.InitClientManager(1, 1, nil)
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
	if err := k8s.Manager.RegisterClient(4245, []byte(config), "default"); err != nil {
		t.Fatal(err)
	}
	client, err := k8s.Manager.GetClient(4245)
	if err != nil {
		t.Fatal(err)
	}
	client.Config.ContentType = "application/json"
	client.Config.AcceptContentTypes = "application/json"
	client.Clientset, err = kubernetes.NewForConfig(client.Config)
	if err != nil {
		t.Fatal(err)
	}
	return client.Clientset.AppsV1().Deployments("default")
}

func approvedDeploymentFixture() (*appsv1.Deployment, *model.AgentAction, StagedActionParams) {
	replicas := int32(1)
	d := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default", UID: "uid-1", Generation: 3, ResourceVersion: "10"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}
	p := StagedActionParams{Action: "scale_deployment", Name: "example", Namespace: "default", Replicas: 2}
	raw, _ := json.Marshal(p)
	a := &model.AgentAction{ClusterID: 4245, ResourceUID: "uid-1", BaseGeneration: 3, Parameters: string(raw)}
	return d, a, p
}

func TestPreviewCapturesSameDeploymentAsDryRun(t *testing.T) {
	d, _, params := approvedDeploymentFixture()
	gets, puts := 0, 0
	approvedDeploymentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			gets++
			json.NewEncoder(w).Encode(d)
		case http.MethodPut:
			puts++
			var submitted appsv1.Deployment
			if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
				t.Error(err)
			}
			if r.URL.Query().Get("dryRun") != "All" || submitted.ResourceVersion != "10" {
				t.Error("preview lost version or dry-run constraint")
			}
			json.NewEncoder(w).Encode(&submitted)
			// A subsequent snapshot GET would incorrectly approve this revision.
			d.Generation++
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	preview, uid, generation, err := (&Service{}).PreviewStagedAction(context.Background(), 4245, params)
	if err != nil || gets != 1 || puts != 1 || uid != "uid-1" || generation != 3 || !strings.Contains(preview, "spec.replicas: 1") {
		t.Fatalf("inconsistent preview: gets=%d puts=%d uid=%s generation=%d preview=%s err=%v", gets, puts, uid, generation, preview, err)
	}
}

func TestApprovedDeploymentRejectsStaleAndConflictingWrites(t *testing.T) {
	for _, stale := range []bool{true, false} {
		t.Run(fmt.Sprintf("stale=%v", stale), func(t *testing.T) {
			d, action, params := approvedDeploymentFixture()
			if stale {
				d.Generation++
			}
			gets, puts := 0, 0
			deployments := approvedDeploymentServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					gets++
					json.NewEncoder(w).Encode(d)
					return
				}
				puts++
				var submitted appsv1.Deployment
				json.NewDecoder(r.Body).Decode(&submitted)
				if submitted.ResourceVersion != "10" {
					t.Error("write does not use checked resourceVersion")
				}
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","message":"object has been modified","code":409}`)
			})
			_, _, err := executeApprovedDeployment(context.Background(), deployments, action, params)
			wantPuts := 1
			if stale {
				wantPuts = 0
			}
			if err == nil || gets != 1 || puts != wantPuts {
				t.Fatalf("stale write rebased/retried: gets=%d puts=%d err=%v", gets, puts, err)
			}
		})
	}
}

func TestObservedDeploymentSurvivesRequestCancellation(t *testing.T) {
	d, action, _ := approvedDeploymentFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gets, puts, deletes := 0, 0, 0
	approvedDeploymentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			gets++
			json.NewEncoder(w).Encode(d)
		case http.MethodPut:
			puts++
			json.NewDecoder(r.Body).Decode(d)
			d.Generation++
			d.ResourceVersion = "11"
			d.Status = appsv1.DeploymentStatus{ObservedGeneration: d.Generation, Replicas: 2, UpdatedReplicas: 2, ReadyReplicas: 2, AvailableReplicas: 2}
			cancel() // HTTP client disconnects after the API accepts the change.
			json.NewEncoder(w).Encode(d)
		case http.MethodDelete:
			deletes++
			t.Error("request cancellation triggered rollback")
		}
	})
	result, observation, rollback, err := (&Service{}).ExecuteObservedDeploymentChange(ctx, action)
	if err != nil || result == nil || !result.Success || !strings.HasPrefix(observation, "ready:") || rollback != "" || puts != 1 || gets != 2 || deletes != 0 {
		t.Fatalf("result=%+v observation=%q rollback=%q gets=%d puts=%d deletes=%d err=%v", result, observation, rollback, gets, puts, deletes, err)
	}
}

func TestApprovedDeploymentChecksOutcomeBeforeRetry(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(fmt.Sprintf("applied=%v", applied), func(t *testing.T) {
			d, action, params := approvedDeploymentFixture()
			puts := 0
			deployments := approvedDeploymentServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					json.NewEncoder(w).Encode(d)
					return
				}
				puts++
				var submitted appsv1.Deployment
				json.NewDecoder(r.Body).Decode(&submitted)
				if applied || puts > 1 {
					d = &submitted
					d.Generation++
					d.ResourceVersion = "11"
				}
				if puts == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"ServiceUnavailable","message":"service unavailable","code":503}`)
					return
				}
				json.NewEncoder(w).Encode(d)
			})
			_, after, err := executeApprovedDeployment(context.Background(), deployments, action, params)
			wantPuts := 2
			if applied {
				wantPuts = 1
			}
			if err != nil || after == nil || after.Generation != 4 || puts != wantPuts {
				t.Fatalf("duplicate or missing write: puts=%d after=%+v err=%v", puts, after, err)
			}
		})
	}
}
