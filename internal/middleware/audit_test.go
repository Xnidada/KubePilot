package middleware

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSafeBatchIDsDoesNotConsumeRequestBody(t *testing.T) {
	body := `{"ids":[12,34],"note":"private-payload"}`
	req, err := http.NewRequest(http.MethodPost, "/api/v1/backups/batch-delete", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if got := safeBatchIDs(req); got != "ids=12,34" {
		t.Fatalf("batch IDs = %q", got)
	}
	readBack, err := io.ReadAll(req.Body)
	if err != nil || string(readBack) != body {
		t.Fatalf("handler body changed: %q, %v", readBack, err)
	}
	_ = req.Body.Close()
}

func TestSafeBatchIDsSkipsOtherAndOversizedBodies(t *testing.T) {
	for _, path := range []string{"/api/v1/aiops/agent", "/api/v1/backups/batch-delete"} {
		body := strings.Repeat("x", 5000)
		req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if got := safeBatchIDs(req); got != "" {
			t.Fatalf("captured unapproved payload at %s: %q", path, got)
		}
		readBack, _ := io.ReadAll(req.Body)
		if string(readBack) != body {
			t.Fatalf("handler body changed for %s", path)
		}
		_ = req.Body.Close()
	}
}

func TestExtractResourceTypeForSystemSecurityRoutes(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/system/login-logs/:id":           "login_logs",
		"/api/v1/system/oauth/configs":            "oauth_configs",
		"/api/v1/backups/:id":                     "backup_records",
		"/api/v1/backups/batch-delete":            "backup_records",
		"/api/v1/inspection/reports/:id":          "inspection_reports",
		"/api/v1/inspection/reports/batch-delete": "inspection_reports",
	} {
		if got := extractResourceType(path); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
