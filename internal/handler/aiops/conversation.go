package aiops

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

// ListConversations 获取用户对话列表
func (h *Handler) ListConversations(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var conversations []model.ChatConversation
	if err := h.db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&conversations).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type ConversationInfo struct {
		ID           uint   `json:"id"`
		Title        string `json:"title"`
		ClusterID    *uint  `json:"cluster_id"`
		MessageCount int    `json:"message_count"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}

	result := make([]ConversationInfo, 0, len(conversations))
	for _, conv := range conversations {
		var count int64
		h.db.Model(&model.ChatMessage{}).Where("conversation_id = ?", conv.ID).Count(&count)

		result = append(result, ConversationInfo{
			ID:           conv.ID,
			Title:        conv.Title,
			ClusterID:    conv.ClusterID,
			MessageCount: int(count),
			CreatedAt:    conv.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    conv.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	response.Success(c, result)
}

// CreateConversation 创建对话
func (h *Handler) CreateConversation(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		Title     string `json:"title"`
		ClusterID *uint  `json:"cluster_id"`
		ChatType  string `json:"chat_type"` // chat, agent
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空body
		req.Title = "新对话"
	}

	if req.Title == "" {
		req.Title = "新对话"
	}
	if req.ChatType == "" {
		req.ChatType = "chat"
	}

	conversation := &model.ChatConversation{
		UserID:    userID.(uint),
		Title:     req.Title,
		ClusterID: req.ClusterID,
	}

	if err := h.db.Create(conversation).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, conversation)
}

// GetConversation 获取对话详情
func (h *Handler) GetConversation(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	// 获取消息
	var messages []model.ChatMessage
	h.db.Where("conversation_id = ?", conversation.ID).Order("created_at ASC").Find(&messages)

	type MessageInfo struct {
		ID        uint   `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}

	msgList := make([]MessageInfo, 0, len(messages))
	for _, msg := range messages {
		msgList = append(msgList, MessageInfo{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	response.Success(c, gin.H{
		"id":         conversation.ID,
		"title":      conversation.Title,
		"cluster_id": conversation.ClusterID,
		"messages":   msgList,
		"created_at": conversation.CreatedAt,
		"updated_at": conversation.UpdatedAt,
	})
}

// UpdateConversation 更新对话
func (h *Handler) UpdateConversation(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	var req struct {
		Title     string `json:"title"`
		ClusterID *uint  `json:"cluster_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.ClusterID != nil {
		updates["cluster_id"] = req.ClusterID
	}

	if len(updates) > 0 {
		h.db.Model(&conversation).Updates(updates)
	}

	response.SuccessWithMessage(c, "conversation updated", nil)
}

// DeleteConversation 删除对话
func (h *Handler) DeleteConversation(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	// 删除关联的消息
	h.db.Where("conversation_id = ?", conversation.ID).Delete(&model.ChatMessage{})

	// 删除对话
	h.db.Delete(&conversation)

	response.SuccessWithMessage(c, "conversation deleted", nil)
}

// ClearConversation 清空对话消息
func (h *Handler) ClearConversation(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	h.db.Where("conversation_id = ?", conversation.ID).Delete(&model.ChatMessage{})

	response.SuccessWithMessage(c, "conversation cleared", nil)
}

// ListMessages 获取消息列表
func (h *Handler) ListMessages(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	// 验证对话属于用户
	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 100
	}

	var messages []model.ChatMessage
	var total int64

	h.db.Model(&model.ChatMessage{}).Where("conversation_id = ?", conversation.ID).Count(&total)
	h.db.Where("conversation_id = ?", conversation.ID).
		Order("created_at ASC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&messages)

	type MessageInfo struct {
		ID        uint   `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]MessageInfo, 0, len(messages))
	for _, msg := range messages {
		result = append(result, MessageInfo{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	response.PageSuccess(c, result, total, page, size)
}

// AddMessage 添加消息
func (h *Handler) AddMessage(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")

	// 验证对话属于用户
	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	var req struct {
		Role    string `json:"role" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.Role != "user" && req.Role != "assistant" && req.Role != "system" {
		response.BadRequest(c, "invalid role")
		return
	}

	message := &model.ChatMessage{
		ConversationID: conversation.ID,
		Role:           req.Role,
		Content:        req.Content,
	}

	if err := h.db.Create(message).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 更新对话的更新时间
	h.db.Model(&conversation).Update("updated_at", message.CreatedAt)

	// 如果是用户的第一条消息，更新对话标题
	if req.Role == "user" {
		var count int64
		h.db.Model(&model.ChatMessage{}).Where("conversation_id = ? AND role = ?", conversation.ID, "user").Count(&count)
		if count == 1 {
			title := req.Content
			if len(title) > 50 {
				title = title[:50] + "..."
			}
			h.db.Model(&conversation).Update("title", title)
		}
	}

	response.Created(c, gin.H{
		"id":         message.ID,
		"role":       message.Role,
		"content":    message.Content,
		"created_at": message.CreatedAt,
	})
}

// DeleteMessage 删除消息
func (h *Handler) DeleteMessage(c *gin.Context) {
	userID, _ := c.Get("user_id")
	convID := c.Param("id")
	msgID := c.Param("msgId")

	// 验证对话属于用户
	var conversation model.ChatConversation
	if err := h.db.Where("id = ? AND user_id = ?", convID, userID).First(&conversation).Error; err != nil {
		response.NotFound(c, "conversation not found")
		return
	}

	var message model.ChatMessage
	if err := h.db.Where("id = ? AND conversation_id = ?", msgID, conversation.ID).First(&message).Error; err != nil {
		response.NotFound(c, "message not found")
		return
	}

	h.db.Delete(&message)

	response.SuccessWithMessage(c, "message deleted", nil)
}
