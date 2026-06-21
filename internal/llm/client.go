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

// Message 对话消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 对话请求
type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse 对话响应
type ChatResponse struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
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
		// 克隆请求（因为body只能读一次）
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

		// 成功响应
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		// 4xx 客户端错误不重试
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		// 5xx 服务器错误或 429 限流，进行重试
		lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			// 429 限流，等待更长时间
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
		} else {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doStreamRequest 发送流式HTTP请求
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

	// 添加历史消息（最多保留最近10轮）
	maxHistory := 20 // 10轮对话 = 20条消息
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	messages = append(messages, history...)

	// 添加用户消息
	messages = append(messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	return messages
}

// BuildDiagnosisMessages 构建诊断消息
func BuildDiagnosisMessages(resourceType, resourceName, namespace, problem string, context map[string]interface{}) []Message {
	contextJSON, _ := json.Marshal(context)

	prompt := fmt.Sprintf(`请诊断以下 Kubernetes 资源问题：

## 资源信息
- 类型: %s
- 名称: %s
- 命名空间: %s

## 问题描述
%s

## 相关上下文
%s

请提供：
1. 可能的原因分析
2. 排查步骤
3. 解决方案
4. 预防措施`, resourceType, resourceName, namespace, problem, string(contextJSON))

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
