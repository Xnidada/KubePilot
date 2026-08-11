package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIClient OpenAI客户端
type OpenAIClient struct {
	baseClient
}

// OpenAIRequest OpenAI请求格式
type OpenAIRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

// OpenAIResponse OpenAI响应格式
type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// OpenAIStreamResponse OpenAI流式响应格式
type OpenAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// NewOpenAIClient 创建OpenAI客户端
func NewOpenAIClient(cfg *LLMConfig) *OpenAIClient {
	return &OpenAIClient{
		baseClient: newBaseClient(cfg),
	}
}

// Chat 对话（支持 tools / tool_calls）
func (c *OpenAIClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := c.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := c.config.Model
	if model == "" {
		model = "gpt-3.5-turbo"
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

	openAIReq := OpenAIRequest{
		Model:       model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      false,
	}

	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	respBody, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	choice := openAIResp.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream 流式对话
func (c *OpenAIClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	baseURL := c.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := c.config.Model
	if model == "" {
		model = "gpt-3.5-turbo"
	}

	temperature := c.config.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	maxTokens := c.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}

	openAIReq := OpenAIRequest{
		Model:       model,
		Messages:    req.Messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      true,
	}

	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

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
				if err != io.EOF {
					ch <- StreamChunk{Content: "", Done: true}
				}
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					ch <- StreamChunk{Content: "", Done: true}
					break
				}

				var streamResp OpenAIStreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err == nil {
					if len(streamResp.Choices) > 0 {
						ch <- StreamChunk{
							Content: streamResp.Choices[0].Delta.Content,
							Done:    false,
						}
					}
				}
			}
		}
	}()

	return ch, nil
}
