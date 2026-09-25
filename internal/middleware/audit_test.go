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

func TestMaskSensitiveDataRedactsOAuthClientSecret(t *testing.T) {
	masked := maskSensitiveData([]byte(`{"provider":"github","client_secret":"do-not-log"}`), "/api/v1/system/oauth/configs")
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(masked), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["client_secret"] != "******" || decoded["provider"] != "github" {
		t.Fatalf("unexpected masked OAuth config: %#v", decoded)
	}
	if got := maskSensitiveData([]byte(`not-json`), "/api/v1/system/oauth/configs"); got != "[masked]" {
		t.Fatalf("invalid secret payload must be fully masked, got %q", got)
	}
}

func TestExtractResourceTypeForSystemSecurityRoutes(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/system/login-logs/:id": "login_logs",
		"/api/v1/system/oauth/configs":  "oauth_configs",
	} {
		if got := extractResourceType(path); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
