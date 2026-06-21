package model

import (
	"time"
)

// UserTwoFactor 用户两步验证配置
type UserTwoFactor struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id" gorm:"uniqueIndex;not null"`
	User        User      `json:"user" gorm:"foreignKey:UserID"`
	Secret      string    `json:"-" gorm:"size:128;not null"` // TOTP secret, 不返回给前端
	IsEnabled   bool      `json:"is_enabled" gorm:"default:false"`
	BackupCodes string    `json:"-" gorm:"type:text"` // JSON array of backup codes
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (UserTwoFactor) TableName() string {
	return "user_two_factors"
}
