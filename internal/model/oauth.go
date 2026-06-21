package model

import (
	"time"
)

// OAuthConfig OAuth 配置
type OAuthConfig struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Provider     string    `json:"provider" gorm:"uniqueIndex;size:32;not null"` // github, gitlab, google, ldap
	Name         string    `json:"name" gorm:"size:64;not null"`
	ClientID     string    `json:"client_id" gorm:"size:256"`
	ClientSecret string    `json:"-" gorm:"size:256"` // 不返回给前端
	RedirectURL  string    `json:"redirect_url" gorm:"size:512"`
	// OAuth endpoints
	AuthURL      string    `json:"auth_url" gorm:"size:512"`
	TokenURL     string    `json:"token_url" gorm:"size:512"`
	UserInfoURL  string    `json:"userinfo_url" gorm:"size:512"`
	Scopes       string    `json:"scopes" gorm:"type:text"` // JSON array
	// LDAP 配置
	LDAPHost     string    `json:"ldap_host" gorm:"size:256"`
	LDAPPort     int       `json:"ldap_port"`
	LDAPBaseDN   string    `json:"ldap_base_dn" gorm:"size:256"`
	LDAPBindDN   string    `json:"ldap_bind_dn" gorm:"size:256"`
	LDAPBindPass string    `json:"-" gorm:"size:256"`
	LDAPUserAttr string    `json:"ldap_user_attr" gorm:"size:64"`
	LDAPFilter   string    `json:"ldap_filter" gorm:"size:512"`
	// 通用配置
	Enabled      bool      `json:"enabled" gorm:"default:true"`
	DefaultRole  uint      `json:"default_role" gorm:"default:2"` // 默认角色 ID
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (OAuthConfig) TableName() string {
	return "oauth_configs"
}

// OAuthUser OAuth 用户关联
type OAuthUser struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"user_id" gorm:"index;not null"`
	User       User      `json:"user" gorm:"foreignKey:UserID"`
	Provider   string    `json:"provider" gorm:"size:32;not null"`
	ExternalID string    `json:"external_id" gorm:"size:256;not null"` // 外部用户 ID
	Username   string    `json:"username" gorm:"size:128"`
	Email      string    `json:"email" gorm:"size:128"`
	Avatar     string    `json:"avatar" gorm:"size:512"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (OAuthUser) TableName() string {
	return "oauth_users"
}
