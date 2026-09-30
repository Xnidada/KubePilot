package aiops

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"gorm.io/gorm"
)

type runtimeConfigKey struct{}
type runtimeConfig struct {
	client llm.Client
	config *model.LLMConfig
}

// Each request reads the committed default once. In-flight tool calls, retries
// and billing all retain that immutable snapshot, even across configuration edits.
func (s *Service) requestLLM(ctx context.Context) (context.Context, llm.Client, error) {
	if snapshot, ok := ctx.Value(runtimeConfigKey{}).(runtimeConfig); ok {
		return ctx, snapshot.client, nil
	}
	snapshot := runtimeConfig{client: s.llmClient}
	if s.db != nil {
		var cfg model.LLMConfig
		err := s.db.WithContext(ctx).Where("is_active = ?", true).First(&cfg).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return ctx, nil, fmt.Errorf("load LLM configuration: %w", err)
		}
		if err == nil {
			key, err := crypto.OpenSecret(cfg.APIKey, s.encryptKey)
			if err != nil {
				return ctx, nil, err
			}
			snapshot.client, err = llm.NewClient(&llm.LLMConfig{Provider: llm.LLMProvider(cfg.Provider), APIKey: key, BaseURL: cfg.BaseURL, Model: cfg.Model, Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, Timeout: cfg.Timeout})
			if err != nil {
				return ctx, nil, err
			}
			snapshot.config = &cfg
		}
	}
	if snapshot.client == nil {
		return ctx, nil, fmt.Errorf("LLM service not configured")
	}
	return context.WithValue(ctx, runtimeConfigKey{}, snapshot), snapshot.client, nil
}
