package aiops

import (
	"context"
	"encoding/json"
	"time"

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
