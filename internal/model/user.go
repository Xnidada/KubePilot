package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Username  string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	Email     string         `json:"email" gorm:"uniqueIndex;size:128;not null"`
	Password  string         `json:"-" gorm:"size:256;not null"`
	RealName  string         `json:"real_name" gorm:"size:64"`
	Phone     string         `json:"phone" gorm:"size:20"`
	Avatar    string         `json:"avatar" gorm:"size:256"`
	Status    int            `json:"status" gorm:"default:1"` // 1:active, 0:disabled
	RoleID    uint           `json:"role_id" gorm:"index"`
	Role      Role           `json:"role" gorm:"foreignKey:RoleID"`
	LastLogin *time.Time     `json:"last_login"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (User) TableName() string {
	return "users"
}

type Role struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:64;not null"`
	Description string         `json:"description" gorm:"size:256"`
	Permissions string         `json:"permissions" gorm:"type:text"` // JSON string
	IsSystem    bool           `json:"is_system" gorm:"default:false"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Role) TableName() string {
	return "roles"
}

type UserCluster struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	UserID          uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_user_cluster_namespace"`
	User            User      `json:"user" gorm:"foreignKey:UserID"`
	ClusterID       uint      `json:"cluster_id" gorm:"not null;uniqueIndex:idx_user_cluster_namespace;index"`
	Cluster         Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace       string    `json:"namespace" gorm:"size:64;not null;default:'*';uniqueIndex:idx_user_cluster_namespace"`
	PermissionLevel string    `json:"permission_level" gorm:"size:20;not null;default:'read'"` // read, write, admin
	CreatedAt       time.Time `json:"created_at"`
}

func (UserCluster) TableName() string {
	return "user_clusters"
}

// ChatConversation 对话会话
type ChatConversation struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id" gorm:"index;not null"`
	User        User      `json:"user" gorm:"foreignKey:UserID"`
	Title       string    `json:"title" gorm:"size:256;not null"`
	ClusterID   *uint     `json:"cluster_id"`
	Cluster     *Cluster  `json:"cluster" gorm:"foreignKey:ClusterID"`
	LLMConfigID *uint     `json:"llm_config_id"`
	IsArchived  bool      `json:"is_archived" gorm:"default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ChatConversation) TableName() string {
	return "chat_conversations"
}

// ChatMessage 对话消息
type ChatMessage struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	ConversationID uint             `json:"conversation_id" gorm:"index;not null"`
	Conversation   ChatConversation `json:"conversation" gorm:"foreignKey:ConversationID"`
	Role           string           `json:"role" gorm:"size:20;not null"` // user, assistant, system
	Content        string           `json:"content" gorm:"type:text;not null"`
	Extras         string           `json:"extras" gorm:"type:text"` // JSON: tool_trace, pending_action_ids
	TokenUsage     int              `json:"token_usage"`
	CreatedAt      time.Time        `json:"created_at"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

// AgentAction Agent执行的动作
type AgentAction struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	UserID         uint       `json:"user_id" gorm:"index"`
	ConversationID *uint      `json:"conversation_id" gorm:"index"` // optional; staged actions may have no chat
	ActionType     string     `json:"action_type" gorm:"size:20;not null"` // query, create, update, delete, scale
	ResourceType   string     `json:"resource_type" gorm:"size:64;not null"`
	ResourceName   string     `json:"resource_name" gorm:"size:128"`
	Namespace      string     `json:"namespace" gorm:"size:64"`
	ClusterID      uint       `json:"cluster_id"`
	Description    string     `json:"description" gorm:"type:text"`
	Parameters     string     `json:"parameters" gorm:"type:text"` // JSON
	DryRunResult   string     `json:"dry_run_result" gorm:"type:text"`
	Status         string     `json:"status" gorm:"size:20;default:'pending'"` // pending, confirmed, executed, failed, cancelled
	Result         string     `json:"result" gorm:"type:text"`
	CreatedAt      time.Time  `json:"created_at"`
	ExecutedAt     *time.Time `json:"executed_at"`
}

func (AgentAction) TableName() string {
	return "agent_actions"
}

// AgentToolTrace stores one agent turn's tool call observability payload.
type AgentToolTrace struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"user_id" gorm:"index"`
	ClusterID      uint      `json:"cluster_id" gorm:"index"`
	ConversationID uint      `json:"conversation_id" gorm:"index"`
	Payload        string    `json:"payload" gorm:"type:text"`
	ToolCount      int       `json:"tool_count"`
	CreatedAt      time.Time `json:"created_at"`
}

func (AgentToolTrace) TableName() string {
	return "agent_tool_traces"
}

// TokenUsageLog records token consumption per AI interaction.
type TokenUsageLog struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"user_id" gorm:"index"`
	ConversationID   uint      `json:"conversation_id" gorm:"index"`
	LLMConfigID      uint      `json:"llm_config_id" gorm:"index"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	ChatType         string    `json:"chat_type" gorm:"size:20"` // "agent" | "chat" | "explain" | "diagnose"
	CreatedAt        time.Time `json:"created_at"`
}

func (TokenUsageLog) TableName() string {
	return "token_usage_logs"
}

// LLMConfig LLM配置
type LLMConfig struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Provider    string    `json:"provider" gorm:"size:20;not null;default:'openai'"` // openai, anthropic
	APIKey      string    `json:"api_key" gorm:"type:text"`
	BaseURL     string    `json:"base_url" gorm:"size:256"`
	Model       string    `json:"model" gorm:"size:64"`
	Temperature float64   `json:"temperature" gorm:"default:0.7"`
	MaxTokens   int       `json:"max_tokens" gorm:"default:2048"`
	Timeout     int       `json:"timeout" gorm:"default:120"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	InputPricePerM  float64   `json:"input_price_per_m" gorm:"default:2.5"`   // 输入单价 $/1M tokens
	OutputPricePerM float64   `json:"output_price_per_m" gorm:"default:10.0"` // 输出单价 $/1M tokens
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMConfig) TableName() string {
	return "llm_configs"
}
