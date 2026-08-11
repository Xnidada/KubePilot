package aiops

import (
	"fmt"
	"strings"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
)

const (
	agentMemoryFetchLimit   = 20
	agentMemoryRecentTurns  = 3 // user+assistant pairs roughly
	agentMemoryUserMaxRunes = 400
	agentMemoryAsstMaxRunes = 600
	agentMemoryBulletMax    = 8
	agentMemoryBulletRunes  = 180
)

// buildAgentMemoryMessages builds layered context for the tool-calling agent:
// episodic summary (older turns) + short recent working turns.
// Does not include the current user message (caller appends it).
func (s *Service) buildAgentMemoryMessages(conversationID uint, currentUserMsg string) []llm.Message {
	if conversationID == 0 {
		return nil
	}

	var rows []model.ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(agentMemoryFetchLimit).
		Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil
	}

	// oldest → newest
	msgs := make([]model.ChatMessage, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		msgs = append(msgs, rows[i])
	}

	// Drop duplicate trailing user message already saved by frontend.
	if len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		if last.Role == "user" && strings.TrimSpace(last.Content) == strings.TrimSpace(currentUserMsg) {
			msgs = msgs[:len(msgs)-1]
		}
	}
	if len(msgs) == 0 {
		return nil
	}

	cleaned := make([]model.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		c := strings.TrimSpace(sanitizeAgentHistory(m.Content))
		if c == "" || isAgentHistoryNoise(c) {
			continue
		}
		m.Content = c
		cleaned = append(cleaned, m)
	}
	if len(cleaned) == 0 {
		return nil
	}

	// Split: keep last ~agentMemoryRecentTurns*2 messages as working; rest → episodic.
	keepN := agentMemoryRecentTurns * 2
	if keepN > len(cleaned) {
		keepN = len(cleaned)
	}
	cut := len(cleaned) - keepN
	older := cleaned[:cut]
	recent := cleaned[cut:]

	out := make([]llm.Message, 0, len(recent)+1)
	if summary := buildEpisodicSummary(older); summary != "" {
		out = append(out, llm.Message{
			Role:    "user",
			Content: "【会话摘要·仅供参考，细节仍须用工具核实】\n" + summary,
		})
		out = append(out, llm.Message{
			Role:    "assistant",
			Content: "已了解会话摘要。涉及集群现状时我会先调用工具查询。",
		})
	}

	for _, m := range recent {
		max := agentMemoryAsstMaxRunes
		if m.Role == "user" {
			max = agentMemoryUserMaxRunes
		}
		out = append(out, llm.Message{
			Role:    m.Role,
			Content: truncateRunes(stripDryRunNoise(m.Content), max),
		})
	}
	return out
}

func isAgentHistoryNoise(content string) bool {
	t := strings.TrimSpace(content)
	switch t {
	case "确认执行", "请求 dry-run 预览", "[用户确认相关消息已省略]", "❌ 操作已取消":
		return true
	}
	if strings.HasPrefix(t, "✅ ") && (strings.Contains(t, "已删除") || strings.Contains(t, "已创建") || strings.Contains(t, "已扩")) {
		// Keep short success lines in episodic via buildEpisodicSummary; for working memory they're ok.
		// Not pure noise.
		return false
	}
	return false
}

func stripDryRunNoise(content string) string {
	// Drop long dry-run / pending blocks from assistant turns.
	if i := strings.Index(content, "待确认写操作"); i >= 0 {
		content = strings.TrimSpace(content[:i])
	}
	if i := strings.Index(content, "[dry-run]"); i >= 0 && i > 80 {
		content = strings.TrimSpace(content[:i])
	}
	return content
}

func buildEpisodicSummary(older []model.ChatMessage) string {
	if len(older) == 0 {
		return ""
	}
	bullets := make([]string, 0, agentMemoryBulletMax)
	for _, m := range older {
		if len(bullets) >= agentMemoryBulletMax {
			break
		}
		c := stripDryRunNoise(m.Content)
		c = strings.TrimSpace(c)
		if c == "" || isAgentHistoryNoise(c) {
			continue
		}
		prefix := "用户"
		if m.Role == "assistant" {
			prefix = "结论"
			// Prefer first line of assistant reply
			if idx := strings.IndexAny(c, "\n。"); idx > 0 && idx < 120 {
				c = c[:idx]
			}
		} else {
			prefix = "目标"
		}
		bullets = append(bullets, fmt.Sprintf("- %s：%s", prefix, truncateRunes(c, agentMemoryBulletRunes)))
	}
	if len(bullets) == 0 {
		return ""
	}
	return strings.Join(bullets, "\n")
}
