package aiops

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"gorm.io/gorm"
)

func (s *Service) sealActionParameters(raw string) (string, error) {
	if s.encryptKey == "" {
		return "", fmt.Errorf("action encryption key is not configured")
	}
	return crypto.SealSecret(raw, s.encryptKey)
}

func (s *Service) openActionParameters(raw string) ([]byte, error) {
	opened, err := crypto.OpenSecret(raw, s.encryptKey)
	return []byte(opened), err
}

func safeAgentTraceArgs(name, args string) string {
	if toolIsWrite(name) || name == "propose_mutation" {
		return "[变更参数已隐藏，请查看待确认动作的预览]"
	}
	limit := 500
	if isRetryableQueryTool(name) {
		limit = 8192
	}
	return truncateRunes(args, limit)
}

func safeAgentTraceResult(name, result string, isError bool) string {
	if isError && (toolIsWrite(name) || name == "propose_mutation") {
		return "变更预览失败；原始错误未写入工具轨迹，以免泄露参数。请查看 Agent 的脱敏说明。"
	}
	return truncateRunes(result, toolResultMaxChars)
}

// MigrateAgentActionSecrets seals old action payloads and removes historical
// mutation arguments from the two Agent trace stores before HTTP starts.
func MigrateAgentActionSecrets(db *gorm.DB, key string) error {
	if key == "" {
		return fmt.Errorf("action encryption key is not configured")
	}
	var actions []model.AgentAction
	if err := db.Select("id", "parameters").Where("parameters <> '' AND parameters NOT LIKE ?", "enc:v1:%").
		FindInBatches(&actions, 100, func(_ *gorm.DB, _ int) error {
			for _, action := range actions {
				sealed, err := crypto.SealSecret(action.Parameters, key)
				if err != nil {
					return err
				}
				if err := db.Model(&model.AgentAction{}).Where("id = ? AND parameters = ?", action.ID, action.Parameters).
					UpdateColumn("parameters", sealed).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error; err != nil {
		return fmt.Errorf("migrate agent actions: %w", err)
	}

	var messages []model.ChatMessage
	if err := db.Select("id", "extras").Where("extras <> ''").FindInBatches(&messages, 100, func(_ *gorm.DB, _ int) error {
		for _, message := range messages {
			var extras MessageExtras
			if json.Unmarshal([]byte(message.Extras), &extras) != nil || len(extras.ToolTrace) == 0 {
				continue
			}
			changed := false
			for i := range extras.ToolTrace {
				item := &extras.ToolTrace[i]
				args := safeAgentTraceArgs(item.Name, item.Args)
				result := safeAgentTraceResult(item.Name, item.Result, item.IsError)
				if item.Args != args || item.Result != result {
					item.Args, item.Result = args, result
					changed = true
				}
			}
			if !changed {
				continue
			}
			payload, err := json.Marshal(extras)
			if err != nil {
				return err
			}
			if err := db.Model(&model.ChatMessage{}).Where("id = ? AND extras = ?", message.ID, message.Extras).
				UpdateColumn("extras", string(payload)).Error; err != nil {
				return err
			}
		}
		return nil
	}).Error; err != nil {
		return fmt.Errorf("migrate agent message traces: %w", err)
	}

	var traces []model.AgentToolTrace
	if err := db.Select("id", "payload").FindInBatches(&traces, 100, func(_ *gorm.DB, _ int) error {
		for _, trace := range traces {
			var payload struct {
				Tools      []ToolTraceItem `json:"tools"`
				PendingIDs []uint          `json:"pending_ids"`
			}
			if json.Unmarshal([]byte(trace.Payload), &payload) != nil {
				continue
			}
			for i := range payload.Tools {
				item := &payload.Tools[i]
				item.Args = safeAgentTraceArgs(item.Name, item.Args)
				item.Result = safeAgentTraceResult(item.Name, item.Result, item.IsError)
			}
			updated, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if strings.TrimSpace(trace.Payload) == string(updated) {
				continue
			}
			if err := db.Model(&model.AgentToolTrace{}).Where("id = ? AND payload = ?", trace.ID, trace.Payload).
				UpdateColumn("payload", string(updated)).Error; err != nil {
				return err
			}
		}
		return nil
	}).Error; err != nil {
		return fmt.Errorf("migrate agent tool traces: %w", err)
	}
	return nil
}
