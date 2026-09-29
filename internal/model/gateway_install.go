package model

import "time"

// GatewayInstallJob is one guarded, resumable installation attempt per cluster.
type GatewayInstallJob struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	ClusterID  uint       `json:"cluster_id" gorm:"uniqueIndex;not null"`
	UserID     uint       `json:"user_id"`
	Status     string     `json:"status" gorm:"size:32;not null;index"`
	Message    string     `json:"message" gorm:"type:text"`
	LeaseUntil *time.Time `json:"-" gorm:"index"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (GatewayInstallJob) TableName() string { return "gateway_install_jobs" }

type GatewayInstallLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	JobID     uint      `json:"job_id" gorm:"index;not null"`
	Level     string    `json:"level" gorm:"size:16"`
	Message   string    `json:"message" gorm:"type:text"`
	CreatedAt time.Time `json:"at"`
}

func (GatewayInstallLog) TableName() string { return "gateway_install_logs" }
