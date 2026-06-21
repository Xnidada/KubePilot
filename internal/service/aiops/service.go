package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service AIOps服务
type Service struct {
	db            *gorm.DB
	llmClient     llm.Client
	chatHistories map[uint][]llm.Message // userID -> chat history (fallback)
	cache         cache.Cache            // 缓存实例
	encryptKey    string                 // 用于解密kubeconfig
}

// NewService 创建AIOps服务
func NewService(db *gorm.DB, llmConfig *llm.LLMConfig, encryptKey string, cacheInstance ...cache.Cache) (*Service, error) {
	svc := &Service{
		db:            db,
		chatHistories: make(map[uint][]llm.Message),
		encryptKey:    encryptKey,
	}

	// 如果提供了缓存实例，使用缓存
	if len(cacheInstance) > 0 && cacheInstance[0] != nil {
		svc.cache = cacheInstance[0]
	}

	// 尝试从数据库加载配置
	var dbConfig model.LLMConfig
	if err := db.Where("is_active = ?", true).Order("id desc").First(&dbConfig).Error; err == nil {
		// 使用数据库配置
		llmConfig = &llm.LLMConfig{
			Provider:    llm.LLMProvider(dbConfig.Provider),
			APIKey:      dbConfig.APIKey,
			BaseURL:     dbConfig.BaseURL,
			Model:       dbConfig.Model,
			Temperature: dbConfig.Temperature,
			MaxTokens:   dbConfig.MaxTokens,
			Timeout:     dbConfig.Timeout,
		}
	}

	if llmConfig != nil && llmConfig.APIKey != "" {
		client, err := llm.NewClient(llmConfig)
		if err != nil {
			// 不返回错误，允许服务启动但AI功能不可用
			fmt.Printf("Warning: failed to create LLM client: %v\n", err)
		} else {
			svc.llmClient = client
		}
	}

	return svc, nil
}

// UpdateConfig 更新LLM配置
func (s *Service) UpdateConfig(cfg *llm.LLMConfig) error {
	client, err := llm.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}
	s.llmClient = client
	return nil
}

// IsConfigured 检查是否已配置
func (s *Service) IsConfigured() bool {
	return s.llmClient != nil
}

// Chat 对话请求
type ChatRequest struct {
	Message    string `json:"message" binding:"required"`
	ClusterID  uint   `json:"cluster_id"`
	Context    string `json:"context"` // 额外上下文
}

// ChatResponse 对话响应
type ChatResponse struct {
	Content string `json:"content"`
	Usage   llm.Usage `json:"usage"`
}

// Chat 智能对话
func (s *Service) Chat(ctx context.Context, userID uint, req *ChatRequest) (*ChatResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured. Please set LLM API key in config")
	}

	// 获取用户的对话历史
	history := s.getChatHistory(ctx, userID)

	// 如果有集群上下文，添加到消息中
	message := req.Message
	if req.ClusterID > 0 {
		clusterInfo, err := s.getClusterContext(req.ClusterID)
		if err == nil {
			message = fmt.Sprintf("当前集群信息:\n%s\n\n用户问题: %s", clusterInfo, message)
		}
	}

	// 构建消息
	messages := llm.BuildMessages(history, message)

	// 调用LLM
	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM chat failed: %w", err)
	}

	// 更新对话历史
	history = append(history, llm.Message{Role: "user", Content: req.Message})
	history = append(history, llm.Message{Role: "assistant", Content: resp.Content})

	// 保留最近20条消息
	if len(history) > 20 {
		history = history[len(history)-20:]
	}

	// 保存历史
	s.saveChatHistoryToCache(ctx, userID, history)

	// 保存到数据库
	s.saveChatHistory(userID, req.Message, resp.Content)

	return &ChatResponse{
		Content: resp.Content,
		Usage:   resp.Usage,
	}, nil
}

// getChatHistory 获取对话历史
func (s *Service) getChatHistory(ctx context.Context, userID uint) []llm.Message {
	// 优先从缓存获取
	if s.cache != nil {
		key := fmt.Sprintf("chat:history:%d", userID)
		data, err := s.cache.Get(ctx, key)
		if err == nil {
			var history []llm.Message
			if json.Unmarshal([]byte(data), &history) == nil {
				return history
			}
		}
	}

	// 回退到内存
	return s.chatHistories[userID]
}

// saveChatHistoryToCache 保存对话历史到缓存
func (s *Service) saveChatHistoryToCache(ctx context.Context, userID uint, history []llm.Message) {
	// 保存到缓存
	if s.cache != nil {
		key := fmt.Sprintf("chat:history:%d", userID)
		data, _ := json.Marshal(history)
		s.cache.Set(ctx, key, string(data), 24*time.Hour)
	}

	// 同时保存到内存作为备份
	s.chatHistories[userID] = history
}

// ChatStream 流式对话
func (s *Service) ChatStream(ctx context.Context, userID uint, req *ChatRequest) (<-chan llm.StreamChunk, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured. Please set LLM API key in config")
	}

	// 获取用户的对话历史
	history := s.chatHistories[userID]

	// 如果有集群上下文，添加到消息中
	message := req.Message
	if req.ClusterID > 0 {
		clusterInfo, err := s.getClusterContext(req.ClusterID)
		if err == nil {
			message = fmt.Sprintf("当前集群信息:\n%s\n\n用户问题: %s", clusterInfo, message)
		}
	}

	// 构建消息
	messages := llm.BuildMessages(history, message)

	// 调用LLM流式API
	ch, err := s.llmClient.ChatStream(ctx, &llm.ChatRequest{
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM stream failed: %w", err)
	}

	// 异步更新历史
	go func() {
		var fullContent string
		for chunk := range ch {
			fullContent += chunk.Content
			if chunk.Done {
				// 更新对话历史
				history = append(history, llm.Message{Role: "user", Content: req.Message})
				history = append(history, llm.Message{Role: "assistant", Content: fullContent})
				if len(history) > 20 {
					history = history[len(history)-20:]
				}
				s.chatHistories[userID] = history
				s.saveChatHistory(userID, req.Message, fullContent)
			}
		}
	}()

	return ch, nil
}

// ClearHistory 清除对话历史
func (s *Service) ClearHistory(userID uint) {
	delete(s.chatHistories, userID)
}

// DiagnosisRequest 诊断请求
type DiagnosisRequest struct {
	ClusterID    uint   `json:"cluster_id" binding:"required"`
	ResourceType string `json:"resource_type" binding:"required"` // deployment, pod, node, etc.
	ResourceName string `json:"resource_name" binding:"required"`
	Namespace    string `json:"namespace"`
	Problem      string `json:"problem" binding:"required"` // 问题描述
}

// DiagnosisResponse 诊断响应
type DiagnosisResponse struct {
	Analysis   string   `json:"analysis"`    // 原因分析
	Steps      []string `json:"steps"`       // 排查步骤
	Solutions  []string `json:"solutions"`   // 解决方案
	Prevention []string `json:"prevention"`  // 预防措施
	Commands   []string `json:"commands"`    // 相关命令
}

// Diagnose 智能诊断
func (s *Service) Diagnose(ctx context.Context, req *DiagnosisRequest) (*DiagnosisResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured. Please set LLM API key in config")
	}

	// 获取资源上下文
	resourceContext, err := s.getResourceContext(req.ClusterID, req.ResourceType, req.ResourceName, req.Namespace)
	if err != nil {
		resourceContext = map[string]interface{}{
			"error": fmt.Sprintf("无法获取资源信息: %v", err),
		}
	}

	// 构建诊断消息
	messages := llm.BuildDiagnosisMessages(
		req.ResourceType,
		req.ResourceName,
		req.Namespace,
		req.Problem,
		resourceContext,
	)

	// 调用LLM
	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM diagnosis failed: %w", err)
	}

	// 解析响应
	diagnosis := s.parseDiagnosisResponse(resp.Content)

	return diagnosis, nil
}

// getClusterContext 获取集群上下文
func (s *Service) getClusterContext(clusterID uint) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	info, err := k8s.Manager.GetClusterInfo(clusterID)
	if err != nil {
		return "", err
	}

	// 获取节点信息
	_, _ = client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// 获取Pods
	pods, _ := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// 统计
	runningPods := 0
	pendingPods := 0
	failedPods := 0
	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case "Running":
			runningPods++
		case "Pending":
			pendingPods++
		case "Failed":
			failedPods++
		}
	}

	context := fmt.Sprintf(`集群版本: %s
节点数量: %d
CPU容量: %s
内存容量: %s
Pod总数: %d (运行中: %d, 等待中: %d, 失败: %d)`,
		info.Version,
		info.NodeCount,
		info.CPUCapacity,
		info.MemCapacity,
		len(pods.Items),
		runningPods,
		pendingPods,
		failedPods,
	)

	return context, nil
}

// getResourceContext 获取资源上下文
func (s *Service) getResourceContext(clusterID uint, resourceType, resourceName, namespace string) (map[string]interface{}, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	result := make(map[string]interface{})

	switch resourceType {
	case "pod":
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		result["status"] = pod.Status.Phase
		result["node"] = pod.Spec.NodeName
		result["containers"] = len(pod.Spec.Containers)
		result["restarts"] = 0
		for _, cs := range pod.Status.ContainerStatuses {
			result["restarts"] = result["restarts"].(int) + int(cs.RestartCount)
		}
		result["labels"] = pod.Labels
		result["conditions"] = pod.Status.Conditions

		// 获取最近的事件
		events, _ := client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.name=%s", resourceName),
		})
		if events != nil && len(events.Items) > 0 {
			recentEvents := make([]map[string]string, 0)
			for i, e := range events.Items {
				if i >= 5 {
					break
				}
				recentEvents = append(recentEvents, map[string]string{
					"type":    e.Type,
					"reason":  e.Reason,
					"message": e.Message,
					"time":    e.LastTimestamp.Format(time.RFC3339),
				})
			}
			result["recent_events"] = recentEvents
		}

	case "deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		result["replicas"] = *deploy.Spec.Replicas
		result["ready"] = deploy.Status.ReadyReplicas
		result["available"] = deploy.Status.AvailableReplicas
		result["labels"] = deploy.Labels
		result["selector"] = deploy.Spec.Selector.MatchLabels

	case "node":
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		result["status"] = node.Status.Conditions
		result["capacity"] = node.Status.Capacity
		result["allocatable"] = node.Status.Allocatable
		result["node_info"] = node.Status.NodeInfo
	}

	return result, nil
}

// parseDiagnosisResponse 解析诊断响应
func (s *Service) parseDiagnosisResponse(content string) *DiagnosisResponse {
	// 简单解析，实际可以用更复杂的NLP
	diagnosis := &DiagnosisResponse{
		Analysis: content,
	}

	// 尝试提取结构化信息
	// 这里简化处理，实际可以用正则或LLM进一步解析
	return diagnosis
}

// saveChatHistory 保存对话历史
func (s *Service) saveChatHistory(userID uint, userMsg, assistantMsg string) {
	// TODO: 保存到 chat_messages 表
}

// ==================== AI 驱动功能 ====================

// ExplainRequest 划词解释请求
type ExplainRequest struct {
	Text      string `json:"text" binding:"required"`
	ClusterID uint   `json:"cluster_id"`
	Context   string `json:"context"` // 上下文信息
}

// ExplainResponse 划词解释响应
type ExplainResponse struct {
	Explanation string `json:"explanation"`
	Examples    string `json:"examples,omitempty"`
	References  string `json:"references,omitempty"`
}

// ExplainText 划词解释
func (s *Service) ExplainText(ctx context.Context, req *ExplainRequest) (*ExplainResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	systemPrompt := `你是 KubePilot AI 助手，专门解释 Kubernetes 相关的概念、命令、配置和错误信息。

解释规则：
1. 如果是 K8S 概念，给出清晰的定义和用途说明
2. 如果是 kubectl 命令，解释每个参数的含义
3. 如果是 YAML 配置，逐字段解释
4. 如果是错误信息，解释错误原因和解决方法
5. 如果是普通技术术语，给出技术解释

回复格式：
- 使用中文
- 使用 Markdown 格式
- 适当给出示例
- 保持简洁明了`

	userPrompt := fmt.Sprintf("请解释以下内容：\n\n```\n%s\n```", req.Text)
	if req.Context != "" {
		userPrompt += fmt.Sprintf("\n\n上下文信息：\n%s", req.Context)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("explanation failed: %w", err)
	}

	return &ExplainResponse{
		Explanation: resp.Content,
	}, nil
}

// ExplainTextStream 流式划词解释
func (s *Service) ExplainTextStream(ctx context.Context, req *ExplainRequest) (<-chan llm.StreamChunk, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	systemPrompt := `你是 KubePilot AI 助手，专门解释 Kubernetes 相关的概念、命令、配置和错误信息。
解释要简洁明了，使用中文和 Markdown 格式。`

	userPrompt := fmt.Sprintf("请解释以下内容：\n\n```\n%s\n```", req.Text)
	if req.Context != "" {
		userPrompt += fmt.Sprintf("\n\n上下文信息：\n%s", req.Context)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	return s.llmClient.ChatStream(ctx, &llm.ChatRequest{
		Messages: messages,
		Stream:   true,
	})
}

// ResourceGuideRequest 资源指南请求
type ResourceGuideRequest struct {
	ClusterID    uint   `json:"cluster_id" binding:"required"`
	ResourceType string `json:"resource_type" binding:"required"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace"`
}

// ResourceGuideResponse 资源指南响应
type ResourceGuideResponse struct {
	Overview     string   `json:"overview"`      // 概述
	Status       string   `json:"status"`        // 状态分析
	HealthScore  int      `json:"health_score"`  // 健康评分 0-100
	Suggestions  []string `json:"suggestions"`   // 优化建议
	Operations   []string `json:"operations"`    // 常用操作
	Warnings     []string `json:"warnings"`      // 潜在风险
}

// GetResourceGuide 资源指南
func (s *Service) GetResourceGuide(ctx context.Context, req *ResourceGuideRequest) (*ResourceGuideResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	// 获取资源详细信息
	resourceData, err := s.getResourceDetail(ctx, req.ClusterID, req.ResourceType, req.ResourceName, req.Namespace)
	if err != nil {
		resourceData = fmt.Sprintf("无法获取资源信息: %v", err)
	}

	systemPrompt := `你是 KubePilot AI 助手，负责分析 Kubernetes 资源状态并提供运维指南。

请根据提供的资源信息，给出：
1. 资源概述
2. 当前状态分析
3. 健康评分（0-100）
4. 优化建议
5. 常用操作命令
6. 潜在风险警告

回复要求：
- 使用中文
- 基于真实数据分析
- 给出具体可操作的建议
- 使用 JSON 格式返回结构化数据`

	userPrompt := fmt.Sprintf(`请分析以下 K8S 资源并提供运维指南：

资源类型: %s
资源名称: %s
命名空间: %s

资源详细信息:
%s

请以 JSON 格式返回：
{
  "overview": "资源概述",
  "status": "状态分析",
  "health_score": 85,
  "suggestions": ["建议1", "建议2"],
  "operations": ["常用命令1", "常用命令2"],
  "warnings": ["风险1", "风险2"]
}`, req.ResourceType, req.ResourceName, req.Namespace, resourceData)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("resource guide failed: %w", err)
	}

	// 解析 JSON 响应
	guide := &ResourceGuideResponse{}
	content := resp.Content

	// 尝试提取 JSON
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := content[jsonStart : jsonEnd+1]
		if err := parseJSON(jsonStr, guide); err != nil {
			// JSON 解析失败，返回原始内容
			guide.Overview = content
			guide.HealthScore = 50
		}
	} else {
		guide.Overview = content
		guide.HealthScore = 50
	}

	return guide, nil
}

// getResourceDetail 获取资源详细信息
func (s *Service) getResourceDetail(ctx context.Context, clusterID uint, resourceType, resourceName, namespace string) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", err
	}

	var result strings.Builder

	switch resourceType {
	case "pod":
		if resourceName != "" {
			pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, resourceName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Pod: %s\n", pod.Name))
			result.WriteString(fmt.Sprintf("Status: %s\n", pod.Status.Phase))
			result.WriteString(fmt.Sprintf("Node: %s\n", pod.Spec.NodeName))
			result.WriteString(fmt.Sprintf("IP: %s\n", pod.Status.PodIP))
			result.WriteString("Containers:\n")
			for _, c := range pod.Spec.Containers {
				result.WriteString(fmt.Sprintf("  - %s (%s)\n", c.Name, c.Image))
			}
			result.WriteString("Container Statuses:\n")
			for _, cs := range pod.Status.ContainerStatuses {
				result.WriteString(fmt.Sprintf("  - %s: restarts=%d, ready=%v\n", cs.Name, cs.RestartCount, cs.Ready))
			}
			events, _ := client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s", resourceName),
			})
			if events != nil && len(events.Items) > 0 {
				result.WriteString("Recent Events:\n")
				for i, e := range events.Items {
					if i >= 5 {
						break
					}
					result.WriteString(fmt.Sprintf("  - [%s] %s: %s\n", e.Type, e.Reason, e.Message))
				}
			}
		} else {
			pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Total Pods: %d\n", len(pods.Items)))
			for i, p := range pods.Items {
				if i >= 10 {
					result.WriteString("...\n")
					break
				}
				result.WriteString(fmt.Sprintf("- %s/%s: %s\n", p.Namespace, p.Name, p.Status.Phase))
			}
		}

	case "deployment":
		if resourceName != "" {
			deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			replicas := int32(0)
			if deploy.Spec.Replicas != nil {
				replicas = *deploy.Spec.Replicas
			}
			result.WriteString(fmt.Sprintf("Deployment: %s\n", deploy.Name))
			result.WriteString(fmt.Sprintf("Replicas: %d/%d\n", deploy.Status.ReadyReplicas, replicas))
			result.WriteString(fmt.Sprintf("Strategy: %s\n", deploy.Spec.Strategy.Type))
			result.WriteString("Containers:\n")
			for _, c := range deploy.Spec.Template.Spec.Containers {
				result.WriteString(fmt.Sprintf("  - %s (%s)\n", c.Name, c.Image))
			}
		} else {
			deploys, err := client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Total Deployments: %d\n", len(deploys.Items)))
			for i, d := range deploys.Items {
				if i >= 10 {
					break
				}
				result.WriteString(fmt.Sprintf("- %s/%s: %d/%d ready\n", d.Namespace, d.Name, d.Status.ReadyReplicas, int32p(d.Spec.Replicas)))
			}
		}

	case "service":
		if resourceName != "" {
			svc, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, resourceName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Service: %s\n", svc.Name))
			result.WriteString(fmt.Sprintf("Type: %s\n", svc.Spec.Type))
			result.WriteString(fmt.Sprintf("ClusterIP: %s\n", svc.Spec.ClusterIP))
			result.WriteString("Ports:\n")
			for _, p := range svc.Spec.Ports {
				result.WriteString(fmt.Sprintf("  - %d -> %d (%s)\n", p.Port, p.TargetPort.IntValue(), p.Protocol))
			}
		} else {
			svcs, err := client.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Total Services: %d\n", len(svcs.Items)))
			for i, svc := range svcs.Items {
				if i >= 10 {
					break
				}
				result.WriteString(fmt.Sprintf("- %s/%s: %s\n", svc.Namespace, svc.Name, svc.Spec.Type))
			}
		}

	case "node":
		if resourceName != "" {
			node, err := client.Clientset.CoreV1().Nodes().Get(ctx, resourceName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Node: %s\n", node.Name))
			result.WriteString(fmt.Sprintf("Status: %s\n", getNodeStatus(node)))
			result.WriteString(fmt.Sprintf("CPU: %s\n", node.Status.Capacity.Cpu().String()))
			result.WriteString(fmt.Sprintf("Memory: %s\n", node.Status.Capacity.Memory().String()))
			result.WriteString(fmt.Sprintf("OS: %s\n", node.Status.NodeInfo.OSImage))
			result.WriteString(fmt.Sprintf("Kubelet: %s\n", node.Status.NodeInfo.KubeletVersion))
		} else {
			nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				return "", err
			}
			result.WriteString(fmt.Sprintf("Total Nodes: %d\n", len(nodes.Items)))
			for _, n := range nodes.Items {
				result.WriteString(fmt.Sprintf("- %s: %s\n", n.Name, getNodeStatus(&n)))
			}
		}
	}

	return result.String(), nil
}

func int32p(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func getNodeStatus(node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// parseJSON 解析 JSON
func parseJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

// TranslateYAMLRequest YAML 翻译请求
type TranslateYAMLRequest struct {
	YAML      string `json:"yaml" binding:"required"`
	Direction string `json:"direction"` // "to_chinese" or "to_english"
}

// TranslateYAMLResponse YAML 翻译响应
type TranslateYAMLResponse struct {
	Translated string `json:"translated"`
	Notes      string `json:"notes,omitempty"`
}

// TranslateYAML YAML 翻译
func (s *Service) TranslateYAML(ctx context.Context, req *TranslateYAMLRequest) (*TranslateYAMLResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	direction := "to_chinese"
	if req.Direction != "" {
		direction = req.Direction
	}

	systemPrompt := `你是 KubePilot AI 助手，专门翻译 Kubernetes YAML 配置文件。

翻译规则：
1. 保留原始 YAML 结构
2. 将字段名和注释翻译为目标语言
3. 添加中文注释解释每个字段的作用
4. 保留值不翻译（如资源名称、镜像名等）
5. 对于 K8S 特有术语，给出准确的中文翻译

回复格式：直接返回翻译后的 YAML，不要添加额外说明`

	var userPrompt string
	if direction == "to_chinese" {
		userPrompt = fmt.Sprintf("请将以下 YAML 配置翻译成中文（添加中文注释）：\n\n```yaml\n%s\n```", req.YAML)
	} else {
		userPrompt = fmt.Sprintf("请将以下中文 YAML 配置翻译成英文：\n\n```yaml\n%s\n```", req.YAML)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("translation failed: %w", err)
	}

	return &TranslateYAMLResponse{
		Translated: resp.Content,
	}, nil
}

// AnalyzeDescribeRequest Describe 解读请求
type AnalyzeDescribeRequest struct {
	ClusterID    uint   `json:"cluster_id"`
	ResourceType string `json:"resource_type" binding:"required"`
	ResourceName string `json:"resource_name" binding:"required"`
	Namespace    string `json:"namespace"`
	Describe     string `json:"describe"` // 如果为空，自动执行 kubectl describe
}

// AnalyzeDescribeResponse Describe 解读响应
type AnalyzeDescribeResponse struct {
	Summary      string   `json:"summary"`       // 摘要
	KeyInfo      []string `json:"key_info"`      // 关键信息
	Issues       []string `json:"issues"`        // 发现的问题
	Suggestions  []string `json:"suggestions"`   // 建议
	Commands     []string `json:"commands"`      // 相关命令
}

// AnalyzeDescribe Describe 解读
func (s *Service) AnalyzeDescribe(ctx context.Context, req *AnalyzeDescribeRequest) (*AnalyzeDescribeResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	describeOutput := req.Describe

	// 如果没有提供 describe 内容，自动执行
	if describeOutput == "" && req.ClusterID > 0 {
		output, err := s.executeDescribe(ctx, req.ClusterID, req.ResourceType, req.ResourceName, req.Namespace)
		if err != nil {
			describeOutput = fmt.Sprintf("无法获取 describe 信息: %v", err)
		} else {
			describeOutput = output
		}
	}

	systemPrompt := `你是 KubePilot AI 助手，专门解读 kubectl describe 的输出。

请分析 describe 输出并提供：
1. 资源摘要
2. 关键信息提取
3. 发现的问题或异常
4. 优化建议
5. 相关排查命令

回复要求：
- 使用中文
- 重点关注异常状态、错误事件、资源限制等
- 给出具体可操作的建议
- 使用 JSON 格式返回`

	userPrompt := fmt.Sprintf(`请解读以下 kubectl describe 输出：

资源类型: %s
资源名称: %s
命名空间: %s

Describe 输出:
%s

请以 JSON 格式返回：
{
  "summary": "资源摘要",
  "key_info": ["关键信息1", "关键信息2"],
  "issues": ["问题1", "问题2"],
  "suggestions": ["建议1", "建议2"],
  "commands": ["命令1", "命令2"]
}`, req.ResourceType, req.ResourceName, req.Namespace, describeOutput)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("describe analysis failed: %w", err)
	}

	// 解析响应
	result := &AnalyzeDescribeResponse{}
	content := resp.Content
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := content[jsonStart : jsonEnd+1]
		if err := parseJSON(jsonStr, result); err != nil {
			result.Summary = content
		}
	} else {
		result.Summary = content
	}

	return result, nil
}

// executeDescribe 执行 kubectl describe
func (s *Service) executeDescribe(ctx context.Context, clusterID uint, resourceType, resourceName, namespace string) (string, error) {
	args := []string{"describe", resourceType, resourceName}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	result, err := s.ExecuteKubectl(ctx, clusterID, args)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// AnalyzeLogsRequest 日志问诊请求
type AnalyzeLogsRequest struct {
	ClusterID    uint   `json:"cluster_id" binding:"required"`
	ResourceType string `json:"resource_type"` // pod, deployment
	ResourceName string `json:"resource_name" binding:"required"`
	Namespace    string `json:"namespace" binding:"required"`
	Container    string `json:"container"`
	Lines        int    `json:"lines"` // 日志行数
	Logs         string `json:"logs"`  // 如果为空，自动获取
}

// AnalyzeLogsResponse 日志问诊响应
type AnalyzeLogsResponse struct {
	Summary      string   `json:"summary"`       // 日志摘要
	Patterns     []string `json:"patterns"`      // 发现的模式
	Errors       []string `json:"errors"`        // 错误信息
	RootCause    string   `json:"root_cause"`    // 根因分析
	Solutions    []string `json:"solutions"`     // 解决方案
	Commands     []string `json:"commands"`      // 排查命令
	Severity     string   `json:"severity"`      // 严重程度: low, medium, high, critical
}

// AnalyzeLogs 日志问诊
func (s *Service) AnalyzeLogs(ctx context.Context, req *AnalyzeLogsRequest) (*AnalyzeLogsResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	logContent := req.Logs

	// 如果没有提供日志，自动获取
	if logContent == "" && req.ClusterID > 0 {
		lines := req.Lines
		if lines <= 0 {
			lines = 100
		}
		logs, err := s.getPodLogs(ctx, req.ClusterID, req.Namespace, req.ResourceName, req.Container, lines)
		if err != nil {
			logContent = fmt.Sprintf("无法获取日志: %v", err)
		} else {
			logContent = logs
		}
	}

	// 截断过长的日志
	if len(logContent) > 10000 {
		logContent = logContent[:10000] + "\n... (日志已截断)"
	}

	systemPrompt := `你是 KubePilot AI 助手，专门分析 Kubernetes Pod 日志并诊断问题。

请分析日志内容并提供：
1. 日志摘要
2. 发现的模式（如重复错误、异常模式等）
3. 错误信息提取
4. 根因分析
5. 解决方案
6. 排查命令
7. 严重程度评估

回复要求：
- 使用中文
- 重点关注错误、异常、性能问题
- 给出具体可操作的解决方案
- 使用 JSON 格式返回`

	userPrompt := fmt.Sprintf(`请分析以下 Pod 日志：

资源名称: %s
命名空间: %s
容器: %s

日志内容:
%s

请以 JSON 格式返回：
{
  "summary": "日志摘要",
  "patterns": ["模式1", "模式2"],
  "errors": ["错误1", "错误2"],
  "root_cause": "根因分析",
  "solutions": ["解决方案1", "解决方案2"],
  "commands": ["排查命令1", "排查命令2"],
  "severity": "high"
}`, req.ResourceName, req.Namespace, req.Container, logContent)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("log analysis failed: %w", err)
	}

	// 解析响应
	result := &AnalyzeLogsResponse{}
	content := resp.Content
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := content[jsonStart : jsonEnd+1]
		if err := parseJSON(jsonStr, result); err != nil {
			result.Summary = content
			result.Severity = "medium"
		}
	} else {
		result.Summary = content
		result.Severity = "medium"
	}

	return result, nil
}

// getPodLogs 获取 Pod 日志
func (s *Service) getPodLogs(ctx context.Context, clusterID uint, namespace, podName, container string, lines int) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", err
	}

	// 构建日志请求
	logOpts := &corev1.PodLogOptions{
		Follow:   false,
		Previous: false,
	}
	if lines > 0 {
		tailLines := int64(lines)
		logOpts.TailLines = &tailLines
	}
	if container != "" {
		logOpts.Container = container
	}

	req := client.Clientset.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	// 读取日志
	logBytes, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logBytes), nil
}

// ==================== AI Agent ====================

// AgentChatResponse Agent对话响应
type AgentChatResponse struct {
	Content string                   `json:"content"`
	Actions []AgentActionInfo        `json:"actions,omitempty"`
}

// AgentActionInfo Agent动作信息
type AgentActionInfo struct {
	ID           uint   `json:"id"`
	ActionType   string `json:"action_type"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace"`
	Description  string `json:"description"`
	NeedConfirm  bool   `json:"need_confirm"`
}

// AgentChat Agent对话
func (s *Service) AgentChat(ctx context.Context, userID uint, clusterID uint, message string, conversationID uint) (*AgentChatResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	// 获取集群上下文
	clusterContext, _ := s.getClusterContext(clusterID)

	// 获取对话历史
	historyMessages := s.getConversationHistory(conversationID, 20)

	// 尝试直接查询真实数据（如果用户在查询资源）
	realData := s.queryRealData(ctx, clusterID, message)

	// 构建Agent系统提示
	systemPrompt := `你是 KubePilot AI Agent，一个专业的 Kubernetes 运维助手。

## 核心规则

1. **必须使用真实数据** - 系统会提供集群的真实数据，你必须基于这些数据回答，绝对不能编造
2. **删除前必须查询** - 执行删除操作前，必须先确认资源的真实名称
3. **使用准确的资源名称** - 从系统提供的数据中获取资源名称，不要猜测

## 操作格式

对于需要执行的操作，在回复末尾包含 action 代码块。每个操作一个代码块，多个操作多个代码块。

### create_deployment 格式：
` + "```" + `action
{"action": "create_deployment", "namespace": "default", "name": "my-app", "image": "nginx:latest", "replicas": 2, "ports": [80]}
` + "```" + `

### create_service 格式：
` + "```" + `action
{"action": "create_service", "namespace": "default", "name": "my-app-svc", "service_type": "NodePort", "selector": {"app": "my-app"}, "port": 80, "target_port": 80, "node_port": 30080}
` + "```" + `

### delete_deployment 格式：
` + "```" + `action
{"action": "delete_deployment", "namespace": "default", "name": "my-app"}
` + "```" + `

### delete_service 格式：
` + "```" + `action
{"action": "delete_service", "namespace": "default", "name": "my-app-svc"}
` + "```" + `

### delete_pod 格式：
` + "```" + `action
{"action": "delete_pod", "namespace": "default", "name": "my-app-xxx"}
` + "```" + `

### scale_deployment 格式：
` + "```" + `action
{"action": "scale_deployment", "namespace": "default", "name": "my-app", "replicas": 3}
` + "```" + `

## 重要提示

- 创建 Deployment 时必须包含 image 字段
- 创建 Service 时必须包含 selector、port、target_port 字段
- 使用 NodePort 类型时，node_port 范围是 30000-32767
- 多个操作时，每个操作单独一个 action 代码块

## 示例

用户：创建一个 nginx deployment，2个副本，然后创建 NodePort service 对外暴露 30880 端口

回复：我将为您创建 nginx deployment 和 service：

` + "```" + `action
{"action": "create_deployment", "namespace": "default", "name": "nginx-deployment", "image": "nginx:latest", "replicas": 2, "ports": [80]}
` + "```" + `

` + "```" + `action
{"action": "create_service", "namespace": "default", "name": "nginx-service", "service_type": "NodePort", "selector": {"app": "nginx-deployment"}, "port": 80, "target_port": 80, "node_port": 30880}
` + "```" + `

请确认是否执行？

当前集群数据：
` + clusterContext + `

请用中文回复。`

	// 构建消息列表（包含历史）
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, historyMessages...)

	// 如果有真实数据，添加到消息中
	if realData != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "以下是查询到的真实集群数据，请基于这些数据回答：\n\n" + realData,
		})
	}

	messages = append(messages, llm.Message{Role: "user", Content: message})

	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM chat failed: %w", err)
	}

	// 解析响应，判断是否包含需要确认的操作
	actions := s.parseAgentActions(resp.Content, clusterID)

	return &AgentChatResponse{
		Content: resp.Content,
		Actions: actions,
	}, nil
}

// queryRealData 根据用户查询获取真实数据
func (s *Service) queryRealData(ctx context.Context, clusterID uint, message string) string {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return ""
	}

	message = strings.ToLower(message)
	result := ""

	// 判断是否需要查询（删除、查看、查询、列出等操作都需要真实数据）
	needQuery := strings.Contains(message, "删除") || strings.Contains(message, "delete") ||
		strings.Contains(message, "查看") || strings.Contains(message, "查询") ||
		strings.Contains(message, "列出") || strings.Contains(message, "list") ||
		strings.Contains(message, "所有") || strings.Contains(message, "全部")

	// 查询 Services（如果用户提到 svc、service、服务 或需要删除操作）
	if strings.Contains(message, "svc") || strings.Contains(message, "service") ||
		strings.Contains(message, "服务") || needQuery {
		services, err := client.Clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
		if err == nil {
			result += "Service 列表:\n"
			result += "命名空间 | 名称 | 类型 | ClusterIP | 端口\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, svc := range services.Items {
				ports := ""
				for i, p := range svc.Spec.Ports {
					if i > 0 {
						ports += ", "
					}
					ports += fmt.Sprintf("%d", p.Port)
					if p.NodePort > 0 {
						ports += fmt.Sprintf(":%d", p.NodePort)
					}
				}
				result += fmt.Sprintf("%s | %s | %s | %s | %s\n",
					svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP, ports)
			}
			result += "\n"
		}
	}

	// 查询 Deployments（如果用户提到 deploy、deployment、部署 或需要删除操作）
	if strings.Contains(message, "deploy") || strings.Contains(message, "deployment") ||
		strings.Contains(message, "部署") || strings.Contains(message, "nginx") || needQuery {
		deployments, err := client.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err == nil {
			result += "Deployment 列表:\n"
			result += "命名空间 | 名称 | 副本 | 就绪 | 镜像\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, d := range deployments.Items {
				images := ""
				for i, c := range d.Spec.Template.Spec.Containers {
					if i > 0 {
						images += ", "
					}
					images += c.Image
				}
				replicas := int32(0)
				if d.Spec.Replicas != nil {
					replicas = *d.Spec.Replicas
				}
				result += fmt.Sprintf("%s | %s | %d | %d | %s\n",
					d.Namespace, d.Name, replicas, d.Status.ReadyReplicas, images)
			}
			result += "\n"
		}
	}

	// 查询 Pods（如果用户提到 pod、容器 或需要删除操作）
	if strings.Contains(message, "pod") || strings.Contains(message, "容器") || needQuery {
		pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err == nil {
			result += "Pod 列表:\n"
			result += "命名空间 | 名称 | 状态 | 重启次数 | 节点\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, pod := range pods.Items {
				restarts := int32(0)
				for _, cs := range pod.Status.ContainerStatuses {
					restarts += cs.RestartCount
				}
				result += fmt.Sprintf("%s | %s | %s | %d | %s\n",
					pod.Namespace, pod.Name, pod.Status.Phase, restarts, pod.Spec.NodeName)
			}
		}
	}

	return result
}

// getConversationHistory 获取对话历史
func (s *Service) getConversationHistory(conversationID uint, limit int) []llm.Message {
	if conversationID == 0 {
		return nil
	}

	var messages []model.ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil
	}

	// 反转顺序（从旧到新）
	result := make([]llm.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		result = append(result, llm.Message{
			Role:    messages[i].Role,
			Content: messages[i].Content,
		})
	}

	return result
}

// parseAgentActions 解析Agent动作
func (s *Service) parseAgentActions(content string, clusterID uint) []AgentActionInfo {
	actions := make([]AgentActionInfo, 0)

	// 检测是否包含确认提示
	confirmKeywords := []string{
		"请确认是否执行",
		"是否执行此操作",
		"确认执行",
		"请确认",
	}

	needsConfirm := false
	for _, keyword := range confirmKeywords {
		if contains(content, keyword) {
			needsConfirm = true
			break
		}
	}

	if !needsConfirm {
		return actions
	}

	// 检测操作类型
	operationKeywords := map[string]string{
		"创建": "create",
		"部署": "create",
		"删除": "delete",
		"更新": "update",
		"修改": "update",
		"扩容": "update",
		"缩容": "update",
		"重启": "update",
		"回滚": "update",
	}

	// 检测资源类型
	resourceKeywords := map[string]string{
		"Deployment":  "deployments",
		"deployment":  "deployments",
		"Pod":         "pods",
		"pod":         "pods",
		"Service":     "services",
		"service":     "services",
		"ConfigMap":   "configmaps",
		"configmap":   "configmaps",
		"Secret":      "secrets",
		"secret":      "secrets",
		"Ingress":     "ingresses",
		"ingress":     "ingresses",
		"Namespace":   "namespaces",
		"namespace":   "namespaces",
		"Node":        "nodes",
		"node":        "nodes",
		"PV":          "pvs",
		"PVC":         "pvcs",
	}

	actionType := "execute"
	resourceType := "unknown"

	for keyword, at := range operationKeywords {
		if contains(content, keyword) {
			actionType = at
			break
		}
	}

	for keyword, rt := range resourceKeywords {
		if contains(content, keyword) {
			resourceType = rt
			break
		}
	}

	// 提取操作描述（取第一行或前100个字符）
	description := content
	if len(content) > 100 {
		description = content[:100] + "..."
	}

	actions = append(actions, AgentActionInfo{
		ActionType:   actionType,
		ResourceType: resourceType,
		Description:  description,
		NeedConfirm:  true,
	})

	return actions
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ExecuteAgentAction 执行Agent动作
func (s *Service) ExecuteAgentAction(ctx context.Context, action *model.AgentAction) (string, error) {
	client, err := k8s.Manager.GetClient(action.ClusterID)
	if err != nil {
		return "", fmt.Errorf("cluster not connected: %w", err)
	}

	// 根据动作类型执行
	switch action.ActionType {
	case "query":
		return s.executeQueryAction(ctx, client, action)
	case "create":
		return s.executeCreateAction(ctx, client, action)
	case "delete":
		return s.executeDeleteAction(ctx, client, action)
	default:
		return "", fmt.Errorf("unsupported action type: %s", action.ActionType)
	}
}

// executeQueryAction 执行查询动作
func (s *Service) executeQueryAction(ctx context.Context, client *k8s.ClusterClient, action *model.AgentAction) (string, error) {
	switch action.ResourceType {
	case "pods":
		pods, err := client.Clientset.CoreV1().Pods(action.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Found %d pods", len(pods.Items)), nil
	case "deployments":
		deploys, err := client.Clientset.AppsV1().Deployments(action.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Found %d deployments", len(deploys.Items)), nil
	default:
		return "", fmt.Errorf("unsupported resource type: %s", action.ResourceType)
	}
}

// executeCreateAction 执行创建动作
func (s *Service) executeCreateAction(ctx context.Context, client *k8s.ClusterClient, action *model.AgentAction) (string, error) {
	// TODO: 实现创建逻辑
	return "Create action executed", nil
}

// executeDeleteAction 执行删除动作
func (s *Service) executeDeleteAction(ctx context.Context, client *k8s.ClusterClient, action *model.AgentAction) (string, error) {
	// TODO: 实现删除逻辑
	return "Delete action executed", nil
}
