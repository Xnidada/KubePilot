package model

import "gorm.io/gorm"

// Serialize all default/config mutations, including the empty-table case.
func WithLLMConfigWrite(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE llm_configs IN EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

func EnsureSingleActiveLLMConfig(db *gorm.DB) error {
	return WithLLMConfigWrite(db, func(tx *gorm.DB) error {
		// Preserve the same highest-ID default selected by older releases.
		if err := tx.Exec("UPDATE llm_configs SET is_active = false WHERE is_active = true AND id <> (SELECT MAX(id) FROM llm_configs WHERE is_active = true)").Error; err != nil {
			return err
		}
		return tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS llm_configs_one_active ON llm_configs (is_active) WHERE is_active = true").Error
	})
}
