package model

import (
	"time"
)

// EventForwardRule Event 转发规则
type EventForwardRule struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ClusterID   uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster     Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	Description string    `json:"description" gorm:"size:512"`
	WebhookURL  string    `json:"webhook_url" gorm:"size:512;not null"`
	// 过滤条件
	Namespaces  string    `json:"namespaces" gorm:"type:text"`  // JSON array, 空表示所有
	Resources   string    `json:"resources" gorm:"type:text"`   // JSON array: pod, deployment, node, etc.
	EventTypes  string    `json:"event_types" gorm:"type:text"` // JSON array: Normal, Warning
	Reasons     string    `json:"reasons" gorm:"type:text"`     // JSON array: 过滤特定 reason
	// Webhook 配置
	Headers     string    `json:"headers" gorm:"type:text"`     // JSON: 自定义 headers
	Template    string    `json:"template" gorm:"type:text"`    // 自定义消息模板
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (EventForwardRule) TableName() string {
	return "event_forward_rules"
}

// EventForwardLog Event 转发日志
type EventForwardLog struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	RuleID      uint      `json:"rule_id" gorm:"index;not null"`
	Rule        EventForwardRule `json:"rule" gorm:"foreignKey:RuleID"`
	ClusterID   uint      `json:"cluster_id" gorm:"index"`
	Namespace   string    `json:"namespace" gorm:"size:64"`
	Resource    string    `json:"resource" gorm:"size:128"`
	EventType   string    `json:"event_type" gorm:"size:20"`
	Reason      string    `json:"reason" gorm:"size:128"`
	Message     string    `json:"message" gorm:"type:text"`
	Status      string    `json:"status" gorm:"size:20;not null"` // success, failed
	StatusCode  int       `json:"status_code"`
	Error       string    `json:"error" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
}

func (EventForwardLog) TableName() string {
	return "event_forward_logs"
}
