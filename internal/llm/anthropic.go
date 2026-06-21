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

// AnthropicRequest Anthropic请求格式
type AnthropicRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// AnthropicResponse Anthropic响应格式
type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
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

// Chat 对话
func (c *AnthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
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

	maxTokens := c.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}

	// 提取系统消息
	systemPrompt := ""
	var messages []Message
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			messages = append(messages, msg)
		}
	}

	anthropicReq := AnthropicRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      systemPrompt,
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

	content := ""
	if len(anthropicResp.Content) > 0 {
		content = anthropicResp.Content[0].Text
	}

	return &ChatResponse{
		Content: content,
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

	maxTokens := c.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}

	// 提取系统消息
	systemPrompt := ""
	var messages []Message
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			messages = append(messages, msg)
		}
	}

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
