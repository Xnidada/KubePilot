package aiops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"gorm.io/gorm"
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
		if !ValidMemoryContent(state.Goal) {
			state.Goal = "[敏感或过长输入已省略]"
		}
		body := fmt.Sprintf("【结构化会话状态·历史事实可能过期，集群现状必须调用工具核实】\n当前目标：%s\n涉及对象：%s\n已验证事实：%s\n未完成事项：%s",
			state.Goal, defaultText(state.Entities, "无"), facts, defaultText(state.OpenItems, "无"))
		out = append(out, llm.Message{Role: "user", Content: truncateRunes(body, 1800)})
		out = append(out, llm.Message{Role: "assistant", Content: "已加载结构化状态；我会先核实有时效的集群事实。"})
	}

	memories := s.retrieveMemories(userID, clusterID, current, 4)
	if len(memories) > 0 {
		lines := make([]string, 0, len(memories))
		for _, m := range memories {
			lines = append(lines, fmt.Sprintf("- [%s, 来源 %s, 置信度 %.0f%%] %s", m.Type, m.Source, m.Confidence*100, m.Content))
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
	if s.db == nil || limit <= 0 {
		return nil
	}
	q := s.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now())
	q = q.Where("cluster_id IS NULL OR cluster_id = ?", clusterID)
	// Legacy auto-generated answer memories had no reliable verification. Keep
	// them visible for cleanup, but never inject them into a prompt.
	q = q.Where("type = ? OR (type = ? AND source LIKE ?)", "preference", "verified_knowledge", "reviewed:%")
	terms := memorySearchTerms(query)
	if len(terms) > 0 {
		parts := []string{"is_pinned = ?"}
		args := []interface{}{true}
		for _, term := range terms {
			parts = append(parts, "LOWER(content) LIKE ? ESCAPE '\\'")
			args = append(args, "%"+escapeLike(term)+"%")
		}
		q = q.Where("("+strings.Join(parts, " OR ")+")", args...)
	}
	var rows []model.AgentMemory
	if q.Order("is_pinned DESC, updated_at DESC").Limit(200).Find(&rows).Error != nil {
		return nil
	}
	filtered := rows[:0]
	for _, row := range rows {
		if ValidMemoryContent(row.Content) && ValidMemoryContent(row.Source) {
			filtered = append(filtered, row)
		}
	}
	rows = filtered
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := memoryRelevance(rows[i], terms), memoryRelevance(rows[j], terms)
		if left == right {
			return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
		}
		return left > right
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

var memoryWords = regexp.MustCompile(`[A-Za-z0-9_./-]+|[\p{Han}]+`)
var memorySecrets = regexp.MustCompile(`(?i)(password|passwd|secret|api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|kubeconfig|private[_-]?key|密码|密钥|口令)\s*[:=：是为]\s*\S+|bearer\s+\S+|-----BEGIN[^\n]*PRIVATE KEY|sk-[A-Za-z0-9]{12,}|gh[pousr]_[A-Za-z0-9]{12,}|AKIA[0-9A-Z]{16}`)

// ValidMemoryContent rejects obvious credentials and oversized payloads. It is
// a guardrail, not a replacement for human review of operational knowledge.
func ValidMemoryContent(content string) bool {
	content = strings.TrimSpace(content)
	return content != "" && len([]rune(content)) <= 800 && !memorySecrets.MatchString(content)
}

func memorySearchTerms(query string) []string {
	seen := make(map[string]bool)
	terms := make([]string, 0, 64)
	add := func(term string) {
		term = strings.ToLower(term)
		if len([]rune(term)) >= 2 && !seen[term] && len(terms) < 64 {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	for _, word := range memoryWords.FindAllString(query, -1) {
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		if runes[0] >= '\u4e00' && runes[0] <= '\u9fff' {
			for i := 0; i+1 < len(runes); i++ {
				add(string(runes[i : i+2]))
			}
		} else {
			add(word)
		}
	}
	if len(terms) > 16 {
		return append(terms[:8:8], terms[len(terms)-8:]...)
	}
	return terms
}

func escapeLike(term string) string {
	term = strings.ReplaceAll(term, `\`, `\\`)
	term = strings.ReplaceAll(term, `%`, `\%`)
	return strings.ReplaceAll(term, `_`, `\_`)
}

func memoryRelevance(m model.AgentMemory, terms []string) float64 {
	score := m.Confidence
	if m.IsPinned {
		score += 4
	}
	content := strings.ToLower(m.Content)
	for _, term := range terms {
		if strings.Contains(content, term) {
			score += 2
		}
	}
	return score
}

func (s *Service) refreshConversationState(userID, clusterID, conversationID uint, userMsg, _ string, trace []ToolTraceItem, pending []PendingActionInfo) {
	if conversationID == 0 || s.db == nil {
		return
	}
	facts := make([]memoryFact, 0, minInt(len(trace), 3))
	for _, t := range trace {
		if fact, ok := verifiedToolFact(t); ok {
			facts = append(facts, fact)
		}
		if len(facts) == 3 {
			break
		}
	}
	factsJSON, _ := json.Marshal(facts)
	open := "无"
	if len(pending) > 0 {
		open = fmt.Sprintf("有 %d 个写操作等待确认", len(pending))
	}
	goal := truncateRunes(userMsg, 500)
	if !ValidMemoryContent(userMsg) {
		goal = "[敏感或过长输入已省略]"
	}
	state := model.ConversationState{ConversationID: conversationID, UserID: userID, ClusterID: clusterID, Goal: goal, Facts: string(factsJSON), OpenItems: open, UpdatedAt: time.Now()}
	_ = s.db.Where("conversation_id = ?", conversationID).Assign(state).FirstOrCreate(&state).Error
	// Only explicit, non-sensitive preferences are promoted automatically.
	// Operational knowledge needs a human-provided verification source.
	if looksLikePreference(userMsg) {
		s.storeMemory(userID, nil, "preference", userMsg, "conversation:"+fmt.Sprint(conversationID), 0.8, nil)
	}
}

func (s *Service) storeMemory(userID uint, clusterID *uint, typ, content, source string, confidence float64, expiresAt *time.Time) {
	content = strings.TrimSpace(content)
	if !ValidMemoryContent(content) || !ValidMemoryContent(source) {
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
	m := model.AgentMemory{UserID: userID, ClusterID: clusterID, Type: typ, Content: content, Source: source, Confidence: confidence, ExpiresAt: expiresAt}
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		return tx.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: userID, Action: "create", Detail: "automatic"}).Error
	})
}

func verifiedToolFact(t ToolTraceItem) (memoryFact, bool) {
	if t.IsError || t.Name != "get_resource" {
		return memoryFact{}, false
	}
	var args struct {
		ResourceType string `json:"resource_type"`
		Namespace    string `json:"namespace"`
		Name         string `json:"name"`
	}
	var doc struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
		Status struct {
			Phase         string `json:"phase"`
			ReadyReplicas int32  `json:"readyReplicas"`
		} `json:"status"`
	}
	if json.Unmarshal([]byte(t.Args), &args) != nil || json.Unmarshal([]byte(t.Result), &doc) != nil || args.Name == "" || doc.Metadata.Name != args.Name {
		return memoryFact{}, false
	}
	if args.Namespace != "" && doc.Metadata.Namespace != args.Namespace {
		return memoryFact{}, false
	}
	var detail string
	switch strings.ToLower(args.ResourceType) {
	case "pod", "pods":
		if doc.Status.Phase != "" {
			detail = "Pod 阶段=" + doc.Status.Phase
		}
	case "deployment", "deployments", "deploy":
		if doc.Spec.Replicas != nil {
			detail = fmt.Sprintf("Deployment 就绪副本=%d/%d", doc.Status.ReadyReplicas, *doc.Spec.Replicas)
		}
	case "namespace", "namespaces", "ns":
		if doc.Status.Phase != "" {
			detail = "Namespace 阶段=" + doc.Status.Phase
		}
	}
	if detail == "" {
		return memoryFact{}, false
	}
	identity := doc.Metadata.Namespace + "/" + doc.Metadata.Name
	if !ValidMemoryContent(identity) || !ValidMemoryContent(detail) {
		return memoryFact{}, false
	}
	return memoryFact{Text: identity + " " + detail, Source: "tool:get_resource:status_v1", Confidence: 0.9, ExpiresAt: time.Now().Add(10 * time.Minute)}, true
}

func activeFacts(raw string) string {
	var facts []memoryFact
	if json.Unmarshal([]byte(raw), &facts) != nil {
		return "无"
	}
	parts := make([]string, 0, len(facts))
	now := time.Now()
	for _, f := range facts {
		if f.Source == "tool:get_resource:status_v1" && f.ExpiresAt.After(now) && ValidMemoryContent(f.Text) {
			parts = append(parts, fmt.Sprintf("%s（来源 %s，置信度 %.0f%%，有效至 %s）", f.Text, f.Source, f.Confidence*100, f.ExpiresAt.Format(time.RFC3339)))
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
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) persistAgentRunMetric(userID, clusterID, conversationID, assistantMessageID uint, userMsg string, usage llm.Usage, trace []ToolTraceItem, started time.Time, memoryHits int) {
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
	rec := model.AgentRunMetric{UserID: userID, ClusterID: clusterID, ConversationID: conversationID, AssistantMessageID: assistantMessageID, PromptTokens: usage.PromptTokens, ToolCount: len(trace), RepeatedToolCalls: repeated, MemoryHits: memoryHits, UserCorrection: correction, LatencyMs: time.Since(started).Milliseconds(), CreatedAt: time.Now()}
	_ = s.db.Create(&rec).Error
}

type MemoryMetricSummary struct {
	Runs                   int64    `json:"runs"`
	AvgPromptTokens        float64  `json:"avg_prompt_tokens"`
	RepeatedToolCallRate   float64  `json:"repeated_tool_call_rate"`
	ContextHitRate         float64  `json:"context_hit_rate"`
	ErrorAssertionRate     *float64 `json:"error_assertion_rate"`
	ErrorAssertionReviewed int64    `json:"error_assertion_reviewed"`
	UserCorrectionRate     float64  `json:"user_correction_rate"`
	AvgLatencyMs           float64  `json:"avg_latency_ms"`
}

func (s *Service) GetMemoryMetricSummary(days int) (*MemoryMetricSummary, error) {
	if days <= 0 {
		days = 30
	}
	var row struct {
		Runs, ErrorAssertionReviewed                                                            int64
		AvgPromptTokens, RepeatedToolCallRate, ContextHitRate, UserCorrectionRate, AvgLatencyMs float64
		ErrorAssertionRate                                                                      sql.NullFloat64
	}
	err := s.db.Model(&model.AgentRunMetric{}).Where("created_at >= ?", time.Now().AddDate(0, 0, -days)).Select(`COUNT(*) AS runs, COALESCE(AVG(prompt_tokens),0) AS avg_prompt_tokens, COALESCE(AVG(CASE WHEN repeated_tool_calls > 0 THEN 1.0 ELSE 0.0 END),0) AS repeated_tool_call_rate, COALESCE(AVG(CASE WHEN memory_hits > 0 THEN 1.0 ELSE 0.0 END),0) AS context_hit_rate, COUNT(CASE WHEN error_assertion_reviewed THEN 1 END) AS error_assertion_reviewed, AVG(CASE WHEN error_assertion_reviewed THEN CASE WHEN error_assertion THEN 1.0 ELSE 0.0 END END) AS error_assertion_rate, COALESCE(AVG(CASE WHEN user_correction THEN 1.0 ELSE 0.0 END),0) AS user_correction_rate, COALESCE(AVG(latency_ms),0) AS avg_latency_ms`).Scan(&row).Error
	var rate *float64
	if row.ErrorAssertionRate.Valid {
		rate = &row.ErrorAssertionRate.Float64
	}
	return &MemoryMetricSummary{Runs: row.Runs, AvgPromptTokens: row.AvgPromptTokens, RepeatedToolCallRate: row.RepeatedToolCallRate, ContextHitRate: row.ContextHitRate, ErrorAssertionRate: rate, ErrorAssertionReviewed: row.ErrorAssertionReviewed, UserCorrectionRate: row.UserCorrectionRate, AvgLatencyMs: row.AvgLatencyMs}, err
}
