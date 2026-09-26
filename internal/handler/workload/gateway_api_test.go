package workload

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestGatewayAPIListsOnlyAuthorizedNamespaceAndSafeFields(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: gatewayAPIGroup, Version: "v1", Resource: "gateways"}
	object := func(namespace, name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": gatewayAPIGroup + "/v1", "kind": "Gateway",
			"metadata": map[string]interface{}{
				"namespace": namespace, "name": name,
				"annotations": map[string]interface{}{"private": "must-not-leak"},
			},
			"spec": map[string]interface{}{"gatewayClassName": "public"},
		}}
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "GatewayList"})
	for _, item := range []*unstructured.Unstructured{object("team-a", "gateway-a"), object("team-b", "gateway-b")} {
		if _, err := client.Resource(gvr).Namespace(item.GetNamespace()).Create(context.Background(), item, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	namespaces := gatewayQueryNamespaces("", map[string]struct{}{"team-a": {}})
	items, err := listGatewayAPIItems(context.Background(), client, gvr, "Gateway", namespaces)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Namespace != "team-a" || items[0].Name != "gateway-a" {
		t.Fatalf("namespace isolation failed: %#v", items)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-leak") {
		t.Fatalf("metadata annotation leaked: %s", encoded)
	}
	if got := gatewayQueryNamespaces("", map[string]struct{}{"team-b": {}, "team-a": {}}); !reflect.DeepEqual(got, []string{"team-a", "team-b"}) {
		t.Fatalf("unexpected namespace order: %v", got)
	}
}
