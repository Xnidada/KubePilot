package auth

import (
	"testing"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	"github.com/kubepilot/kubepilot/internal/testutil"
)

func TestPostgresPasswordChangeRevokesPendingLogin(t *testing.T) {
	db := testutil.Postgres(t, &model.User{})
	manager := utils.NewJWTManager("test-only-key", time.Hour, "test")
	service := NewService(db, manager)
	hash, err := crypto.HashPassword("old-test-password")
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "test", Email: "test@example.invalid", Password: hash, Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	before, err := service.Login(&LoginRequest{Username: user.Username, Password: "old-test-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ChangePassword(user.ID, "old-test-password", "new-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateTokenForUser(user.ID, 0); err == nil {
		t.Fatal("old pending login minted a fresh token")
	}
	if _, err := service.Login(&LoginRequest{Username: user.Username, Password: "old-test-password"}); err == nil {
		t.Fatal("old password accepted")
	}
	after, err := service.Login(&LoginRequest{Username: user.Username, Password: "new-test-password"})
	if err != nil {
		t.Fatal(err)
	}
	oldClaims, err := manager.ParseToken(before.Token)
	if err != nil {
		t.Fatal(err)
	}
	newClaims, err := manager.ParseToken(after.Token)
	if err != nil {
		t.Fatal(err)
	}
	if newClaims.SessionVersion != oldClaims.SessionVersion+1 {
		t.Fatal("password update did not increment JWT session version")
	}
}
