package model

import (
	"time"

	"gorm.io/gorm"
)

type AppTemplate struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"size:128;not null;index"`
	ChartRepo   string         `json:"chart_repo" gorm:"size:256;not null"`
	ChartName   string         `json:"chart_name" gorm:"size:128;not null"`
	Version     string         `json:"version" gorm:"size:32"`
	Description string         `json:"description" gorm:"size:1024"`
	Icon        string         `json:"icon" gorm:"size:512"`
	Category    string         `json:"category" gorm:"size:64;index"`
	Keywords    string         `json:"keywords" gorm:"size:512"` // JSON array
	Values      string         `json:"values" gorm:"type:text"` // default values.yaml
	Readme      string         `json:"readme" gorm:"type:text"`
	Downloads   int            `json:"downloads" gorm:"default:0"`
	Enabled     bool           `json:"enabled" gorm:"default:true"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (AppTemplate) TableName() string {
	return "app_templates"
}

type AppDeployment struct {
	ID         uint         `json:"id" gorm:"primaryKey"`
	UserID     uint         `json:"user_id" gorm:"index;not null"`
	User       User         `json:"user" gorm:"foreignKey:UserID"`
	ClusterID  uint         `json:"cluster_id" gorm:"index;not null"`
	Cluster    Cluster      `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace  string       `json:"namespace" gorm:"size:64;not null"`
	TemplateID uint         `json:"template_id" gorm:"index"`
	Template   AppTemplate  `json:"template" gorm:"foreignKey:TemplateID"`
	ReleaseName string      `json:"release_name" gorm:"size:128;not null;index"`
	ChartName  string       `json:"chart_name" gorm:"size:128;not null"`
	ChartVersion string     `json:"chart_version" gorm:"size:32"`
	Values     string       `json:"values" gorm:"type:text"` // JSON values
	Status     string       `json:"status" gorm:"size:20;default:'pending'"` // pending, deploying, deployed, failed, uninstalling
	Message    string       `json:"message" gorm:"type:text"`
	Revision   int          `json:"revision" gorm:"default:1"`
	DeployedAt *time.Time   `json:"deployed_at"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (AppDeployment) TableName() string {
	return "app_deployments"
}

type ChartRepository struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"uniqueIndex;size:128;not null"`
	URL       string         `json:"url" gorm:"size:512;not null"`
	Username  string         `json:"username" gorm:"size:128"`
	Password  string         `json:"-" gorm:"size:256"`
	Type      string         `json:"type" gorm:"size:20;default:'helm'"` // helm, oci
	Enabled   bool           `json:"enabled" gorm:"default:true"`
	LastSync  *time.Time     `json:"last_sync"`
	Status    string         `json:"status" gorm:"size:20;default:'unknown'"` // unknown, synced, error
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ChartRepository) TableName() string {
	return "chart_repositories"
}
