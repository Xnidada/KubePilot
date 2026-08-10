package model

import (
	"time"

	"gorm.io/gorm"
)

// UserGroup groups users so cluster grants can be managed in one place.
type UserGroup struct {
	ID          uint              `json:"id" gorm:"primaryKey"`
	Name        string            `json:"name" gorm:"uniqueIndex;size:64;not null"`
	Description string            `json:"description" gorm:"size:256"`
	Status      int               `json:"status" gorm:"not null;default:1;index"`
	Members     []UserGroupMember `json:"members,omitempty" gorm:"foreignKey:GroupID"`
	Clusters    []GroupCluster    `json:"clusters,omitempty" gorm:"foreignKey:GroupID"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	DeletedAt   gorm.DeletedAt    `json:"-" gorm:"index"`
}

func (UserGroup) TableName() string {
	return "user_groups"
}

// UserGroupMember associates a user with a user group.
type UserGroupMember struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	GroupID   uint      `json:"group_id" gorm:"not null;uniqueIndex:idx_group_user"`
	Group     UserGroup `json:"group,omitempty" gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UserID    uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_group_user;index"`
	User      User      `json:"user,omitempty" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Status    int       `json:"status" gorm:"not null;default:1;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserGroupMember) TableName() string {
	return "user_group_members"
}

// GroupCluster grants a group access to one cluster and namespace.
type GroupCluster struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	GroupID         uint      `json:"group_id" gorm:"not null;uniqueIndex:idx_group_cluster_namespace"`
	Group           UserGroup `json:"group,omitempty" gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ClusterID       uint      `json:"cluster_id" gorm:"not null;uniqueIndex:idx_group_cluster_namespace;index"`
	Cluster         Cluster   `json:"cluster,omitempty" gorm:"foreignKey:ClusterID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Namespace       string    `json:"namespace" gorm:"size:64;not null;default:'*';uniqueIndex:idx_group_cluster_namespace"`
	PermissionLevel string    `json:"permission_level" gorm:"size:20;not null;default:'read'"`
	Status          int       `json:"status" gorm:"not null;default:1;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (GroupCluster) TableName() string {
	return "group_clusters"
}
