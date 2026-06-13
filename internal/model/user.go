package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Username  string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	Email     string         `json:"email" gorm:"uniqueIndex;size:128;not null"`
	Password  string         `json:"-" gorm:"size:256;not null"`
	RealName  string         `json:"real_name" gorm:"size:64"`
	Phone     string         `json:"phone" gorm:"size:20"`
	Avatar    string         `json:"avatar" gorm:"size:256"`
	Status    int            `json:"status" gorm:"default:1"` // 1:active, 0:disabled
	RoleID    uint           `json:"role_id" gorm:"index"`
	Role      Role           `json:"role" gorm:"foreignKey:RoleID"`
	LastLogin *time.Time     `json:"last_login"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (User) TableName() string {
	return "users"
}

type Role struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:64;not null"`
	Description string         `json:"description" gorm:"size:256"`
	Permissions string         `json:"permissions" gorm:"type:text"` // JSON string
	IsSystem    bool           `json:"is_system" gorm:"default:false"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Role) TableName() string {
	return "roles"
}

type UserCluster struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	UserID          uint      `json:"user_id" gorm:"index;not null"`
	User            User      `json:"user" gorm:"foreignKey:UserID"`
	ClusterID       uint      `json:"cluster_id" gorm:"index;not null"`
	Cluster         Cluster   `json:"cluster" gorm:"foreignKey:ClusterID"`
	Namespace       string    `json:"namespace" gorm:"size:64;default:'*'"`
	PermissionLevel string    `json:"permission_level" gorm:"size:20;default:'read'"` // read, write, admin
	CreatedAt       time.Time `json:"created_at"`
}

func (UserCluster) TableName() string {
	return "user_clusters"
}
