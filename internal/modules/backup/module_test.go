package backup

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/module"
)

func TestBackupDeletionRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	New().RegisterRoutes(&module.Context{Host: &module.Host{}}, router.Group("/api/v1"))

	want := map[string]bool{
		"DELETE /api/v1/backups/:id":        false,
		"POST /api/v1/backups/batch-delete": false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, registered := range want {
		if !registered {
			t.Errorf("missing route %s", key)
		}
	}
}
