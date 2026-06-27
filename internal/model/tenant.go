package model

import (
	"time"
)

// Tenant 租户/团队
type Tenant struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"uniqueIndex;size:64;not null"`
	DisplayName string `json:"display_name" gorm:"size:128"`
	Description string `json:"description" gorm:"size:512"`

	// 资源配额
	MaxCPU      string `json:"max_cpu" gorm:"size:32"`
	MaxMemory   string `json:"max_memory" gorm:"size:32"`
	MaxGPU      int    `json:"max_gpu"`
	MaxNamespaces int  `json:"max_namespaces" gorm:"default:5"`
	MaxPods     int    `json:"max_pods" gorm:"default:50"`

	// 状态
	Status    string    `json:"status" gorm:"size:20;default:'active'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Tenant) TableName() string {
	return "tenants"
}

// TenantNamespace 租户命名空间
type TenantNamespace struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TenantID  uint      `json:"tenant_id" gorm:"index;not null"`
	Tenant    Tenant    `json:"tenant" gorm:"foreignKey:TenantID"`
	ClusterID uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster   Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Name      string    `json:"name" gorm:"size:64;not null"`
	CreatedAt time.Time `json:"created_at"`
}

func (TenantNamespace) TableName() string {
	return "tenant_namespaces"
}

// TenantMember 租户成员
type TenantMember struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TenantID  uint      `json:"tenant_id" gorm:"index;not null"`
	Tenant    Tenant    `json:"tenant" gorm:"foreignKey:TenantID"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	Role      string    `json:"role" gorm:"size:32;default:'member'"` // owner, admin, member, viewer
	CreatedAt time.Time `json:"created_at"`
}

func (TenantMember) TableName() string {
	return "tenant_members"
}
