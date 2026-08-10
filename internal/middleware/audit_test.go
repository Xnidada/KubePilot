package middleware

import (
	"encoding/json"
	"testing"
)

func TestMaskSensitiveDataRedactsNestedFields(t *testing.T) {
	payload := map[string]interface{}{
		"username": "alice",
		"password": "secret",
		"nested": map[string]interface{}{
			"api_key": "abc",
			"items": []interface{}{
				map[string]interface{}{"kubeconfig": "cfg", "name": "demo"},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	masked := maskSensitiveData(raw, "/api/v1/clusters")
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(masked), &decoded); err != nil {
		t.Fatalf("unmarshal masked body: %v", err)
	}
	if decoded["password"] != "******" {
		t.Fatalf("password not masked: %#v", decoded["password"])
	}
	nested := decoded["nested"].(map[string]interface{})
	if nested["api_key"] != "******" {
		t.Fatalf("api_key not masked")
	}
	items := nested["items"].([]interface{})
	first := items[0].(map[string]interface{})
	if first["kubeconfig"] != "******" {
		t.Fatalf("kubeconfig not masked")
	}
	if first["name"] != "demo" {
		t.Fatalf("non-sensitive field changed")
	}
}
