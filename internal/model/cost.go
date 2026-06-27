package model

import (
	"time"
)

// CostConfig 资源成本配置
type CostConfig struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ClusterID    uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster      Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	CPUPerUnit   float64   `json:"cpu_per_unit" gorm:"default:0.032"`    // 每 mCPU 每小时成本（美元）
	MemPerUnit   float64   `json:"mem_per_unit" gorm:"default:0.004"`    // 每 MB 每小时成本（美元）
	GPUPerUnit   float64   `json:"gpu_per_unit" gorm:"default:1.5"`      // 每 GPU 每小时成本（美元）
	Currency     string    `json:"currency" gorm:"size:10;default:'USD'"` // 货币单位
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (CostConfig) TableName() string {
	return "cost_configs"
}
