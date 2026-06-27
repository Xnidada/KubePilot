package model

import (
	"time"
)

// BackupSchedule 备份计划
type BackupSchedule struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"size:128;not null"`
	ClusterID   uint   `json:"cluster_id" gorm:"index;not null"`
	Cluster     Cluster `json:"cluster" gorm:"foreignKey:ClusterID"`

	// 备份配置
	Namespaces  string `json:"namespaces" gorm:"type:text"`    // JSON array, 空表示全量
	Resources   string `json:"resources" gorm:"type:text"`     // JSON array: deployments, services, etc.
	Schedule    string `json:"schedule" gorm:"size:64"`        // Cron 表达式
	TTL         string `json:"ttl" gorm:"size:32;default:'720h'"` // 保留时间

	// 存储配置
	StorageLocation string `json:"storage_location" gorm:"size:128"`

	// 状态
	Status    string    `json:"status" gorm:"size:20;default:'active'"`
	LastBackup *time.Time `json:"last_backup"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (BackupSchedule) TableName() string {
	return "backup_schedules"
}

// BackupRecord 备份记录
type BackupRecord struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	ScheduleID   *uint  `json:"schedule_id"`
	Schedule     *BackupSchedule `json:"schedule" gorm:"foreignKey:ScheduleID"`
	ClusterID    uint   `json:"cluster_id" gorm:"index;not null"`
	Cluster      Cluster `json:"cluster" gorm:"foreignKey:ClusterID"`
	BackupName   string `json:"backup_name" gorm:"size:128;not null"`

	// 备份信息
	Namespaces   string `json:"namespaces" gorm:"type:text"`
	Resources    string `json:"resources" gorm:"type:text"`

	// 状态
	Status       string     `json:"status" gorm:"size:20;not null"` // pending, in_progress, completed, failed
	Phase        string     `json:"phase" gorm:"size:32"`
	VolumeSnapshots int     `json:"volume_snapshots"`
	Errors       int        `json:"errors"`
	Warnings     int        `json:"warnings"`

	// 时间
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (BackupRecord) TableName() string {
	return "backup_records"
}

// RestoreRecord 恢复记录
type RestoreRecord struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	BackupID    uint   `json:"backup_id" gorm:"index;not null"`
	Backup      BackupRecord `json:"backup" gorm:"foreignKey:BackupID"`
	ClusterID   uint   `json:"cluster_id" gorm:"index;not null"`
	RestoreName string `json:"restore_name" gorm:"size:128;not null"`

	// 恢复配置
	Namespaces  string `json:"namespaces" gorm:"type:text"`

	// 状态
	Status      string     `json:"status" gorm:"size:20;not null"`
	Errors      int        `json:"errors"`
	Warnings    int        `json:"warnings"`

	// 时间
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (RestoreRecord) TableName() string {
	return "restore_records"
}
