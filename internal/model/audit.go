package model

import (
	"time"
)

type AuditLog struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	UserID       uint      `json:"user_id" gorm:"index"`
	User         User      `json:"user" gorm:"foreignKey:UserID"`
	Username     string    `json:"username" gorm:"size:64"`
	Action       string    `json:"action" gorm:"size:32;not null;index"` // create, update, delete, get, list, exec
	ResourceType string    `json:"resource_type" gorm:"size:64;not null;index"`
	ResourceName string    `json:"resource_name" gorm:"size:128"`
	ClusterID    uint      `json:"cluster_id" gorm:"index"`
	Cluster      Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace    string    `json:"namespace" gorm:"size:64;index"`
	RequestBody  string    `json:"request_body" gorm:"type:text"`
	ResponseCode int       `json:"response_code"`
	Latency      int64 `json:"latency"` // milliseconds
	IP           string    `json:"ip" gorm:"size:45"`
	UserAgent    string    `json:"user_agent" gorm:"size:256"`
	Success      bool      `json:"success"`
	Error        string    `json:"error" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at" gorm:"index"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
