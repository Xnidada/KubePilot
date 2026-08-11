package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
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
	Problem      string `json:"problem"` // 问题描述（可选）
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

	problem := req.Problem

	// 如果没有提供问题描述，自动获取资源的 describe 信息
	if problem == "" {
		describeInfo, err := s.getResourceDescribeInfo(ctx, req.ClusterID, req.ResourceType, req.ResourceName, req.Namespace)
		if err == nil && describeInfo != "" {
			problem = fmt.Sprintf("请分析以下 K8S 资源的 describe 信息，找出问题并给出解决方案：\n\n%s", describeInfo)
		} else {
			problem = fmt.Sprintf("请分析 %s 资源 %s 可能存在的问题", req.ResourceType, req.ResourceName)
		}
	}

	messages := []llm.Message{
		{Role: "system", Content: "你是 K8S 运维专家。请用中文回答，分析问题原因并给出解决方案和排查命令。回答要简洁明了。"},
		{Role: "user", Content: problem},
	}

	// 调用LLM
	resp, err := s.llmClient.Chat(ctx, &llm.ChatRequest{
		Messages:  messages,
		MaxTokens: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM diagnosis failed: %w", err)
	}

	// 解析响应
	diagnosis := s.parseDiagnosisResponse(resp.Content)
	return diagnosis, nil
}

// getResourceDescribeInfo 获取资源的 describe 信息
func (s *Service) getResourceDescribeInfo(ctx context.Context, clusterID uint, resourceType, resourceName, namespace string) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", err
	}

	var result string

	switch resourceType {
	case "pod":
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result = formatPodDescribe(pod)

	case "deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result = formatDeploymentDescribe(deploy)

	case "service":
		svc, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result = formatServiceDescribe(svc)

	case "node":
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result = formatNodeDescribe(node)

	default:
		return "", fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	return result, nil
}

// formatPodDescribe 格式化 Pod describe 信息
func formatPodDescribe(pod *corev1.Pod) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name: %s\n", pod.Name))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", pod.Namespace))
	sb.WriteString(fmt.Sprintf("Status: %s\n", pod.Status.Phase))
	sb.WriteString(fmt.Sprintf("Node: %s\n", pod.Spec.NodeName))
	sb.WriteString(fmt.Sprintf("IP: %s\n", pod.Status.PodIP))
	sb.WriteString(fmt.Sprintf("Start Time: %s\n", pod.Status.StartTime))

	// Labels
	sb.WriteString("Labels:\n")
	for k, v := range pod.Labels {
		sb.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
	}

	// Containers
	sb.WriteString("Containers:\n")
	for _, c := range pod.Spec.Containers {
		sb.WriteString(fmt.Sprintf("  %s:\n", c.Name))
		sb.WriteString(fmt.Sprintf("    Image: %s\n", c.Image))
		if c.Resources.Requests != nil {
			sb.WriteString(fmt.Sprintf("    Requests: cpu=%s, memory=%s\n",
				c.Resources.Requests.Cpu().String(),
				c.Resources.Requests.Memory().String()))
		}
		if c.Resources.Limits != nil {
			sb.WriteString(fmt.Sprintf("    Limits: cpu=%s, memory=%s\n",
				c.Resources.Limits.Cpu().String(),
				c.Resources.Limits.Memory().String()))
		}
	}

	// Container Statuses
	sb.WriteString("Container Statuses:\n")
	for _, cs := range pod.Status.ContainerStatuses {
		sb.WriteString(fmt.Sprintf("  %s:\n", cs.Name))
		sb.WriteString(fmt.Sprintf("    Ready: %v\n", cs.Ready))
		sb.WriteString(fmt.Sprintf("    Restart Count: %d\n", cs.RestartCount))
		if cs.State.Waiting != nil {
			sb.WriteString(fmt.Sprintf("    Waiting: %s - %s\n", cs.State.Waiting.Reason, cs.State.Waiting.Message))
		}
		if cs.State.Running != nil {
			sb.WriteString(fmt.Sprintf("    Running: started at %s\n", cs.State.Running.StartedAt.Time.Format(time.RFC3339)))
		}
		if cs.State.Terminated != nil {
			sb.WriteString(fmt.Sprintf("    Terminated: %s (exit code %d) - %s\n",
				cs.State.Terminated.Reason, cs.State.Terminated.ExitCode, cs.State.Terminated.Message))
		}
		if cs.LastTerminationState.Terminated != nil {
			lt := cs.LastTerminationState.Terminated
			sb.WriteString(fmt.Sprintf("    Last Termination: %s (exit code %d) - %s\n",
				lt.Reason, lt.ExitCode, lt.Message))
		}
	}

	// Conditions
	sb.WriteString("Conditions:\n")
	for _, cond := range pod.Status.Conditions {
		sb.WriteString(fmt.Sprintf("  %s: %s (%s)\n", cond.Type, cond.Status, cond.Reason))
	}

	return sb.String()
}

// formatDeploymentDescribe 格式化 Deployment describe 信息
func formatDeploymentDescribe(deploy *appsv1.Deployment) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name: %s\n", deploy.Name))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", deploy.Namespace))

	replicas := int32(0)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}
	sb.WriteString(fmt.Sprintf("Replicas: %d/%d\n", deploy.Status.ReadyReplicas, replicas))
	sb.WriteString(fmt.Sprintf("Strategy: %s\n", deploy.Spec.Strategy.Type))

	sb.WriteString("Selector:\n")
	for k, v := range deploy.Spec.Selector.MatchLabels {
		sb.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
	}

	sb.WriteString("Containers:\n")
	for _, c := range deploy.Spec.Template.Spec.Containers {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", c.Name, c.Image))
	}

	sb.WriteString("Conditions:\n")
	for _, cond := range deploy.Status.Conditions {
		sb.WriteString(fmt.Sprintf("  %s: %s (%s)\n", cond.Type, cond.Status, cond.Reason))
	}

	return sb.String()
}

// formatServiceDescribe 格式化 Service describe 信息
func formatServiceDescribe(svc *corev1.Service) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name: %s\n", svc.Name))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", svc.Namespace))
	sb.WriteString(fmt.Sprintf("Type: %s\n", svc.Spec.Type))
	sb.WriteString(fmt.Sprintf("ClusterIP: %s\n", svc.Spec.ClusterIP))

	sb.WriteString("Ports:\n")
	for _, p := range svc.Spec.Ports {
		sb.WriteString(fmt.Sprintf("  %d -> %d (%s)\n", p.Port, p.TargetPort.IntValue(), p.Protocol))
	}

	sb.WriteString("Selector:\n")
	for k, v := range svc.Spec.Selector {
		sb.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
	}

	return sb.String()
}

// formatNodeDescribe 格式化 Node describe 信息
func formatNodeDescribe(node *corev1.Node) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name: %s\n", node.Name))
	sb.WriteString(fmt.Sprintf("CPU: %s\n", node.Status.Capacity.Cpu().String()))
	sb.WriteString(fmt.Sprintf("Memory: %s\n", node.Status.Capacity.Memory().String()))

	sb.WriteString("Conditions:\n")
	for _, cond := range node.Status.Conditions {
		sb.WriteString(fmt.Sprintf("  %s: %s (%s)\n", cond.Type, cond.Status, cond.Reason))
	}

	sb.WriteString("Addresses:\n")
	for _, addr := range node.Status.Addresses {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", addr.Type, addr.Address))
	}

	return sb.String()
}

// getResourceDescribe 获取资源的 describe 信息（极简版，避免超时）
func (s *Service) getResourceDescribe(ctx context.Context, clusterID uint, resourceType, resourceName, namespace string) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", err
	}

	var result strings.Builder

	switch resourceType {
	case "pod":
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result.WriteString(fmt.Sprintf("Pod: %s/%s\n", pod.Namespace, pod.Name))
		result.WriteString(fmt.Sprintf("Status: %s\n", pod.Status.Phase))
		result.WriteString(fmt.Sprintf("Node: %s\n", pod.Spec.NodeName))
		for _, cs := range pod.Status.ContainerStatuses {
			result.WriteString(fmt.Sprintf("Container %s: ready=%v restarts=%d\n", cs.Name, cs.Ready, cs.RestartCount))
		}

	case "deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		result.WriteString(fmt.Sprintf("Deployment: %s/%s\n", deploy.Namespace, deploy.Name))
		result.WriteString(fmt.Sprintf("Replicas: %d/%d\n", deploy.Status.ReadyReplicas, replicas))

	case "service":
		svc, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result.WriteString(fmt.Sprintf("Service: %s/%s\n", svc.Namespace, svc.Name))
		result.WriteString(fmt.Sprintf("Type: %s ClusterIP: %s\n", svc.Spec.Type, svc.Spec.ClusterIP))

	case "node":
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		result.WriteString(fmt.Sprintf("Node: %s\n", node.Name))
		for _, c := range node.Status.Conditions {
			if c.Type == "Ready" {
				result.WriteString(fmt.Sprintf("Ready: %s\n", c.Status))
			}
		}
	}

	return result.String(), nil
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

// getResourceContext 获取资源上下文（简化版，避免超时）
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

	case "deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		result["replicas"] = replicas
		result["ready"] = deploy.Status.ReadyReplicas
		result["available"] = deploy.Status.AvailableReplicas

	case "node":
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		for _, c := range node.Status.Conditions {
			if c.Type == "Ready" {
				result["ready"] = c.Status
			}
		}
		result["cpu"] = node.Status.Capacity.Cpu().String()
		result["memory"] = node.Status.Capacity.Memory().String()
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

// saveChatHistory 保存对话历史到 chat_conversations / chat_messages
func (s *Service) saveChatHistory(userID uint, userMsg, assistantMsg string) {
	if s.db == nil || userID == 0 {
		return
	}
	title := strings.TrimSpace(userMsg)
	if title == "" {
		title = "新对话"
	}
	runes := []rune(title)
	if len(runes) > 40 {
		title = string(runes[:40]) + "..."
	}

	var conv model.ChatConversation
	// Reuse the latest non-archived conversation updated within 2 hours, else create.
	cutoff := time.Now().Add(-2 * time.Hour)
	err := s.db.Where("user_id = ? AND is_archived = ? AND updated_at >= ?", userID, false, cutoff).
		Order("updated_at DESC").First(&conv).Error
	if err != nil {
		conv = model.ChatConversation{
			UserID: userID,
			Title:  title,
		}
		if err := s.db.Create(&conv).Error; err != nil {
			return
		}
	}

	msgs := []model.ChatMessage{
		{ConversationID: conv.ID, Role: "user", Content: userMsg},
		{ConversationID: conv.ID, Role: "assistant", Content: assistantMsg},
	}
	if err := s.db.Create(&msgs).Error; err != nil {
		return
	}
	_ = s.db.Model(&conv).Update("updated_at", time.Now()).Error
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
	Content        string              `json:"content"`
	Actions        []AgentActionInfo   `json:"actions,omitempty"` // compat: filled from pending_actions
	PendingActions []PendingActionInfo `json:"pending_actions,omitempty"`
	ToolTrace      []ToolTraceItem     `json:"tool_trace,omitempty"`
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
	DryRun       string `json:"dry_run,omitempty"`
}

const agentSystemPrompt = `你是 KubePilot AI Agent，只能通过【原生工具调用】查询与变更 Kubernetes 集群。

## 硬性规则
1. 涉及集群现状、资源用途、镜像、标签、控制器时，必须先调用工具（list/get/describe/events/logs），禁止仅凭名称猜测。
2. 没有本轮工具结果时，只能说明「尚未查询」或继续调用工具，不能断言资源是否存在。
3. 写操作必须：先 list/get 确认准确名称 → 再调用 stage_mutation。禁止声称已经删除/创建/扩缩容。
4. 【严禁】在回复中输出 JSON action、` + "```action" + ` 代码块、或伪工具协议。写操作只能通过 stage_mutation 工具完成。
5. 批量删除/变更：在同一次助手回合中并行多次调用 stage_mutation（每个资源一次），不要只列名单让用户确认。
6. propose_mutation 仅预览；需要用户确认时必须 stage_mutation。
7. 用户指定命名空间时，工具参数必须带上该 namespace。
8. Secret 值不可读取明文。
9. 用中文回答；最终回复只总结工具结果，并标明依据。

## 工具
- 查询：list_resources / get_resource / get_events / get_pod_logs / describe_resource
- 预览：propose_mutation
- 暂存待确认：stage_mutation（UI 确认后才会真正执行）
`

// AgentChat Agent对话（原生 Tool Calling 循环）
func (s *Service) AgentChat(ctx context.Context, userID uint, clusterID uint, message string, conversationID uint) (*AgentChatResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM service not configured")
	}

	clusterContext, _ := s.getClusterContext(clusterID)

	historyMessages := s.getConversationHistory(conversationID, 10)
	for i := range historyMessages {
		// Drop legacy fake-action fences so the model is not reinforced by old turns.
		historyMessages[i].Content = truncateRunes(sanitizeAgentHistory(historyMessages[i].Content), 2000)
	}
	// Avoid duplicating the user message already persisted by the frontend.
	if len(historyMessages) > 0 {
		last := historyMessages[len(historyMessages)-1]
		if last.Role == "user" && strings.TrimSpace(last.Content) == strings.TrimSpace(message) {
			historyMessages = historyMessages[:len(historyMessages)-1]
		}
	}

	system := agentSystemPrompt
	if strings.TrimSpace(clusterContext) != "" {
		system += "\n\n当前集群摘要（仅供参考，详情仍须用工具查询）：\n" + truncateRunes(clusterContext, 1500)
	}

	messages := []llm.Message{{Role: "system", Content: system}}
	messages = append(messages, historyMessages...)
	messages = append(messages, llm.Message{Role: "user", Content: message})

	tools := agentToolDefinitions()
	var trace []ToolTraceItem
	var pending []PendingActionInfo

	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var finalContent string
	nudgeUsed := false
	for round := 0; round < agentMaxToolRounds; round++ {
		resp, err := s.llmClient.Chat(chatCtx, &llm.ChatRequest{
			Messages:  messages,
			Tools:     tools,
			MaxTokens: 2048,
		})
		if err != nil {
			return nil, fmt.Errorf("LLM chat failed: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			finalContent = resp.Content
			// Recover from models that dump fake action JSON instead of calling tools.
			if !nudgeUsed && len(pending) == 0 && looksLikeFakeAgentActions(finalContent) {
				nudgeUsed = true
				messages = append(messages,
					llm.Message{Role: "assistant", Content: finalContent},
					llm.Message{Role: "user", Content: "禁止输出 action JSON 或伪协议。请立即用工具：先 list_resources/get_resource 核对名称，再对每个目标调用 stage_mutation。不要只列名单。"},
				)
				finalContent = ""
				continue
			}
			break
		}

		// Append assistant turn with tool_calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			name := tc.Function.Name
			args := tc.Function.Arguments
			exec := s.executeAgentTool(chatCtx, userID, clusterID, conversationID, name, args)
			trace = append(trace, ToolTraceItem{
				Name:    name,
				Args:    truncateRunes(args, 500),
				Result:  truncateRunes(exec.Content, 1500),
				IsError: exec.IsError,
			})
			if exec.Pending != nil {
				pending = append(pending, *exec.Pending)
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       name,
				Content:    exec.Content,
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
	// Strip leftover fake action fences from the visible reply when we already staged.
	if len(pending) > 0 {
		finalContent = stripFakeAgentActionBlocks(finalContent)
	}

	actions := make([]AgentActionInfo, 0, len(pending))
	for _, p := range pending {
		actions = append(actions, AgentActionInfo{
			ID:           p.ID,
			ActionType:   p.Action,
			ResourceType: p.Action,
			ResourceName: p.Name,
			Namespace:    p.Namespace,
			Description:  p.Description,
			NeedConfirm:  true,
			DryRun:       p.DryRun,
		})
	}

	return &AgentChatResponse{
		Content:        finalContent,
		Actions:        actions,
		PendingActions: pending,
		ToolTrace:      trace,
	}, nil
}

func looksLikeFakeAgentActions(content string) bool {
	c := strings.ToLower(content)
	if strings.Contains(content, "```action") {
		return true
	}
	if strings.Contains(c, `"action"`) && (strings.Contains(c, "delete_") || strings.Contains(c, "create_") || strings.Contains(c, "scale_")) {
		return true
	}
	if strings.Contains(content, "请确认是否执行") && (strings.Contains(c, "delete_") || strings.Contains(c, "create_")) {
		return true
	}
	return false
}

func stripFakeAgentActionBlocks(content string) string {
	re := regexp.MustCompile("(?s)```action\\s*\\n.*?\\n```")
	out := re.ReplaceAllString(content, "")
	out = strings.TrimSpace(out)
	if out == "" {
		return "已暂存变更，请在界面确认后执行。"
	}
	return out
}

func sanitizeAgentHistory(content string) string {
	re := regexp.MustCompile("(?s)```action\\s*\\n.*?\\n```")
	out := re.ReplaceAllString(content, "[旧式 action 块已忽略，请改用工具]")
	// Drop UI-only confirm chatter that confuses the tool loop.
	for _, noise := range []string{"请求 dry-run 预览", "确认执行"} {
		if strings.TrimSpace(out) == noise {
			return "[用户确认相关消息已省略]"
		}
	}
	return out
}

const agentResourceListLimit = 40

var (
	nsKeywordPattern = regexp.MustCompile(`(?i)(?:命名空间|namespace|ns)[:：\s/-]*([a-z0-9]([-a-z0-9]*[a-z0-9])?)`)
	nsUnderPattern   = regexp.MustCompile(`(?i)([a-z0-9]([-a-z0-9]*[a-z0-9])?)\s*(?:命名空间)?\s*(?:下|里|中的|内的)`)
	podNamePattern   = regexp.MustCompile(`(?i)\b([a-z0-9]([-a-z0-9]*[a-z0-9])?-[a-z0-9]{4,10}(-[a-z0-9]{5})?)\b`)
)

func truncateAgentText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...(已截断)"
}

func isDiagnoseIntent(message string) bool {
	lower := strings.ToLower(message)
	keywords := []string{
		"为什么", "为啥", "原因", "报错", "错误", "失败", "异常", "崩溃",
		"重启", "restart", "crash", "oom", "pending", "imagepull",
		"日志", "log", "event", "事件", "describe", "排查", "诊断", "分析",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) || strings.Contains(message, kw) {
			return true
		}
	}
	return false
}

func extractNamespaceFromMessage(message string, known []string) string {
	lower := strings.ToLower(message)

	// 显式写法优先：命名空间 xxx / namespace:xxx / ns/xxx
	if m := nsKeywordPattern.FindStringSubmatch(message); len(m) > 1 {
		return m[1]
	}
	if m := nsUnderPattern.FindStringSubmatch(message); len(m) > 1 {
		cand := m[1]
		for _, ns := range known {
			if strings.EqualFold(ns, cand) {
				return ns
			}
		}
		return cand
	}

	// 仅当消息中出现独立命名空间词时才匹配（避免把 pod 名前缀误认为 ns）
	best := ""
	for _, ns := range known {
		if ns == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)(?:^|[^a-z0-9-])` + regexp.QuoteMeta(ns) + `(?:[^a-z0-9-]|$)`)
		if re.MatchString(lower) && len(ns) > len(best) {
			best = ns
		}
	}
	return best
}

func extractPodNameFromMessage(message string, knownPods []string) string {
	lower := strings.ToLower(message)
	best := ""
	for _, name := range knownPods {
		if name == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(name)) && len(name) > len(best) {
			best = name
		}
	}
	if best != "" {
		return best
	}
	if m := podNamePattern.FindStringSubmatch(message); len(m) > 1 {
		return m[1]
	}
	return ""
}

func (s *Service) findPodByName(ctx context.Context, clusterID uint, namespace, podName string) (*corev1.Pod, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, err
	}
	if namespace != "" {
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err == nil {
			return pod, nil
		}
	}
	pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var exact *corev1.Pod
	var partials []corev1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Name == podName {
			exact = p
			break
		}
		if strings.Contains(p.Name, podName) || strings.Contains(podName, p.Name) {
			partials = append(partials, *p)
		}
	}
	if exact != nil {
		return exact, nil
	}
	if len(partials) == 1 {
		return &partials[0], nil
	}
	if namespace != "" {
		return nil, fmt.Errorf("pods %q not found in namespace %q", podName, namespace)
	}
	return nil, fmt.Errorf("pods %q not found", podName)
}

func redactSecretValue(raw string) string {
	n := len(raw)
	if n == 0 {
		return "<empty>"
	}
	if n <= 8 {
		return fmt.Sprintf("<redacted len=%d>", n)
	}
	return fmt.Sprintf("%s...%s <redacted len=%d>", raw[:4], raw[n-4:], n)
}

func isSensitiveConfigKey(key string) bool {
	k := strings.ToLower(key)
	needles := []string{
		"token", "password", "passwd", "secret", "apikey", "api_key", "access_key",
		"private", "credential", "auth", "bearer", "cert", "key.pem",
	}
	for _, n := range needles {
		if strings.Contains(k, n) {
			return true
		}
	}
	return false
}

func redactConfigMapValue(key, value string) string {
	lowerVal := strings.ToLower(value)
	sensitiveInline := strings.Contains(lowerVal, "token") ||
		strings.Contains(lowerVal, "password") ||
		strings.Contains(lowerVal, "glrt-") ||
		strings.Contains(lowerVal, "glpat-") ||
		strings.Contains(lowerVal, "begin private key")
	if isSensitiveConfigKey(key) || sensitiveInline {
		// 对多行配置（如 config.toml）做逐行脱敏，保留结构便于分析
		if strings.Contains(value, "\n") {
			lines := strings.Split(value, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				lowerLine := strings.ToLower(trimmed)
				if strings.Contains(lowerLine, "token") ||
					strings.Contains(lowerLine, "password") ||
					strings.Contains(lowerLine, "secret") ||
					strings.Contains(lowerLine, "glrt-") ||
					strings.Contains(lowerLine, "glpat-") {
					if idx := strings.Index(line, "="); idx >= 0 {
						left := line[:idx+1]
						right := strings.TrimSpace(line[idx+1:])
						right = strings.Trim(right, `"'`)
						lines[i] = left + " " + redactSecretValue(right)
					} else {
						lines[i] = redactSecretValue(line)
					}
				}
			}
			return strings.Join(lines, "\n")
		}
		return redactSecretValue(value)
	}
	return value
}

func collectPodConfigRefs(pod *corev1.Pod) (configMaps, secrets map[string]struct{}) {
	configMaps = map[string]struct{}{}
	secrets = map[string]struct{}{}

	addCM := func(name string) {
		if name != "" {
			configMaps[name] = struct{}{}
		}
	}
	addSec := func(name string) {
		if name != "" {
			secrets[name] = struct{}{}
		}
	}

	for _, v := range pod.Spec.Volumes {
		if v.ConfigMap != nil {
			addCM(v.ConfigMap.Name)
		}
		if v.Secret != nil {
			addSec(v.Secret.SecretName)
		}
		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.ConfigMap != nil {
					addCM(src.ConfigMap.Name)
				}
				if src.Secret != nil {
					addSec(src.Secret.Name)
				}
			}
		}
	}

	containers := append([]corev1.Container{}, pod.Spec.InitContainers...)
	containers = append(containers, pod.Spec.Containers...)
	for _, c := range containers {
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if e.ValueFrom.ConfigMapKeyRef != nil {
				addCM(e.ValueFrom.ConfigMapKeyRef.Name)
			}
			if e.ValueFrom.SecretKeyRef != nil {
				addSec(e.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				addCM(ef.ConfigMapRef.Name)
			}
			if ef.SecretRef != nil {
				addSec(ef.SecretRef.Name)
			}
		}
	}
	return configMaps, secrets
}

func (s *Service) appendPodRelatedConfigs(ctx context.Context, clusterID uint, pod *corev1.Pod, sb *strings.Builder) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return
	}

	cmNames, secNames := collectPodConfigRefs(pod)

	// Owner / 控制器信息，便于关联 Deployment 配置
	sb.WriteString("### OwnerReferences\n")
	if len(pod.OwnerReferences) == 0 {
		sb.WriteString("无\n\n")
	} else {
		for _, o := range pod.OwnerReferences {
			sb.WriteString(fmt.Sprintf("- %s/%s (controller=%v)\n", o.Kind, o.Name, o.Controller != nil && *o.Controller))
		}
		sb.WriteString("\n")
		for _, o := range pod.OwnerReferences {
			if o.Kind != "ReplicaSet" {
				continue
			}
			rs, err := client.Clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, o.Name, metav1.GetOptions{})
			if err != nil {
				sb.WriteString(fmt.Sprintf("获取 ReplicaSet %s 失败: %v\n\n", o.Name, err))
				continue
			}
			for _, ro := range rs.OwnerReferences {
				if ro.Kind != "Deployment" {
					continue
				}
				dep, err := client.Clientset.AppsV1().Deployments(pod.Namespace).Get(ctx, ro.Name, metav1.GetOptions{})
				if err != nil {
					sb.WriteString(fmt.Sprintf("获取 Deployment %s 失败: %v\n\n", ro.Name, err))
					continue
				}
				sb.WriteString(fmt.Sprintf("### 关联 Deployment: %s\n", dep.Name))
				sb.WriteString(formatDeploymentDescribe(dep))
				sb.WriteString("\n")
				// Deployment 模板也可能引用额外 CM/Secret
				tmpPod := &corev1.Pod{Spec: dep.Spec.Template.Spec}
				tmpPod.Namespace = pod.Namespace
				extraCM, extraSec := collectPodConfigRefs(tmpPod)
				for name := range extraCM {
					cmNames[name] = struct{}{}
				}
				for name := range extraSec {
					secNames[name] = struct{}{}
				}
			}
		}
	}

	sb.WriteString("### 容器环境变量(明文 Value)\n")
	for _, c := range pod.Spec.Containers {
		sb.WriteString(fmt.Sprintf("- container=%s\n", c.Name))
		if len(c.Env) == 0 && len(c.EnvFrom) == 0 {
			sb.WriteString("  (无)\n")
			continue
		}
		for _, e := range c.Env {
			if e.ValueFrom != nil {
				src := "valueFrom"
				if e.ValueFrom.ConfigMapKeyRef != nil {
					src = fmt.Sprintf("configMap:%s/%s", e.ValueFrom.ConfigMapKeyRef.Name, e.ValueFrom.ConfigMapKeyRef.Key)
				} else if e.ValueFrom.SecretKeyRef != nil {
					src = fmt.Sprintf("secret:%s/%s", e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key)
				} else if e.ValueFrom.FieldRef != nil {
					src = fmt.Sprintf("fieldRef:%s", e.ValueFrom.FieldRef.FieldPath)
				}
				sb.WriteString(fmt.Sprintf("  %s <= %s\n", e.Name, src))
				continue
			}
			sb.WriteString(fmt.Sprintf("  %s=%s\n", e.Name, truncateAgentText(e.Value, 200)))
		}
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				sb.WriteString(fmt.Sprintf("  envFrom configMap=%s\n", ef.ConfigMapRef.Name))
			}
			if ef.SecretRef != nil {
				sb.WriteString(fmt.Sprintf("  envFrom secret=%s\n", ef.SecretRef.Name))
			}
		}
	}
	sb.WriteString("\n")

	sb.WriteString("### 关联 ConfigMap\n")
	if len(cmNames) == 0 {
		sb.WriteString("无\n\n")
	} else {
		for name := range cmNames {
			cm, err := client.Clientset.CoreV1().ConfigMaps(pod.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				sb.WriteString(fmt.Sprintf("- %s: 获取失败: %v\n", name, err))
				continue
			}
			sb.WriteString(fmt.Sprintf("- ConfigMap/%s (keys=%d)\n", cm.Name, len(cm.Data)+len(cm.BinaryData)))
			for k, v := range cm.Data {
				safe := redactConfigMapValue(k, v)
				sb.WriteString(fmt.Sprintf("  [%s]\n```\n%s\n```\n", k, truncateAgentText(safe, 2500)))
			}
			for k, v := range cm.BinaryData {
				sb.WriteString(fmt.Sprintf("  [%s] <binary len=%d>\n", k, len(v)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### 关联 Secret（已脱敏）\n")
	if len(secNames) == 0 {
		sb.WriteString("无\n\n")
	} else {
		for name := range secNames {
			sec, err := client.Clientset.CoreV1().Secrets(pod.Namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				sb.WriteString(fmt.Sprintf("- %s: 获取失败: %v\n", name, err))
				continue
			}
			sb.WriteString(fmt.Sprintf("- Secret/%s type=%s keys=%d\n", sec.Name, sec.Type, len(sec.Data)))
			for k, v := range sec.Data {
				sb.WriteString(fmt.Sprintf("  %s=%s\n", k, redactSecretValue(string(v))))
			}
		}
		sb.WriteString("\n")
	}
}

func (s *Service) collectPodDiagnostics(ctx context.Context, clusterID uint, namespace, podName string) string {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return ""
	}

	var sb strings.Builder
	pod, err := s.findPodByName(ctx, clusterID, namespace, podName)
	if err != nil || pod == nil {
		return fmt.Sprintf("无法获取 Pod %s（namespace=%s）: %v\n", podName, namespace, err)
	}

	sb.WriteString(fmt.Sprintf("## Pod 诊断数据: %s/%s\n\n", pod.Namespace, pod.Name))
	sb.WriteString("### Describe\n")
	sb.WriteString(formatPodDescribe(pod))
	sb.WriteString("\n")

	events, evErr := client.Clientset.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", pod.Name),
	})
	sb.WriteString("### Events\n")
	if evErr != nil {
		sb.WriteString(fmt.Sprintf("获取事件失败: %v\n", evErr))
	} else if len(events.Items) == 0 {
		sb.WriteString("无相关事件\n")
	} else {
		items := events.Items
		if len(items) > 15 {
			items = items[len(items)-15:]
		}
		for i := len(items) - 1; i >= 0; i-- {
			e := items[i]
			sb.WriteString(fmt.Sprintf("- [%s] %s count=%d: %s\n", e.Type, e.Reason, e.Count, e.Message))
		}
	}
	sb.WriteString("\n")

	appendLogs := func(title string, previous bool) {
		opts := &corev1.PodLogOptions{Previous: previous}
		tail := int64(80)
		opts.TailLines = &tail
		req := client.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			sb.WriteString(fmt.Sprintf("### %s\n获取失败: %v\n\n", title, err))
			return
		}
		defer stream.Close()
		data, err := io.ReadAll(stream)
		if err != nil {
			sb.WriteString(fmt.Sprintf("### %s\n读取失败: %v\n\n", title, err))
			return
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			content = "(空)"
		}
		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", title, truncateAgentText(content, 4000)))
	}
	appendLogs("当前日志", false)
	appendLogs("上一次崩溃日志(--previous)", true)

	s.appendPodRelatedConfigs(ctx, clusterID, pod, &sb)

	return truncateAgentText(sb.String(), 20000)
}

// queryRealData 根据用户查询获取真实数据（按意图/命名空间拉取，作为 LLM 上下文）
func (s *Service) queryRealData(ctx context.Context, clusterID uint, message string) string {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return ""
	}

	lower := strings.ToLower(message)
	result := ""

	knownNS := []string{}
	if nsList, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		for _, ns := range nsList.Items {
			knownNS = append(knownNS, ns.Name)
		}
	}
	targetNS := extractNamespaceFromMessage(message, knownNS)

	// 诊断意图：自动拉取目标 Pod 的 describe / events / logs
	diagnose := isDiagnoseIntent(message)
	knownPods := []string{}
	podNSByName := map[string]string{}
	if diagnose || strings.Contains(lower, "pod") {
		pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, p := range pods.Items {
				knownPods = append(knownPods, p.Name)
				podNSByName[p.Name] = p.Namespace
			}
		}
	}
	podName := extractPodNameFromMessage(message, knownPods)
	if diagnose && podName != "" {
		ns := targetNS
		if ns == "" {
			ns = podNSByName[podName]
		}
		result += s.collectPodDiagnostics(ctx, clusterID, ns, podName)
		// 已拿到诊断上下文时，不必再塞全量列表，避免冲淡关键证据
		return truncateAgentText(result, 22000)
	}

	wantSvc := strings.Contains(lower, "svc") || strings.Contains(lower, "service") || strings.Contains(message, "服务")
	wantDeploy := strings.Contains(lower, "deploy") || strings.Contains(lower, "deployment") ||
		strings.Contains(message, "部署") || strings.Contains(lower, "nginx")
	wantPod := strings.Contains(lower, "pod") || strings.Contains(message, "容器") || strings.Contains(message, "副本")
	isDelete := strings.Contains(message, "删除") || strings.Contains(lower, "delete")
	isList := strings.Contains(message, "查看") || strings.Contains(message, "查询") ||
		strings.Contains(message, "列出") || strings.Contains(lower, "list") ||
		strings.Contains(message, "所有") || strings.Contains(message, "全部")

	// 未点名资源类型时：删除操作才同时拉三类；普通查看只拉最相关的一类，默认 Pod
	if !wantSvc && !wantDeploy && !wantPod {
		if isDelete {
			wantSvc, wantDeploy, wantPod = true, true, true
		} else if isList {
			wantPod = true
		}
	}

	listNS := ""
	if targetNS != "" {
		listNS = targetNS
		result += fmt.Sprintf("查询范围命名空间: %s\n\n", targetNS)
	}

	if wantSvc {
		services, err := client.Clientset.CoreV1().Services(listNS).List(ctx, metav1.ListOptions{})
		if err == nil {
			items := services.Items
			total := len(items)
			if total > agentResourceListLimit {
				items = items[:agentResourceListLimit]
			}
			result += fmt.Sprintf("Service 列表（共 %d 条，展示前 %d）:\n", total, len(items))
			result += "命名空间 | 名称 | 类型 | ClusterIP | 端口\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, svc := range items {
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

	if wantDeploy {
		deployments, err := client.Clientset.AppsV1().Deployments(listNS).List(ctx, metav1.ListOptions{})
		if err == nil {
			items := deployments.Items
			total := len(items)
			if total > agentResourceListLimit {
				items = items[:agentResourceListLimit]
			}
			result += fmt.Sprintf("Deployment 列表（共 %d 条，展示前 %d）:\n", total, len(items))
			result += "命名空间 | 名称 | 副本 | 就绪 | 镜像\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, d := range items {
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

	if wantPod {
		pods, err := client.Clientset.CoreV1().Pods(listNS).List(ctx, metav1.ListOptions{})
		if err == nil {
			items := pods.Items
			total := len(items)
			if total > agentResourceListLimit {
				items = items[:agentResourceListLimit]
			}
			result += fmt.Sprintf("Pod 列表（共 %d 条，展示前 %d）:\n", total, len(items))
			result += "命名空间 | 名称 | 状态 | 重启次数 | 节点\n"
			result += "--- | --- | --- | --- | ---\n"
			for _, pod := range items {
				restarts := int32(0)
				for _, cs := range pod.Status.ContainerStatuses {
					restarts += cs.RestartCount
				}
				result += fmt.Sprintf("%s | %s | %s | %d | %s\n",
					pod.Namespace, pod.Name, pod.Status.Phase, restarts, pod.Spec.NodeName)
			}
		}
	}

	return truncateAgentText(result, 12000)
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

// executeCreateAction 执行创建动作（未实现，拒绝假成功）
func (s *Service) executeCreateAction(ctx context.Context, client *k8s.ClusterClient, action *model.AgentAction) (string, error) {
	return "", fmt.Errorf("create action is not implemented; use staged confirm flow for mutating operations")
}

// executeDeleteAction 执行删除动作（未实现，拒绝假成功）
func (s *Service) executeDeleteAction(ctx context.Context, client *k8s.ClusterClient, action *model.AgentAction) (string, error) {
	return "", fmt.Errorf("delete action is not implemented; use staged confirm flow for mutating operations")
}
