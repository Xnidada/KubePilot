package model

import (
	"time"
)

// WebhookConfig Webhook 配置
type WebhookConfig struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"size:128;not null"`
	Type        string `json:"type" gorm:"size:32;not null"` // slack, teams, dingtalk, custom
	URL         string `json:"url" gorm:"size:512;not null"`

	// 认证
	Secret      string `json:"secret" gorm:"size:256"`

	// 过滤条件
	Events      string `json:"events" gorm:"type:text"`    // JSON array: alert, event, backup, etc.
	Namespaces  string `json:"namespaces" gorm:"type:text"` // JSON array, 空表示所有
	Severity    string `json:"severity" gorm:"size:32"`     // info, warning, error, critical

	// 模板
	Template    string `json:"template" gorm:"type:text"` // 自定义消息模板

	// 状态
	Enabled     bool       `json:"enabled" gorm:"default:true"`
	LastFiredAt *time.Time `json:"last_fired_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (WebhookConfig) TableName() string {
	return "webhook_configs"
}

// WebhookLog Webhook 调用日志
type WebhookLog struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	WebhookID  uint   `json:"webhook_id" gorm:"index;not null"`
	Webhook    WebhookConfig `json:"webhook" gorm:"foreignKey:WebhookID"`

	// 事件信息
	EventType  string `json:"event_type" gorm:"size:64"`
	EventData  string `json:"event_data" gorm:"type:text"`

	// 请求信息
	RequestURL string `json:"request_url" gorm:"size:512"`
	RequestBody string `json:"request_body" gorm:"type:text"`

	// 响应信息
	StatusCode int    `json:"status_code"`
	Response   string `json:"response" gorm:"type:text"`
	Error      string `json:"error" gorm:"type:text"`

	// 状态
	Status    string    `json:"status" gorm:"size:20"` // success, failed
	CreatedAt time.Time `json:"created_at"`
}

func (WebhookLog) TableName() string {
	return "webhook_logs"
}
