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
	UserID          uint      `json:"user_id" gorm:"index;not null"`
	User            User      `json:"user" gorm:"foreignKey:UserID"`
	ClusterID       uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster         Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace       string    `json:"namespace" gorm:"size:64;default:'*'"`
	PermissionLevel string    `json:"permission_level" gorm:"size:20;default:'read'"` // read, write, admin
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
	TokenUsage     int              `json:"token_usage"`
	CreatedAt      time.Time        `json:"created_at"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

// AgentAction Agent执行的动作
type AgentAction struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	ConversationID uint             `json:"conversation_id" gorm:"index;not null"`
	Conversation   ChatConversation `json:"conversation" gorm:"foreignKey:ConversationID"`
	ActionType     string           `json:"action_type" gorm:"size:20;not null"` // query, create, update, delete
	ResourceType   string           `json:"resource_type" gorm:"size:64;not null"`
	ResourceName   string           `json:"resource_name" gorm:"size:128"`
	Namespace      string           `json:"namespace" gorm:"size:64"`
	ClusterID      uint             `json:"cluster_id"`
	Description    string           `json:"description" gorm:"type:text"`
	Parameters     string           `json:"parameters" gorm:"type:text"` // JSON
	Status         string           `json:"status" gorm:"size:20;default:'pending'"` // pending, confirmed, executed, failed
	Result         string           `json:"result" gorm:"type:text"`
	CreatedAt      time.Time        `json:"created_at"`
	ExecutedAt     *time.Time       `json:"executed_at"`
}

func (AgentAction) TableName() string {
	return "agent_actions"
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
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMConfig) TableName() string {
	return "llm_configs"
}
