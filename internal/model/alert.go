package model

import (
	"time"

	"gorm.io/gorm"
)

type AlertRule struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	Name       string         `json:"name" gorm:"size:128;not null"`
	ClusterID  uint           `json:"cluster_id" gorm:"index;not null"`
	Cluster    Cluster        `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace  string         `json:"namespace" gorm:"size:64"`
	Resource   string         `json:"resource" gorm:"size:64"` // node, pod, deployment
	Metric     string         `json:"metric" gorm:"size:64;not null"` // cpu, memory, disk, restarts
	Condition  string         `json:"condition" gorm:"size:20;not null"` // >, <, >=, <=, ==, !=
	Threshold  float64        `json:"threshold" gorm:"not null"`
	Duration   string         `json:"duration" gorm:"size:20"` // 5m, 10m, 1h
	Channels   string         `json:"channels" gorm:"type:text"` // JSON array of channel IDs
	Enabled    bool           `json:"enabled" gorm:"default:true"`
	LastAlert  *time.Time     `json:"last_alert"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (AlertRule) TableName() string {
	return "alert_rules"
}

type AlertHistory struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	RuleID     uint       `json:"rule_id" gorm:"index;not null"`
	Rule       AlertRule  `json:"rule" gorm:"foreignKey:RuleID"`
	ClusterID  uint       `json:"cluster_id" gorm:"index"`
	Cluster    Cluster    `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace  string     `json:"namespace" gorm:"size:64"`
	Resource   string     `json:"resource" gorm:"size:128"`
	Message    string     `json:"message" gorm:"type:text"`
	Value      float64    `json:"value"`
	Status     string     `json:"status" gorm:"size:20;default:'firing'"` // firing, resolved
	TriggeredAt time.Time `json:"triggered_at" gorm:"index"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Notified   bool       `json:"notified"`
	NotifyAt   *time.Time `json:"notify_at"`
}

func (AlertHistory) TableName() string {
	return "alert_history"
}

type NotificationChannel struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"size:64;not null"`
	Type      string         `json:"type" gorm:"size:20;not null"` // email, webhook, dingtalk, wechat
	Config    string         `json:"config" gorm:"type:text"` // JSON config
	Enabled   bool           `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (NotificationChannel) TableName() string {
	return "notification_channels"
}
