package aiops

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
)

// persistAgentToolTrace stores one agent round's tool invocations for observability.
func (s *Service) persistAgentToolTrace(userID, clusterID, conversationID uint, userMsg string, trace []ToolTraceItem, pending []PendingActionInfo) {
	if s.db == nil || len(trace) == 0 {
		return
	}
	type row struct {
		Name       string `json:"name"`
		Args       string `json:"args"`
		Result     string `json:"result"`
		IsError    bool   `json:"is_error"`
		DurationMs int64  `json:"duration_ms,omitempty"`
	}
	rows := make([]row, 0, len(trace))
	for _, t := range trace {
		rows = append(rows, row{
			Name: t.Name, Args: t.Args, Result: truncateRunes(t.Result, 2000),
			IsError: t.IsError, DurationMs: t.DurationMs,
		})
	}
	pendingIDs := make([]uint, 0, len(pending))
	for _, p := range pending {
		pendingIDs = append(pendingIDs, p.ID)
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"tools":        rows,
		"pending_ids":  pendingIDs,
		"user_message": truncateRunes(userMsg, 500),
	})
	rec := model.AgentToolTrace{
		UserID:         userID,
		ClusterID:      clusterID,
		ConversationID: conversationID,
		Payload:        string(payload),
		ToolCount:      len(trace),
		CreatedAt:      time.Now(),
	}
	_ = s.db.WithContext(context.Background()).Create(&rec).Error
}

// persistTokenUsage records token consumption for an AI interaction.
func (s *Service) persistTokenUsage(userID, conversationID uint, usage llm.Usage, chatType string) {
	if s.db == nil || usage.TotalTokens == 0 {
		return
	}
	// Look up active LLM config ID
	var llmConfigID uint
	var cfg model.LLMConfig
	if err := s.db.Where("is_active = ?", true).First(&cfg).Error; err == nil {
		llmConfigID = cfg.ID
	}
	rec := model.TokenUsageLog{
		UserID:           userID,
		ConversationID:   conversationID,
		LLMConfigID:      llmConfigID,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		ChatType:         chatType,
		CreatedAt:        time.Now(),
	}
	_ = s.db.WithContext(context.Background()).Create(&rec).Error
}

// TokenUsageStats is the response for the token usage stats API.
type TokenUsageStats struct {
	TotalTokens          int                `json:"total_tokens"`
	TotalPromptTokens    int                `json:"total_prompt_tokens"`
	TotalCompletionTokens int               `json:"total_completion_tokens"`
	TotalCostEstimate    float64            `json:"total_cost_estimate"`
	ByDay                []TokenUsageByDay  `json:"by_day"`
	ByModel              []TokenUsageByModel `json:"by_model"`
	ByUser               []TokenUsageByUser  `json:"by_user"`
	ByType               []TokenUsageByType  `json:"by_type"`
}

type TokenUsageByDay struct {
	Date             string `json:"date"`
	TotalTokens      int    `json:"total_tokens"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

type TokenUsageByModel struct {
	Model       string `json:"model"`
	TotalTokens int    `json:"total_tokens"`
}

type TokenUsageByUser struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	TotalTokens int    `json:"total_tokens"`
}

type TokenUsageByType struct {
	ChatType    string `json:"chat_type"`
	TotalTokens int    `json:"total_tokens"`
}

// GetTokenUsageStats returns aggregated token usage statistics.
func (s *Service) GetTokenUsageStats(days int) (*TokenUsageStats, error) {
	if s.db == nil {
		return nil, nil
	}
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	base := s.db.Model(&model.TokenUsageLog{}).Where("created_at >= ?", since)

	// Totals
	var totals struct {
		TotalTokens      int
		PromptTokens     int
		CompletionTokens int
	}
	base.Select("COALESCE(SUM(total_tokens),0) as total_tokens, COALESCE(SUM(prompt_tokens),0) as prompt_tokens, COALESCE(SUM(completion_tokens),0) as completion_tokens").Scan(&totals)

	// Cost estimate: GPT-4o pricing $2.5/1M input, $10/1M output (rough average)
	costEstimate := float64(totals.PromptTokens)*2.5/1_000_000 + float64(totals.CompletionTokens)*10.0/1_000_000

	// By day
	var byDay []TokenUsageByDay
	s.db.Model(&model.TokenUsageLog{}).
		Select("DATE(created_at) as date, SUM(total_tokens) as total_tokens, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").Find(&byDay)

	// By model (join with llm_configs)
	var byModel []TokenUsageByModel
	s.db.Model(&model.TokenUsageLog{}).
		Select("COALESCE(c.model, 'unknown') as model, SUM(t.total_tokens) as total_tokens").
		Joins("LEFT JOIN llm_configs c ON t.llm_config_id = c.id").
		Where("t.created_at >= ?", since).
		Group("c.model").Order("total_tokens DESC").Find(&byModel)

	// By user (join with users)
	var byUser []TokenUsageByUser
	s.db.Model(&model.TokenUsageLog{}).
		Select("t.user_id, COALESCE(u.username, 'unknown') as username, SUM(t.total_tokens) as total_tokens").
		Joins("LEFT JOIN users u ON t.user_id = u.id").
		Where("t.created_at >= ?", since).
		Group("t.user_id, u.username").Order("total_tokens DESC").Limit(20).Find(&byUser)

	// By type
	var byType []TokenUsageByType
	s.db.Model(&model.TokenUsageLog{}).
		Select("chat_type, SUM(total_tokens) as total_tokens").
		Where("created_at >= ?", since).
		Group("chat_type").Order("total_tokens DESC").Find(&byType)

	return &TokenUsageStats{
		TotalTokens:          totals.TotalTokens,
		TotalPromptTokens:    totals.PromptTokens,
		TotalCompletionTokens: totals.CompletionTokens,
		TotalCostEstimate:    costEstimate,
		ByDay:                byDay,
		ByModel:              byModel,
		ByUser:               byUser,
		ByType:               byType,
	}, nil
}

// GetTokenUsageRecent returns the most recent token usage log entries.
func (s *Service) GetTokenUsageRecent(limit int) ([]model.TokenUsageLog, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var logs []model.TokenUsageLog
	s.db.Order("created_at DESC").Limit(limit).Find(&logs)
	return logs, nil
}
