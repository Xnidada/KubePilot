package aiops

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
)

func TestReadOnlyMCPToolsAndOriginProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Set("role_id", uint(1))
		c.Set("username", "tester")
		c.Set(authz.ContextAuthorizerKey, &authz.Authorizer{})
		c.Next()
	})
	router.POST("/mcp", NewReadOnlyMCPHandler(nil))

	request := func(body, origin string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		router.ServeHTTP(rec, req)
		return rec
	}
	init := request(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, "")
	if init.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", init.Code, init.Body.String())
	}
	list := request(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, "")
	if list.Code != http.StatusOK {
		t.Fatalf("tools/list status=%d body=%s", list.Code, list.Body.String())
	}
	var reply struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply.Result.Tools) != 2 || reply.Result.Tools[0].Name != "list_events" || reply.Result.Tools[1].Name != "list_workloads" {
		t.Fatalf("unexpected MCP tools: %#v", reply.Result.Tools)
	}
	for _, property := range []string{"cluster_id", "namespace", "resource_type"} {
		if _, ok := reply.Result.Tools[1].InputSchema.Properties[property]; !ok {
			t.Errorf("list_workloads schema missing %s", property)
		}
	}
	if _, ok := reply.Result.Tools[1].InputSchema.Properties["name"]; ok {
		t.Fatal("list_workloads advertises an ignored name argument")
	}
	write := request(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"stage_mutation","arguments":{}}}`, "")
	if write.Code == http.StatusOK && bytes.Contains(write.Body.Bytes(), []byte(`"result"`)) {
		t.Fatalf("write tool unexpectedly accepted: %s", write.Body.String())
	}
	denied := request(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`, "https://attacker.invalid")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("cross-origin request status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func TestReadOnlyMCPRejectsMissingPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", NewReadOnlyMCPHandler(nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing principal status=%d body=%s", rec.Code, rec.Body.String())
	}
}
