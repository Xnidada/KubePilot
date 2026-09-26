package model

import (
	"fmt"

	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"gorm.io/gorm"
)

// SealStoredSecrets migrates legacy plaintext credentials before HTTP starts.
func SealStoredSecrets(db *gorm.DB, key string) error {
	if db.Migrator().HasTable(&LLMConfig{}) {
		var configs []LLMConfig
		if err := db.Find(&configs).Error; err != nil {
			return err
		}
		for _, cfg := range configs {
			if _, err := crypto.OpenSecret(cfg.APIKey, key); err != nil {
				return fmt.Errorf("LLM config %d: %w", cfg.ID, err)
			}
			sealed, err := crypto.SealSecret(cfg.APIKey, key)
			if err != nil {
				return err
			}
			if sealed != cfg.APIKey {
				if err := db.Model(&LLMConfig{}).Where("id = ? AND api_key = ?", cfg.ID, cfg.APIKey).UpdateColumn("api_key", sealed).Error; err != nil {
					return err
				}
			}
		}
	}
	var providers []OAuthConfig
	if err := db.Find(&providers).Error; err != nil {
		return err
	}
	for _, cfg := range providers {
		for column, value := range map[string]string{"client_secret": cfg.ClientSecret, "ldap_bind_pass": cfg.LDAPBindPass} {
			if _, err := crypto.OpenSecret(value, key); err != nil {
				return fmt.Errorf("OAuth config %d %s: %w", cfg.ID, column, err)
			}
			sealed, err := crypto.SealSecret(value, key)
			if err != nil {
				return err
			}
			if sealed != value {
				if err := db.Model(&OAuthConfig{}).Where("id = ? AND "+column+" = ?", cfg.ID, value).UpdateColumn(column, sealed).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}
