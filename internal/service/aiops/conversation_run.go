package aiops

import (
	"context"
	"fmt"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type conversationRunKey struct{}
type conversationRun struct {
	ID, UserID uint
	Version    uint64
}

func WithConversationVersion(ctx context.Context, conversation *model.ChatConversation) context.Context {
	return context.WithValue(ctx, conversationRunKey{}, conversationRun{conversation.ID, conversation.UserID, conversation.ContextVersion})
}

func (s *Service) captureConversationRun(ctx context.Context, userID, conversationID uint) (context.Context, error) {
	if conversationID == 0 {
		return ctx, nil
	}
	if run, ok := ctx.Value(conversationRunKey{}).(conversationRun); ok && run.ID == conversationID && run.UserID == userID {
		return ctx, nil
	}
	var conversation model.ChatConversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conversation).Error; err != nil {
		return ctx, err
	}
	return WithConversationVersion(ctx, &conversation), nil
}

// Share the conversation row lock with clear/delete. Capture the version before
// LLM/tool work and never refresh it at persistence time.
func WithConversationWrite(ctx context.Context, db *gorm.DB, conversationID uint, fn func(*gorm.DB) error) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return db.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		if conversationID > 0 {
			run, ok := ctx.Value(conversationRunKey{}).(conversationRun)
			if !ok || run.ID != conversationID {
				return fmt.Errorf("conversation run version missing")
			}
			var conversation model.ChatConversation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", conversationID, run.UserID).First(&conversation).Error; err != nil {
				return err
			}
			if conversation.ContextVersion != run.Version {
				return fmt.Errorf("conversation was cleared; this turn is no longer valid")
			}
		}
		return fn(tx)
	})
}
