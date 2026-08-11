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
	"time"
)

// LLMProvider LLM提供者类型
type LLMProvider string

const (
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
)

// ToolCall is a model-requested function invocation.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the function name and JSON argument string.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition describes a function the model may call (OpenAI shape).
type ToolDefinition struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the OpenAI function schema.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Message 对话消息（支持 tool / tool_calls）
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ChatRequest 对话请求
type ChatRequest struct {
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

// ChatResponse 对话响应
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Usage        Usage      `json:"usage"`
}

// Usage Token 使用情况
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

// LLMConfig LLM配置
type LLMConfig struct {
	Provider    LLMProvider `json:"provider"`
	APIKey      string      `json:"api_key"`
	BaseURL     string      `json:"base_url"`
	Model       string      `json:"model"`
	Temperature float64     `json:"temperature"`
	MaxTokens   int         `json:"max_tokens"`
	Timeout     int         `json:"timeout"` // 秒
}

// Client LLM客户端接口
type Client interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
}

// NewClient 创建LLM客户端
func NewClient(cfg *LLMConfig) (Client, error) {
	switch cfg.Provider {
	case ProviderOpenAI:
		return NewOpenAIClient(cfg), nil
	case ProviderAnthropic:
		return NewAnthropicClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}
}

// baseClient 基础客户端
type baseClient struct {
	config     *LLMConfig
	httpClient *http.Client
}

func newBaseClient(cfg *LLMConfig) baseClient {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return baseClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// doRequest 发送HTTP请求（带重试）
func (c *baseClient) doRequest(req *http.Request) ([]byte, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		clonedReq := req.Clone(req.Context())
		if req.Body != nil {
			bodyBytes, _ := io.ReadAll(req.Body)
			req.Body.Close()
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			clonedReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		resp, err := c.httpClient.Do(clonedReq)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
		} else {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *baseClient) doStreamRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// SystemPrompt 系统提示词
const SystemPrompt = `你是 KubePilot 智能运维助手，专注于 Kubernetes 集群运维。

你的能力：
1. 解答 K8S 相关问题
2. 分析集群问题和告警
3. 提供运维建议和最佳实践
4. 协助排查故障
5. 解释 K8S 概念和命令

回复要求：
- 使用中文回复
- 结构化输出，使用 Markdown 格式
- 给出具体可操作的建议
- 涉及命令时给出完整示例
- 分析问题时列出可能的原因和排查步骤`

// BuildMessages 构建消息列表
func BuildMessages(history []Message, userMessage string) []Message {
	messages := []Message{
		{Role: "system", Content: SystemPrompt},
	}

	maxHistory := 20
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	messages = append(messages, history...)

	messages = append(messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	return messages
}

// BuildDiagnosisMessages 构建诊断消息
func BuildDiagnosisMessages(resourceType, resourceName, namespace, problem string, context map[string]interface{}) []Message {
	prompt := fmt.Sprintf("诊断 K8S %s/%s (ns:%s): %s", resourceType, resourceName, namespace, problem)

	return []Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "user", Content: prompt},
	}
}

// StreamCallback 流式回调
type StreamCallback func(chunk string)

// ReadSSEStream 读取SSE流式响应
func ReadSSEStream(reader *bufio.Reader, callback StreamCallback) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			callback(data)
		}
	}
	return nil
}

// NewFunctionTool builds an OpenAI-compatible tool definition.
func NewFunctionTool(name, description string, parametersJSON string) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  json.RawMessage(parametersJSON),
		},
	}
}
