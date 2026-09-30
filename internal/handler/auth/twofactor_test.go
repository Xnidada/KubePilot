package auth

import (
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTwoFactorCheckFailsClosedOnDatabaseError(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable connect_timeout=1"),
		&gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	required, err := CheckTwoFactorRequired(db, 1)
	if err == nil || required {
		t.Fatalf("database failure must not be treated as disabled 2FA: required=%v err=%v", required, err)
	}
}
