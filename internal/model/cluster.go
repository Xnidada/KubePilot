package model

import (
	"time"

	"gorm.io/gorm"
)

type Cluster struct {
	ID                   uint           `json:"id" gorm:"primaryKey"`
	Name                 string         `json:"name" gorm:"uniqueIndex;size:128;not null"`
	DisplayName          string         `json:"display_name" gorm:"size:128"`
	Description          string         `json:"description" gorm:"size:512"`
	APIServer            string         `json:"api_server" gorm:"size:256;not null"`
	Kubeconfig           string         `json:"-" gorm:"type:text"` // encrypted
	Status               string         `json:"status" gorm:"size:20;default:'unknown'"` // unknown, connected, disconnected, error
	Version              string         `json:"version" gorm:"size:20"`
	NodeCount            int            `json:"node_count" gorm:"default:0"`
	CPUCapacity          string         `json:"cpu_capacity" gorm:"size:32"`
	MemoryCapacity       string         `json:"memory_capacity" gorm:"size:32"`
	PodCapacity          string         `json:"pod_capacity" gorm:"size:32"`
	LastHealthCheck      *time.Time     `json:"last_health_check"`
	LastHealthCheckError string         `json:"last_health_check_error" gorm:"type:text"`
	Tags                 string         `json:"tags" gorm:"size:512"` // JSON string
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Cluster) TableName() string {
	return "clusters"
}

type ClusterNode struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ClusterID    uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster      Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Name         string    `json:"name" gorm:"size:128;not null"`
	Role         string    `json:"role" gorm:"size:32"` // master, worker
	Status       string    `json:"status" gorm:"size:20"`
	IP           string    `json:"ip" gorm:"size:45"`
	CPUCapacity  string    `json:"cpu_capacity" gorm:"size:32"`
	MemCapacity  string    `json:"mem_capacity" gorm:"size:32"`
	PodCapacity  string    `json:"pod_capacity" gorm:"size:32"`
	OS           string    `json:"os" gorm:"size:64"`
	Kernel       string    `json:"kernel" gorm:"size:64"`
	ContainerRT  string    `json:"container_rt" gorm:"size:64"`
	KubeletVer   string    `json:"kubelet_ver" gorm:"size:32"`
	Labels       string    `json:"labels" gorm:"type:text"` // JSON string
	Conditions   string    `json:"conditions" gorm:"type:text"` // JSON string
	LastSynced   time.Time `json:"last_synced"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ClusterNode) TableName() string {
	return "cluster_nodes"
}

type Namespace struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ClusterID uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster   Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Name      string    `json:"name" gorm:"size:128;not null"`
	Status    string    `json:"status" gorm:"size:20"`
	Labels    string    `json:"labels" gorm:"type:text"` // JSON string
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Namespace) TableName() string {
	return "namespaces"
}
