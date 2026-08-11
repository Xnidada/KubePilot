package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AnthropicClient Anthropic客户端
type AnthropicClient struct {
	baseClient
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

// AnthropicRequest Anthropic请求格式
type AnthropicRequest struct {
	Model       string            `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature,omitempty"`
	System      string            `json:"system,omitempty"`
	Tools       []anthropicTool   `json:"tools,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

// AnthropicResponse Anthropic响应格式
type AnthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicStreamResponse Anthropic流式响应格式
type AnthropicStreamResponse struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// NewAnthropicClient 创建Anthropic客户端
func NewAnthropicClient(cfg *LLMConfig) *AnthropicClient {
	return &AnthropicClient{
		baseClient: newBaseClient(cfg),
	}
}

func toAnthropicTools(tools []ToolDefinition) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return out
}

func toAnthropicMessages(msgs []Message) (system string, out []anthropicMessage) {
	var systemParts []string
	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			if strings.TrimSpace(msg.Content) != "" {
				systemParts = append(systemParts, msg.Content)
			}
		case "assistant":
			blocks := []anthropicContentBlock{}
			if strings.TrimSpace(msg.Content) != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: ""})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: blocks})
		case "tool":
			blocks := []anthropicContentBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}}
			// Anthropic expects tool_result inside a user message
			if len(out) > 0 && out[len(out)-1].Role == "user" {
				out[len(out)-1].Content = append(out[len(out)-1].Content, blocks...)
			} else {
				out = append(out, anthropicMessage{Role: "user", Content: blocks})
			}
		default: // user
			out = append(out, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{{Type: "text", Text: msg.Content}},
			})
		}
	}
	system = strings.Join(systemParts, "\n\n")
	return system, out
}

// Chat 对话（支持 tools）
func (c *AnthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := c.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	model := c.config.Model
	if model == "" {
		model = "claude-3-sonnet-20240229"
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = c.config.Temperature
	}
	if temperature == 0 {
		temperature = 0.7
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.config.MaxTokens
	}
	if maxTokens == 0 {
		maxTokens = 2048
	}

	systemPrompt, messages := toAnthropicMessages(req.Messages)

	anthropicReq := AnthropicRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      systemPrompt,
		Tools:       toAnthropicTools(req.Tools),
		Stream:      false,
	}

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	respBody, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var contentParts []string
	var toolCalls []ToolCall
	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				contentParts = append(contentParts, block.Text)
			}
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	finish := anthropicResp.StopReason
	if finish == "tool_use" {
		finish = "tool_calls"
	}

	return &ChatResponse{
		Content:      strings.Join(contentParts, "\n"),
		ToolCalls:    toolCalls,
		FinishReason: finish,
		Usage: Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

// ChatStream 流式对话
func (c *AnthropicClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	baseURL := c.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	model := c.config.Model
	if model == "" {
		model = "claude-3-sonnet-20240229"
	}

	temperature := c.config.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.config.MaxTokens
	}
	if maxTokens == 0 {
		maxTokens = 2048
	}

	systemPrompt, messages := toAnthropicMessages(req.Messages)

	anthropicReq := AnthropicRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      systemPrompt,
		Stream:      true,
	}

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.doStreamRequest(httpReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 100)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				var streamResp AnthropicStreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err == nil {
					if streamResp.Type == "content_block_delta" {
						ch <- StreamChunk{
							Content: streamResp.Delta.Text,
							Done:    false,
						}
					} else if streamResp.Type == "message_stop" {
						ch <- StreamChunk{Content: "", Done: true}
						break
					}
				}
			}
		}
	}()

	return ch, nil
}
