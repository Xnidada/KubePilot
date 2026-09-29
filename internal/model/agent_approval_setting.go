package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// AgentApprovalSetting is a single platform-wide policy row. No row means disabled.
type AgentApprovalSetting struct {
	ID        uint `gorm:"primaryKey"`
	Enabled   bool `gorm:"not null;default:false"`
	UpdatedAt time.Time
}

func (AgentApprovalSetting) TableName() string { return "agent_approval_settings" }

// ProductionWriteBlocked is shared by HTTP writes and background workers.
// The absence of a policy row means the optional gate is off.
func ProductionWriteBlocked(db *gorm.DB, clusterID uint) (bool, error) {
	var setting AgentApprovalSetting
	if err := db.First(&setting, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !setting.Enabled {
		return false, nil
	}
	var cluster Cluster
	if err := db.Select("environment").First(&cluster, clusterID).Error; err != nil {
		return false, err
	}
	return cluster.Environment == "" || cluster.Environment == "production", nil
}
