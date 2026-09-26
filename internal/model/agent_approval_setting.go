package model

import "time"

// AgentApprovalSetting is a single platform-wide policy row. No row means disabled.
type AgentApprovalSetting struct {
	ID        uint `gorm:"primaryKey"`
	Enabled   bool `gorm:"not null;default:false"`
	UpdatedAt time.Time
}

func (AgentApprovalSetting) TableName() string { return "agent_approval_settings" }
