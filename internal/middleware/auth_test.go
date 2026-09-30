package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	"github.com/kubepilot/kubepilot/internal/testutil"
)

func TestPostgresAuthenticationRejectsRevokedSession(t *testing.T) {
	db := testutil.Postgres(t, &model.User{})
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	user := model.User{Username: "session-test", Email: "session@example.invalid", Password: "unused", Status: 1, SessionVersion: 2}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	manager := utils.NewJWTManager("test-only-key", 30*time.Minute, "test")
	for _, version := range []uint64{0, 1, 2} {
		token, err := manager.GenerateToken(user.ID, user.Username, user.RoleID, version)
		if err != nil {
			t.Fatal(err)
		}
		// Refresh must preserve revocation, never upgrade an old token's version.
		token, err = manager.RefreshToken(token)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		accepted := authenticateToken(c, manager, token)
		if accepted != (version == 2) {
			t.Fatalf("version %d accepted=%v", version, accepted)
		}
		if !accepted && w.Code != http.StatusUnauthorized {
			t.Fatalf("revoked token status=%d", w.Code)
		}
	}
}
