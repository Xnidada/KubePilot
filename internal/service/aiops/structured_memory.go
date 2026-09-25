package aiops

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
)

type memoryFact struct {
	Text       string    `json:"text"`
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// buildStructuredMemoryMessages replaces the old free-text episodic summary
// with durable state, retrieved memory, and a small recent working window.
func (s *Service) buildStructuredMemoryMessages(userID, clusterID, conversationID uint, current string) ([]llm.Message, int) {
	if conversationID == 0 {
		return nil, 0
	}
	out := make([]llm.Message, 0, 16)
	var state model.ConversationState
	if err := s.db.Where("conversation_id = ?", conversationID).First(&state).Error; err == nil {
		facts := activeFacts(state.Facts)
		body := fmt.Sprintf("【结构化会话状态·历史事实可能过期，集群现状必须调用工具核实】\n当前目标：%s\n涉及对象：%s\n已验证事实：%s\n未完成事项：%s",
			state.Goal, defaultText(state.Entities, "无"), facts, defaultText(state.OpenItems, "无"))
		out = append(out, llm.Message{Role: "user", Content: truncateRunes(body, 1800)})
		out = append(out, llm.Message{Role: "assistant", Content: "已加载结构化状态；我会先核实有时效的集群事实。"})
	}

	memories := s.retrieveMemories(userID, clusterID, current, 4)
	if len(memories) > 0 {
		lines := make([]string, 0, len(memories))
		for _, m := range memories {
			lines = append(lines, fmt.Sprintf("- [%s, 置信度 %.0f%%] %s", m.Type, m.Confidence*100, m.Content))
		}
		out = append(out, llm.Message{Role: "user", Content: "【检索到的长期记忆·仅作辅助】\n" + strings.Join(lines, "\n")})
		out = append(out, llm.Message{Role: "assistant", Content: "我会仅将这些记忆作为偏好或历史知识使用。"})
	}

	var rows []model.ChatMessage
	_ = s.db.Where("conversation_id = ?", conversationID).Order("created_at DESC").Limit(12).Find(&rows).Error
	for i := len(rows) - 1; i >= 0; i-- {
		m := rows[i]
		if m.Role == "user" && strings.TrimSpace(m.Content) == strings.TrimSpace(current) && i == len(rows)-1 {
			continue
		}
		content := strings.TrimSpace(sanitizeAgentHistory(m.Content))
		if content == "" || isAgentHistoryNoise(content) {
			continue
		}
		limit := agentMemoryAsstMaxRunes
		if m.Role == "user" {
			limit = agentMemoryUserMaxRunes
		}
		out = append(out, llm.Message{Role: m.Role, Content: truncateRunes(stripDryRunNoise(content), limit)})
	}
	return out, len(memories)
}

func (s *Service) retrieveMemories(userID, clusterID uint, query string, limit int) []model.AgentMemory {
	q := s.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now())
	q = q.Where("cluster_id IS NULL OR cluster_id = ?", clusterID)
	terms := strings.Fields(strings.ToLower(query))
	for _, term := range terms[:minInt(len(terms), 4)] {
		if len([]rune(term)) >= 2 {
			q = q.Where("LOWER(content) LIKE ?", "%"+term+"%")
		}
	}
	var rows []model.AgentMemory
	_ = q.Order("is_pinned DESC, updated_at DESC").Limit(limit).Find(&rows).Error
	return rows
}

func (s *Service) refreshConversationState(userID, clusterID, conversationID uint, userMsg, answer string, trace []ToolTraceItem, pending []PendingActionInfo) {
	if conversationID == 0 || s.db == nil {
		return
	}
	facts := make([]memoryFact, 0, minInt(len(trace), 3))
	for _, t := range trace {
		if t.IsError || strings.TrimSpace(t.Result) == "" {
			continue
		}
		facts = append(facts, memoryFact{Text: truncateRunes(strings.TrimSpace(t.Result), 300), Source: "tool:" + t.Name, Confidence: 0.9, ExpiresAt: time.Now().Add(10 * time.Minute)})
		if len(facts) == 3 {
			break
		}
	}
	factsJSON, _ := json.Marshal(facts)
	open := "无"
	if len(pending) > 0 {
		open = fmt.Sprintf("有 %d 个写操作等待确认", len(pending))
	}
	state := model.ConversationState{ConversationID: conversationID, UserID: userID, ClusterID: clusterID, Goal: truncateRunes(userMsg, 500), Facts: string(factsJSON), OpenItems: open, UpdatedAt: time.Now()}
	_ = s.db.Where("conversation_id = ?", conversationID).Assign(state).FirstOrCreate(&state).Error
	// Only explicit user preferences and tool-backed conclusions become long-term memory.
	if looksLikePreference(userMsg) {
		s.storeMemory(userID, nil, "preference", userMsg, "conversation:"+fmt.Sprint(conversationID), 0.8, nil)
	}
	if len(facts) > 0 && len([]rune(answer)) > 0 {
		cluster := clusterID
		s.storeMemory(userID, &cluster, "verified_knowledge", truncateRunes(answer, 500), facts[0].Source, 0.7, ptrTime(time.Now().Add(30*24*time.Hour)))
	}
}

func (s *Service) storeMemory(userID uint, clusterID *uint, typ, content, source string, confidence float64, expiresAt *time.Time) {
	content = strings.TrimSpace(content)
	if content == "" || strings.Contains(strings.ToLower(content), "api_key") || strings.Contains(strings.ToLower(content), "kubeconfig") {
		return
	}
	var existing model.AgentMemory
	q := s.db.Where("user_id = ? AND type = ? AND content = ?", userID, typ, content)
	if clusterID == nil {
		q = q.Where("cluster_id IS NULL")
	} else {
		q = q.Where("cluster_id = ?", *clusterID)
	}
	if q.First(&existing).Error == nil {
		return
	}
	m := model.AgentMemory{UserID: userID, ClusterID: clusterID, Type: typ, Content: truncateRunes(content, 800), Source: source, Confidence: confidence, ExpiresAt: expiresAt}
	if s.db.Create(&m).Error == nil {
		_ = s.db.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: userID, Action: "create", Detail: "automatic"}).Error
	}
}

func activeFacts(raw string) string {
	var facts []memoryFact
	if json.Unmarshal([]byte(raw), &facts) != nil {
		return "无"
	}
	parts := make([]string, 0, len(facts))
	now := time.Now()
	for _, f := range facts {
		if f.ExpiresAt.After(now) {
			parts = append(parts, f.Text)
		}
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, "；")
}
func looksLikePreference(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(s, "以后") || strings.Contains(s, "默认") || strings.Contains(s, "偏好") || strings.Contains(l, "prefer")
}
func defaultText(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
func ptrTime(t time.Time) *time.Time { return &t }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) persistAgentRunMetric(userID, clusterID, conversationID uint, userMsg string, usage llm.Usage, trace []ToolTraceItem, started time.Time, memoryHits int) {
	if s.db == nil {
		return
	}
	seen := map[string]bool{}
	repeated := 0
	for _, t := range trace {
		key := t.Name + ":" + t.Args
		if seen[key] {
			repeated++
		}
		seen[key] = true
	}
	lower := strings.ToLower(userMsg)
	correction := strings.Contains(userMsg, "不对") || strings.Contains(userMsg, "纠正") || strings.Contains(lower, "incorrect")
	rec := model.AgentRunMetric{UserID: userID, ClusterID: clusterID, ConversationID: conversationID, PromptTokens: usage.PromptTokens, ToolCount: len(trace), RepeatedToolCalls: repeated, MemoryHits: memoryHits, UserCorrection: correction, LatencyMs: time.Since(started).Milliseconds(), CreatedAt: time.Now()}
	_ = s.db.Create(&rec).Error
}

type MemoryMetricSummary struct {
	Runs                 int64   `json:"runs"`
	AvgPromptTokens      float64 `json:"avg_prompt_tokens"`
	RepeatedToolCallRate float64 `json:"repeated_tool_call_rate"`
	ContextHitRate       float64 `json:"context_hit_rate"`
	ErrorAssertionRate   float64 `json:"error_assertion_rate"`
	UserCorrectionRate   float64 `json:"user_correction_rate"`
	AvgLatencyMs         float64 `json:"avg_latency_ms"`
}

func (s *Service) GetMemoryMetricSummary(days int) (*MemoryMetricSummary, error) {
	if days <= 0 {
		days = 30
	}
	var row struct {
		Runs                                                                                                        int64
		AvgPromptTokens, RepeatedToolCallRate, ContextHitRate, ErrorAssertionRate, UserCorrectionRate, AvgLatencyMs float64
	}
	err := s.db.Model(&model.AgentRunMetric{}).Where("created_at >= ?", time.Now().AddDate(0, 0, -days)).Select(`COUNT(*) AS runs, COALESCE(AVG(prompt_tokens),0) AS avg_prompt_tokens, COALESCE(AVG(CASE WHEN repeated_tool_calls > 0 THEN 1.0 ELSE 0.0 END),0) AS repeated_tool_call_rate, COALESCE(AVG(CASE WHEN memory_hits > 0 THEN 1.0 ELSE 0.0 END),0) AS context_hit_rate, COALESCE(AVG(CASE WHEN error_assertion THEN 1.0 ELSE 0.0 END),0) AS error_assertion_rate, COALESCE(AVG(CASE WHEN user_correction THEN 1.0 ELSE 0.0 END),0) AS user_correction_rate, COALESCE(AVG(latency_ms),0) AS avg_latency_ms`).Scan(&row).Error
	return &MemoryMetricSummary{Runs: row.Runs, AvgPromptTokens: row.AvgPromptTokens, RepeatedToolCallRate: row.RepeatedToolCallRate, ContextHitRate: row.ContextHitRate, ErrorAssertionRate: row.ErrorAssertionRate, UserCorrectionRate: row.UserCorrectionRate, AvgLatencyMs: row.AvgLatencyMs}, err
}
