package aiops

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	service "github.com/kubepilot/kubepilot/internal/service/aiops"
	"github.com/kubepilot/kubepilot/internal/testutil"
	"gorm.io/gorm"
)

func TestLLMAndActionRoutesRejectSQLConditions(t *testing.T) {
	h := &Handler{service: &service.Service{}}
	for name, handler := range map[string]gin.HandlerFunc{
		"get": h.GetLLMConfigByID, "update": h.UpdateLLMConfig, "delete": h.DeleteLLMConfig,
		"default": h.SetDefaultLLMConfig, "confirm": h.AgentConfirmAction,
	} {
		for _, id := range []string{"0", "-1", "id=1 OR 1=1", "1;SELECT 1", "4294967296"} {
			t.Run(name+"/"+id, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				c.Params = gin.Params{{Key: "id", Value: id}, {Key: "actionId", Value: id}}
				handler(c) // nil DB makes any query before validation fail the test.
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestPostgresPinCannotResurrectForgottenMemory(t *testing.T) {
	db := testutil.Postgres(t, &model.AgentMemory{}, &model.AgentMemoryAudit{})
	memory := model.AgentMemory{UserID: 7, Type: "preference", Content: "concise answers"}
	if err := db.Create(&memory).Error; err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	if err := tx.Delete(&memory).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("user_id", uint(7))
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(memory.ID)}}
	done := make(chan struct{})
	go func() { defer close(done); (&Handler{db: db}).PinMemory(c) }()
	testutil.WaitForRowLock(t, db)
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	<-done
	if w.Code != http.StatusNotFound {
		t.Fatalf("pin raced past forgetting: %d %s", w.Code, w.Body.String())
	}
	var stored model.AgentMemory
	if err := db.Unscoped().First(&stored, memory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.DeletedAt.Valid || stored.IsPinned {
		t.Fatal("forgotten memory was restored")
	}
	var count int64
	if err := db.Model(&model.AgentMemoryAudit{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed pin wrote audit: %d %v", count, err)
	}
}

func TestPostgresClearInvalidatesAndSerializesAgentWrites(t *testing.T) {
	db := testutil.Postgres(t, &model.ChatConversation{}, &model.ChatMessage{}, &model.ConversationState{}, &model.AgentAction{}, &model.AgentActionAudit{}, &model.AgentToolTrace{})
	conversation := model.ChatConversation{UserID: 7, Title: "test"}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	ctx := service.WithConversationVersion(context.Background(), &conversation)
	entered, release := make(chan struct{}), make(chan struct{})
	writeDone, clearDone := make(chan error, 1), make(chan error, 1)
	go func() {
		writeDone <- service.WithConversationWrite(ctx, db, conversation.ID, func(tx *gorm.DB) error {
			close(entered)
			<-release
			return tx.Create(&model.ChatMessage{ConversationID: conversation.ID, Role: "assistant", Content: "old round"}).Error
		})
	}()
	<-entered
	go func() {
		clearDone <- db.Transaction(func(tx *gorm.DB) error { return clearConversationContent(tx, conversation.ID, 7) })
	}()
	testutil.WaitForRowLock(t, db)
	close(release)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-clearDone; err != nil {
		t.Fatal(err)
	}
	called := false
	if err := service.WithConversationWrite(ctx, db, conversation.ID, func(*gorm.DB) error { called = true; return nil }); err == nil || called {
		t.Fatal("old turn persisted after clearing")
	}
	var count int64
	if err := db.Model(&model.ChatMessage{}).Where("conversation_id = ?", conversation.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("clear left messages: %d %v", count, err)
	}
	if err := db.First(&conversation, conversation.ID).Error; err != nil {
		t.Fatal(err)
	}
	newCtx := service.WithConversationVersion(context.Background(), &conversation)
	if err := service.WithConversationWrite(newCtx, db, conversation.ID, func(tx *gorm.DB) error {
		return tx.Create(&model.ChatMessage{ConversationID: conversation.ID, Role: "assistant", Content: "new round"}).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.WithConversationWrite(newCtx, db, conversation.ID, func(*gorm.DB) error { t.Error("deleted conversation accepted a write"); return nil }); err == nil {
		t.Fatal("deleted conversation remained valid")
	}
}
