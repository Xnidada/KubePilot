package model

import (
	"time"
)

// InspectionRule 集群巡检规则
type InspectionRule struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ClusterID   uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster     Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	Description string    `json:"description" gorm:"size:512"`
	Resource    string    `json:"resource" gorm:"size:64;not null"` // node, pod, deployment, service, custom
	CheckType   string    `json:"check_type" gorm:"size:64"`        // status, resource, custom
	Condition   string    `json:"condition" gorm:"size:32"`          // ==, !=, >, <, >=, <=
	Threshold   string    `json:"threshold" gorm:"size:128"`         // 阈值
	Script      string    `json:"script" gorm:"type:text"`           // Lua 脚本或自定义脚本
	Schedule    string    `json:"schedule" gorm:"size:32"`           // cron 表达式, 空表示手动执行
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (InspectionRule) TableName() string {
	return "inspection_rules"
}

// InspectionReport 巡检报告
type InspectionReport struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	RuleID      uint       `json:"rule_id" gorm:"index;not null"`
	Rule        InspectionRule `json:"rule" gorm:"foreignKey:RuleID"`
	ClusterID   uint       `json:"cluster_id" gorm:"index;not null"`
	Cluster     Cluster    `json:"cluster" gorm:"foreignKey:ClusterID"`
	Status      string     `json:"status" gorm:"size:20;not null"` // running, completed, failed
	TotalChecks int        `json:"total_checks"`
	Passed      int        `json:"passed"`
	Failed      int        `json:"failed"`
	Warnings    int        `json:"warnings"`
	Error       string     `json:"error" gorm:"type:text"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (InspectionReport) TableName() string {
	return "inspection_reports"
}

// InspectionResult 巡检结果
type InspectionResult struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	ReportID     uint   `json:"report_id" gorm:"index;not null"`
	Report       InspectionReport `json:"report" gorm:"foreignKey:ReportID"`
	ResourceType string `json:"resource_type" gorm:"size:64;not null"`
	ResourceName string `json:"resource_name" gorm:"size:128;not null"`
	Namespace    string `json:"namespace" gorm:"size:64"`
	Status       string `json:"status" gorm:"size:20;not null"` // pass, fail, warn
	Message      string `json:"message" gorm:"type:text"`
	Details      string `json:"details" gorm:"type:text"` // JSON 格式的详细信息
	CreatedAt    time.Time `json:"created_at"`
}

func (InspectionResult) TableName() string {
	return "inspection_results"
}
