package aiops

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	aiopsService "github.com/kubepilot/kubepilot/internal/service/aiops"
)

func TestOnlyNamedAIViewerCanBrowseOtherUsers(t *testing.T) {
	for _, tc := range []struct {
		role model.Role
		want bool
	}{
		{model.Role{Name: "admin"}, true},
		{model.Role{Name: "aiviewer"}, true},
		{model.Role{Name: "custom-ai-readonly", Permissions: `{"aiops":["view"]}`}, false},
		{model.Role{Name: "viewer"}, false},
	} {
		if got := canBrowseEveryConversation(tc.role); got != tc.want {
			t.Errorf("role %s: got %v, want %v", tc.role.Name, got, tc.want)
		}
	}
}

func TestChatStreamRequiresClusterAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{service: &aiopsService.Service{}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Next()
	})
	router.POST("/chat/stream", h.ChatStream)
	request := httptest.NewRequest(http.MethodPost, "/chat/stream", bytes.NewBufferString(`{"message":"status","cluster_id":42}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing cluster authorization: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
