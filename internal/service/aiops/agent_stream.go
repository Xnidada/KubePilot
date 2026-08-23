package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"golang.org/x/sync/errgroup"
)

// AgentStreamEvent is one SSE payload for /aiops/agent/stream.
type AgentStreamEvent struct {
	Type           string              `json:"type"` // status|tool_start|tool_result|content_delta|done|error
	Status         string              `json:"status,omitempty"`
	Name           string              `json:"name,omitempty"`
	Args           string              `json:"args,omitempty"`
	Result         string              `json:"result,omitempty"`
	IsError        bool                `json:"is_error,omitempty"`
	Delta          string              `json:"delta,omitempty"`
	Content        string              `json:"content,omitempty"`
	Message        string              `json:"message,omitempty"`
	PendingActions []PendingActionInfo `json:"pending_actions,omitempty"`
	ToolTrace      []ToolTraceItem     `json:"tool_trace,omitempty"`
	MessageID      uint                `json:"message_id,omitempty"`
	Pending        *PendingActionInfo  `json:"pending,omitempty"`
	DurationMs     int64               `json:"duration_ms,omitempty"`
}

// MessageExtras is stored on ChatMessage.Extras as JSON.
type MessageExtras struct {
	ToolTrace        []ToolTraceItem `json:"tool_trace,omitempty"`
	PendingActionIDs []uint          `json:"pending_action_ids,omitempty"`
}

type agentLoopResult struct {
	Content string
	Pending []PendingActionInfo
	Trace   []ToolTraceItem
	Usage   llm.Usage
}

type agentEmitFunc func(AgentStreamEvent)

// AgentChatStream runs the tool loop and emits SSE-friendly events.
// When conversationID > 0, persists the assistant message with extras before done.
func (s *Service) AgentChatStream(ctx context.Context, userID, clusterID, conversationID uint, message string, emit agentEmitFunc) error {
	if s.llmClient == nil {
		emit(AgentStreamEvent{Type: "error", Message: "LLM service not configured"})
		return fmt.Errorf("LLM service not configured")
	}
	if emit == nil {
		emit = func(AgentStreamEvent) {}
	}

	emit(AgentStreamEvent{Type: "status", Status: "thinking"})

	res, err := s.runAgentToolLoop(ctx, userID, clusterID, conversationID, message, emit)
	if err != nil {
		emit(AgentStreamEvent{Type: "error", Message: err.Error()})
		return err
	}

	emit(AgentStreamEvent{Type: "status", Status: "summarizing"})
	// Prefer true token stream for the final answer when we already have tool context.
	streamed, streamErr := s.streamFinalViaLLM(ctx, userID, clusterID, conversationID, message, res, emit)
	if streamErr == nil && streamed != "" {
		res.Content = streamed
	} else {
		_ = streamContentDeltas(ctx, res.Content, emit)
	}

	var msgID uint
	if conversationID > 0 {
		msgID, _ = s.persistAssistantMessage(conversationID, res.Content, res.Trace, res.Pending)
	}
	s.persistAgentToolTrace(userID, clusterID, conversationID, message, res.Trace, res.Pending)

	emit(AgentStreamEvent{
		Type:           "done",
		Content:        res.Content,
		PendingActions: res.Pending,
		ToolTrace:      res.Trace,
		MessageID:      msgID,
	})
	return nil
}

// streamFinalViaLLM re-asks without tools for a true token stream when tools already ran.
func (s *Service) streamFinalViaLLM(ctx context.Context, userID, clusterID, conversationID uint, userMsg string, res *agentLoopResult, emit agentEmitFunc) (string, error) {
	if s.llmClient == nil || len(res.Trace) == 0 {
		return "", fmt.Errorf("skip")
	}
	clusterContext, _ := s.getClusterContext(clusterID)
	system := agentSystemPrompt
	if strings.TrimSpace(clusterContext) != "" {
		system += "\n\n当前集群摘要（仅供参考，详情仍须用工具查询）：\n" + truncateRunes(clusterContext, 800)
	}
	var toolDigest strings.Builder
	toolDigest.WriteString("本轮已完成的工具结果（请完整引用其中的资源名称；meta.truncated=false 表示列表完整，禁止声称被截断）：\n")
	for _, t := range res.Trace {
		// Keep enough room for list_resources lines; never use a tiny cap that slices names.
		toolDigest.WriteString(fmt.Sprintf("### tool %s (error=%v)\n%s\n", t.Name, t.IsError, truncateRunes(t.Result, toolResultMaxChars)))
	}
	if len(res.Pending) > 0 {
		toolDigest.WriteString(fmt.Sprintf("已暂存 %d 个写操作待用户确认。\n", len(res.Pending)))
	}
	messages := []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: userMsg},
		{Role: "assistant", Content: toolDigest.String()},
		{Role: "user", Content: "请仅基于以上工具结果用中文给出最终回答。规则：1) 资源名必须原样完整写出，禁止缩写/截断（如不要把 nginx-deployment-xxx 写成 ngi，不要把 kubernetes 写成 kube）；2) 若工具 meta 为 truncated=false 或 showing=total，必须列出全部名称，禁止说「列表被截断」；3) 不要编造未出现的资源；4) 不要再调用工具。"},
	}
	ch, err := s.llmClient.ChatStream(ctx, &llm.ChatRequest{Messages: messages, MaxTokens: 4096})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range ch {
		select {
		case <-ctx.Done():
			return b.String(), ctx.Err()
		default:
		}
		if chunk.Content != "" {
			b.WriteString(chunk.Content)
			emit(AgentStreamEvent{Type: "content_delta", Delta: chunk.Content})
		}
		if chunk.Done {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("empty stream")
	}
	if len(res.Pending) > 0 {
		out = stripFakeAgentActionBlocks(out)
	}
	return out, nil
}

func streamContentDeltas(ctx context.Context, content string, emit agentEmitFunc) error {
	if content == "" {
		return nil
	}
	const chunkRunes = 24
	runes := []rune(content)
	for i := 0; i < len(runes); i += chunkRunes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		end := i + chunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		emit(AgentStreamEvent{Type: "content_delta", Delta: string(runes[i:end])})
		time.Sleep(8 * time.Millisecond)
	}
	return nil
}

func (s *Service) persistAssistantMessage(conversationID uint, content string, trace []ToolTraceItem, pending []PendingActionInfo) (uint, error) {
	ids := make([]uint, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.ID)
	}
	extrasBytes, _ := json.Marshal(MessageExtras{
		ToolTrace:        trace,
		PendingActionIDs: ids,
	})
	msg := &model.ChatMessage{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        content,
		Extras:         string(extrasBytes),
		CreatedAt:      time.Now(),
	}
	if err := s.db.Create(msg).Error; err != nil {
		// Fallback if extras column not yet migrated.
		msg.Extras = ""
		if err2 := s.db.Omit("Extras").Create(msg).Error; err2 != nil {
			return 0, err2
		}
	}
	s.db.Model(&model.ChatConversation{}).Where("id = ?", conversationID).Update("updated_at", msg.CreatedAt)
	return msg.ID, nil
}

// ListPendingActions returns pending staged actions for a conversation.
func (s *Service) ListPendingActions(userID, conversationID uint) ([]PendingActionInfo, error) {
	var rows []model.AgentAction
	q := s.db.Where("status = ? AND conversation_id = ?", "pending", conversationID)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PendingActionInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, PendingActionInfo{
			ID:          r.ID,
			ActionID:    r.ID,
			Action:      r.ResourceType,
			Name:        r.ResourceName,
			Namespace:   r.Namespace,
			Description: r.Description,
			DryRun:      r.DryRunResult,
			NeedConfirm: true,
		})
	}
	return out, nil
}

// CancelPendingActions marks pending actions cancelled for a conversation (or specific IDs).
func (s *Service) CancelPendingActions(userID, conversationID uint, actionIDs []uint) error {
	q := s.db.Model(&model.AgentAction{}).
		Where("status = ? AND conversation_id = ? AND user_id = ?", "pending", conversationID, userID)
	if len(actionIDs) > 0 {
		q = q.Where("id IN ?", actionIDs)
	}
	return q.Update("status", "cancelled").Error
}

func (s *Service) runAgentToolLoop(ctx context.Context, userID, clusterID, conversationID uint, message string, emit agentEmitFunc) (*agentLoopResult, error) {
	clusterContext, _ := s.getClusterContext(clusterID)

	system := agentSystemPrompt
	if strings.TrimSpace(clusterContext) != "" {
		system += "\n\n当前集群摘要（仅供参考，详情仍须用工具查询）：\n" + truncateRunes(clusterContext, 1500)
	}

	messages := []llm.Message{{Role: "system", Content: system}}
	messages = append(messages, s.buildAgentMemoryMessages(conversationID, message)...)
	messages = append(messages, llm.Message{Role: "user", Content: message})

	tools := agentToolDefinitions()
	var trace []ToolTraceItem
	var pending []PendingActionInfo

	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var finalContent string
	var totalUsage llm.Usage
	nudgeUsed := false
	mountNudgeCount := 0
	consecutiveNoToolRounds := 0 // 动态轮次：连续无工具调用轮次计数
	for round := 0; round < agentMaxToolRounds; round++ {
		if err := chatCtx.Err(); err != nil {
			if finalContent == "" && len(pending) > 0 {
				finalContent = "已暂存变更，请在界面确认后执行。"
			}
			if finalContent == "" && len(trace) > 0 {
				finalContent = "请求已取消；已保留此前工具结果。"
			}
			if finalContent != "" || len(trace) > 0 || len(pending) > 0 {
				return &agentLoopResult{Content: finalContent, Pending: pending, Trace: trace, Usage: totalUsage}, nil
			}
			return nil, err
		}

		// 动态轮次提示：剩余轮次 ≤ 3 时追加提示
		remaining := agentMaxToolRounds - round
		if remaining <= 3 && remaining > 0 {
			hint := fmt.Sprintf("\n\n⚠️ 剩余工具调用轮次有限（%d/%d），请优先完成最关键的写操作。", remaining, agentMaxToolRounds)
			if messages[0].Role == "system" && !strings.Contains(messages[0].Content, "剩余工具调用轮次有限") {
				messages[0].Content += hint
			}
		}

		// 智能上下文压缩：预估 token 数，超限时压缩较早的 tool_result
		messages = compressToolResults(messages, 100000) // 100K token 阈值（约 80% of 128K context）

		if emit != nil {
			emit(AgentStreamEvent{Type: "status", Status: "thinking"})
		}
		resp, err := s.llmClient.Chat(chatCtx, &llm.ChatRequest{
			Messages:  messages,
			Tools:     tools,
			MaxTokens: 4096,
		})
		if err != nil {
			return nil, fmt.Errorf("LLM chat failed: %w", err)
		}
			totalUsage.PromptTokens += resp.Usage.PromptTokens
			totalUsage.CompletionTokens += resp.Usage.CompletionTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens

		if len(resp.ToolCalls) == 0 {
			finalContent = resp.Content
			nudge := s.agentNudgeIfNeeded(message, finalContent, trace, pending)
			if nudge != "" {
				isMountNudge := strings.Contains(nudge, "host_path_mounts")
				if isMountNudge && mountNudgeCount < 2 {
					mountNudgeCount++
				} else if !isMountNudge && !nudgeUsed {
					nudgeUsed = true
				} else {
					nudge = ""
				}
			}
			if nudge != "" {
				messages = append(messages,
					llm.Message{Role: "assistant", Content: finalContent},
					llm.Message{Role: "user", Content: nudge},
				)
				finalContent = ""
				continue
			}
			// 动态轮次：非 nudge 轮且无工具调用 → 递增计数器
			consecutiveNoToolRounds++
			if consecutiveNoToolRounds >= 2 {
				break // 连续 2 轮无工具调用，提前退出
			}
			break
		}

		// 有工具调用，重置计数器
		consecutiveNoToolRounds = 0

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// 并发执行 tool_calls：读操作并发，写操作串行
		type toolExecRecord struct {
			Index    int
			Exec     *toolExecResult
			Item     ToolTraceItem
			ToolCall llm.ToolCall
		}
		results := make([]toolExecRecord, len(resp.ToolCalls))

		// 分离读操作和写操作的索引
		var readIdx, writeIdx []int
		for i, tc := range resp.ToolCalls {
			if isReadOnlyTool(tc.Function.Name) {
				readIdx = append(readIdx, i)
			} else {
				writeIdx = append(writeIdx, i)
			}
		}

		// 并发执行读操作
		if len(readIdx) > 0 {
			g, gCtx := errgroup.WithContext(chatCtx)
			g.SetLimit(4)
			for _, idx := range readIdx {
				idx := idx
				g.Go(func() error {
					if err := gCtx.Err(); err != nil {
						return err
					}
					tc := resp.ToolCalls[idx]
					name := tc.Function.Name
					args := tc.Function.Arguments
					if name == "propose_mutation" {
						args = enrichMutationArgsWithUserHints(message, args)
					}
					if emit != nil {
						emit(AgentStreamEvent{Type: "tool_start", Name: name, Args: truncateRunes(args, 500)})
					}
					started := time.Now()
					exec := s.executeAgentTool(gCtx, userID, clusterID, conversationID, name, args)
					item := ToolTraceItem{
						Name:       name,
						Args:       truncateRunes(args, 500),
						Result:     truncateRunes(exec.Content, toolResultMaxChars),
						IsError:    exec.IsError,
						DurationMs: time.Since(started).Milliseconds(),
					}
					results[idx] = toolExecRecord{Index: idx, Exec: &exec, Item: item, ToolCall: tc}
					if emit != nil {
						emit(AgentStreamEvent{
							Type:       "tool_result",
							Name:       name,
							Result:     item.Result,
							IsError:    exec.IsError,
							Pending:    exec.Pending,
							DurationMs: item.DurationMs,
						})
					}
					return nil
				})
			}
			_ = g.Wait() // errors handled per-tool; we proceed with whatever completed
		}

		// 串行执行写操作
		for _, idx := range writeIdx {
			if err := chatCtx.Err(); err != nil {
				if finalContent == "" {
					finalContent = "请求已取消；已保留此前工具结果。"
				}
				return &agentLoopResult{Content: finalContent, Pending: pending, Trace: trace, Usage: totalUsage}, nil
			}
			tc := resp.ToolCalls[idx]
			name := tc.Function.Name
			args := tc.Function.Arguments
			if name == "stage_mutation" || name == "stage_mutations" || name == "propose_mutation" {
				args = enrichMutationArgsWithUserHints(message, args)
			}
			if emit != nil {
				emit(AgentStreamEvent{Type: "tool_start", Name: name, Args: truncateRunes(args, 500)})
			}
			started := time.Now()
			exec := s.executeAgentTool(chatCtx, userID, clusterID, conversationID, name, args)
			item := ToolTraceItem{
				Name:       name,
				Args:       truncateRunes(args, 500),
				Result:     truncateRunes(exec.Content, toolResultMaxChars),
				IsError:    exec.IsError,
				DurationMs: time.Since(started).Milliseconds(),
			}
			results[idx] = toolExecRecord{Index: idx, Exec: &exec, Item: item, ToolCall: tc}
			if emit != nil {
				emit(AgentStreamEvent{
					Type:       "tool_result",
					Name:       name,
					Result:     item.Result,
					IsError:    exec.IsError,
					Pending:    exec.Pending,
					DurationMs: item.DurationMs,
				})
			}
		}

		// 按原始顺序追加 trace、pending、messages
		for _, r := range results {
			if r.Exec == nil {
				continue
			}
			trace = append(trace, r.Item)
			if len(r.Exec.PendingList) > 0 {
				pending = mergePendingActions(pending, r.Exec.PendingList)
			} else if r.Exec.Pending != nil {
				pending = mergePendingActions(pending, []PendingActionInfo{*r.Exec.Pending})
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: r.ToolCall.ID,
				Name:       r.ToolCall.Function.Name,
				Content:    r.Exec.Content,
			})
		}

		if round == agentMaxToolRounds-1 && finalContent == "" {
			finalContent = resp.Content
			if finalContent == "" {
				if len(pending) > 0 {
					finalContent = "已暂存变更，请在界面确认后执行。"
				} else {
					finalContent = "已达到工具调用轮次上限，请根据已查询结果继续提问或缩小范围。"
				}
			}
		}
	}

	if finalContent == "" {
		if len(pending) > 0 {
			finalContent = "已暂存变更，请在界面确认后执行。"
		} else {
			finalContent = "（模型未返回文本；请查看工具轨迹或重试。）"
		}
	}
	if len(pending) > 0 {
		finalContent = stripFakeAgentActionBlocks(finalContent)
	}
	pending = s.dropIncompleteMountDeployments(message, pending)
	s.persistTokenUsage(userID, conversationID, totalUsage, "agent")

	return &agentLoopResult{Content: finalContent, Pending: pending, Trace: trace, Usage: totalUsage}, nil
}

func mergePendingActions(existing []PendingActionInfo, incoming []PendingActionInfo) []PendingActionInfo {
	out := append([]PendingActionInfo{}, existing...)
	for _, in := range incoming {
		replaced := false
		for i, old := range out {
			if old.Action == in.Action && old.Namespace == in.Namespace && old.Name == in.Name {
				out[i] = in
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, in)
		}
	}
	return out
}

// dropIncompleteMountDeployments cancels staged create_deployment that still lack hostPath when user required mounts.
func (s *Service) dropIncompleteMountDeployments(userMsg string, pending []PendingActionInfo) []PendingActionInfo {
	if !isHostMountIntent(userMsg) || len(pending) == 0 {
		return pending
	}
	kept := make([]PendingActionInfo, 0, len(pending))
	for _, p := range pending {
		if p.Action == "create_deployment" && !strings.Contains(p.DryRun, "hostPath.path") {
			if s.db != nil && p.ID > 0 {
				_ = s.db.Model(&model.AgentAction{}).Where("id = ? AND status = ?", p.ID, "pending").
					Update("status", "cancelled").Error
			}
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

func isFixIntent(msg string) bool {
	keys := []string{"修好", "修复", "帮我改", "改一下", "改成", "重建", "删掉重建", "请修改", "请调整", "帮我修"}
	for _, k := range keys {
		if strings.Contains(msg, k) {
			return true
		}
	}
	return false
}

func (s *Service) agentNudgeIfNeeded(userMsg, assistantContent string, trace []ToolTraceItem, pending []PendingActionInfo) string {
	if looksLikeFakeAgentActions(assistantContent) {
		return "禁止输出 action JSON 或伪协议。请立即用工具：先 list_resources/get_resource 核对名称，再对每个目标调用 stage_mutation。不要只列名单。"
	}
	if len(pending) == 0 && isWriteIntent(userMsg) && !traceHasTool(trace, "stage_mutation") && !traceHasTool(trace, "stage_mutations") && !traceHasTool(trace, "delete_by_prefix") {
		return "这是写操作请求。请先用 list_resources/get_resource 核对准确名称，再调用 stage_mutation / stage_mutations / delete_by_prefix。禁止只列名单或声称已执行。"
	}
	// Prefer mount completeness before NodePort — users often ask for both in one request.
	if isHostMountIntent(userMsg) && (isWriteIntent(userMsg) || len(pending) > 0) {
		hasMount := false
		for _, pend := range pending {
			if strings.Contains(pend.DryRun, "hostPath.path") || strings.Contains(pend.DryRun, "volumeMounts") {
				hasMount = true
				break
			}
		}
		for _, t := range trace {
			if (t.Name == "stage_mutation" || t.Name == "stage_mutations" || t.Name == "propose_mutation") &&
				(strings.Contains(t.Args, "host_path") || strings.Contains(t.Args, "hostPath") || strings.Contains(t.Result, "hostPath.path")) &&
				!t.IsError {
				hasMount = true
				break
			}
		}
		if !hasMount {
			return "用户要求主机路径挂载。请重新 stage_mutation：action=create_deployment，并传 host_path_mounts=[{host_path,mount_path},...]。例如网页目录 host_path=/opt/nginx/html mount_path=/usr/share/nginx/html，日志 host_path=/opt/nginx/log mount_path=/var/log/nginx。禁止省略挂载。"
		}
	}
	if wanted := extractRequestedNodePorts(userMsg); len(wanted) > 0 {
		for _, p := range wanted {
			needle := fmt.Sprintf("nodePort: %d", p)
			matched := false
			for _, pend := range pending {
				if strings.Contains(pend.DryRun, needle) || strings.Contains(pend.Description, needle) {
					matched = true
					break
				}
			}
			for _, t := range trace {
				if (t.Name == "stage_mutation" || t.Name == "stage_mutations") && strings.Contains(t.Result, needle) && !t.IsError {
					matched = true
					break
				}
			}
			if !matched && (isWriteIntent(userMsg) || isExternalExposeIntent(userMsg) || len(pending) > 0) {
				return fmt.Sprintf("用户明确要求外部端口 %d。请重新 stage_mutation：action=create_service, service_type=NodePort, port=80, target_port=80, node_port=%d，selector 对齐工作负载标签。禁止省略 node_port。", p, p)
			}
		}
	}
	if isServiceAccessIntent(userMsg) && !isWriteIntent(userMsg) && !isFixIntent(userMsg) && !isExternalExposeIntent(userMsg) && (traceHasTool(trace, "stage_mutation") || traceHasTool(trace, "stage_mutations") || traceHasTool(trace, "delete_by_prefix") || len(pending) > 0) {
		return "用户只是排查访问问题，并未要求修改。请停止写操作，仅基于 diagnose_service/get_resource 的 endpoints 与 selector_near_miss 给出原因和建议；不要 stage_mutation。"
	}
	if isServiceAccessIntent(userMsg) && !traceHasTool(trace, "diagnose_service") && !traceHasTool(trace, "get_resource") {
		return "这是 Service/端口连通性问题。请立即调用 diagnose_service（有端口就传 node_port；有名字就传 namespace+name）。必须根据工具返回的 endpoints 与 selector_near_miss 下结论，禁止臆造 nodePort。"
	}
	if len(trace) == 0 && isClusterQueryIntent(userMsg) && looksLikeClusterStateClaim(assistantContent) {
		return "你尚未调用任何工具，不得断言集群现状。请立即调用 list_resources / get_resource / describe_resource 等工具查询后再回答。"
	}
	return ""
}

func extractRequestedNodePorts(msg string) []int32 {
	var out []int32
	seen := map[int32]bool{}
	n := 0
	for _, r := range msg + " " {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			if n > 100000 {
				n = 0
			}
			continue
		}
		if n >= 30000 && n <= 32767 && !seen[int32(n)] {
			seen[int32(n)] = true
			out = append(out, int32(n))
		}
		n = 0
	}
	return out
}

func isExternalExposeIntent(msg string) bool {
	keys := []string{"外部", "对外", "NodePort", "nodeport", "暴露", "可以访问", "能访问", "访问到"}
	for _, k := range keys {
		if strings.Contains(msg, k) || strings.Contains(strings.ToLower(msg), strings.ToLower(k)) {
			return true
		}
	}
	return false
}

func isHostMountIntent(msg string) bool {
	lower := strings.ToLower(msg)
	keys := []string{"挂载", "hostpath", "host_path", "host path", "主机路径", "本地路径", "网页主目录", "volume_mount", "volumemount"}
	for _, k := range keys {
		if strings.Contains(lower, k) || strings.Contains(msg, k) {
			return true
		}
	}
	return false
}

func traceHasTool(trace []ToolTraceItem, name string) bool {
	for _, t := range trace {
		if t.Name == name {
			return true
		}
	}
	return false
}

func isWriteIntent(msg string) bool {
	lower := strings.ToLower(msg)
	keys := []string{"删除", "创建", "部署", "扩容", "缩容", "扩缩", "scale", "delete", "create", "apply", "更新镜像", "重启"}
	for _, k := range keys {
		if strings.Contains(msg, k) || strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func isClusterQueryIntent(msg string) bool {
	lower := strings.ToLower(msg)
	keys := []string{
		"pod", "pods", "部署", "deployment", "service", "命名空间", "namespace",
		"节点", "node", "日志", "log", "事件", "event", "状态", "查看", "列出", "有哪些", "多少",
		"端口", "nodeport", "访问不了", "无法访问", "连不上", "不通",
	}
	for _, k := range keys {
		if strings.Contains(msg, k) || strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func isServiceAccessIntent(msg string) bool {
	lower := strings.ToLower(msg)
	keys := []string{"无法访问", "访问不了", "连不上", "不通", "nodeport", "端口", "进不去", "打不开"}
	hit := false
	for _, k := range keys {
		if strings.Contains(msg, k) || strings.Contains(lower, k) {
			hit = true
			break
		}
	}
	if !hit {
		return false
	}
	// Port number or service-ish wording
	if strings.Contains(lower, "service") || strings.Contains(msg, "服务") || strings.Contains(lower, "svc") {
		return true
	}
	for _, r := range msg {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func looksLikeClusterStateClaim(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	// Short hedges are fine.
	hedges := []string{"尚未查询", "需要先", "请稍等", "我来查", "我将调用"}
	for _, h := range hedges {
		if strings.Contains(content, h) {
			return false
		}
	}
	claims := []string{
		"共有", "如下", "状态：", "Running", "Succeeded", "Pending", "CrashLoop",
		"命名空间下", "列表", "个 Pod", "个Deployment", "不存在", "存在",
	}
	hits := 0
	for _, c := range claims {
		if strings.Contains(content, c) {
			hits++
		}
	}
	return hits >= 1 && utf8.RuneCountInString(content) > 20
}

// isReadOnlyTool returns true for tools that only read cluster state (safe to run concurrently).
func isReadOnlyTool(name string) bool {
	switch name {
	case "list_resources", "get_resource", "get_events", "get_pod_logs",
		"describe_resource", "diagnose_workload", "diagnose_service", "propose_mutation":
		return true
	}
	return false
}

// estimateMessageTokens provides a rough token estimate for a slice of messages.
// Uses heuristic: ~2 bytes per token for mixed CJK/ASCII content.
func estimateMessageTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content) / 2
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments) / 2
		}
	}
	return total
}

// compressToolResults compresses older tool_result messages when estimated tokens exceed the limit.
// It replaces the content of the earliest tool results with a summary to free up context space.
func compressToolResults(msgs []llm.Message, tokenLimit int) []llm.Message {
	estimated := estimateMessageTokens(msgs)
	if estimated <= tokenLimit {
		return msgs
	}

	// Find tool_result messages and compress from oldest
	compressed := 0
	for i := range msgs {
		if estimated <= tokenLimit {
			break
		}
		if msgs[i].Role == "tool" && len(msgs[i].Content) > 500 {
			original := msgs[i].Content
			msgs[i].Content = summarizeToolResult(original)
			estimated -= (len(original) - len(msgs[i].Content)) / 2
			compressed++
		}
	}
	if compressed > 0 {
		_ = compressed // suppress unused warning; compression happened
	}
	return msgs
}

// summarizeToolResult creates a compact summary of a tool result for context compression.
func summarizeToolResult(content string) string {
	// Extract key lines: resource names, meta line, status summary
	lines := strings.Split(content, "\n")
	var summary strings.Builder
	nameCount := 0
	maxNames := 10

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Keep meta line
		if strings.HasPrefix(trimmed, "meta:") {
			summary.WriteString(trimmed)
			summary.WriteString("\n")
			continue
		}
		// Keep lines that look like resource names (indented, short, no special chars)
		if trimmed != "" && nameCount < maxNames {
			// Heuristic: resource names are short lines with alphanumeric/dashes/dots
			if isResourceNameLine(trimmed) {
				summary.WriteString(trimmed)
				summary.WriteString("\n")
				nameCount++
			}
		}
	}

	// If we found useful content, return it; otherwise return first 200 chars
	result := strings.TrimSpace(summary.String())
	if result == "" {
		result = truncateRunes(content, 200)
	}
	if len(content) > len(result) {
		result += fmt.Sprintf("\n...(compressed, original %d chars)", len(content))
	}
	return result
}

// isResourceNameLine heuristically identifies lines that contain Kubernetes resource names.
func isResourceNameLine(line string) bool {
	// Skip lines that are clearly not resource names
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
		return false
	}
	if strings.Contains(line, "```") || strings.Contains(line, "error") {
		return false
	}
	// Resource name lines are typically short (< 80 chars) and contain alphanumeric/dash/dot/slash
	if len(line) > 80 {
		return false
	}
	for _, r := range line {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '.' || r == '/' || r == '_' || r == ' ' || r == ':') {
			return false
		}
	}
	return true
}
