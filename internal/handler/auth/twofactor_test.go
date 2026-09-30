package auth

import (
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
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

func TestTwoFactorConsumptionPersistsWithCompareAndSwap(t *testing.T) {
	for _, scenario := range []string{"backup", "totp", "already_consumed", "database_error"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"),
				&gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { sqlDB.Close() })
			secret := []byte("test-secret")
			record := model.UserTwoFactor{ID: 7, UserID: 9, IsEnabled: true,
				Secret:      base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret),
				BackupCodes: `["abcd1234","efgh5678"]`}
			// Supply a read snapshot without a live database, but retain GORM's
			// real UPDATE builder to assert the atomic SQL and fail-closed result.
			db.Callback().Query().Replace("gorm:query", func(tx *gorm.DB) {
				*tx.Statement.Dest.(*model.UserTwoFactor) = record
				tx.RowsAffected = 1
			})
			writeErr := errors.New("write unavailable")
			updated := false
			db.Callback().Update().After("gorm:update").Register("test:result", func(tx *gorm.DB) {
				updated = true
				sql := tx.Statement.SQL.String()
				parts := strings.SplitN(sql, " WHERE ", 2)
				if len(parts) != 2 || !strings.Contains(parts[1], "is_enabled =") || !strings.Contains(parts[1], "secret =") {
					t.Fatalf("missing enabled/identity preconditions: %s", sql)
				}
				updates := tx.Statement.Dest.(map[string]any)
				_, writesCodes := updates["backup_codes"]
				if scenario == "totp" {
					if writesCodes {
						t.Fatal("TOTP login must not restore the snapshot's backup codes")
					}
				} else if !strings.Contains(parts[1], "backup_codes =") || updates["backup_codes"] != `["efgh5678"]` {
					t.Fatalf("backup consumption is not guarded by the original list: %s %+v", sql, updates)
				}
				tx.RowsAffected = 1
				if scenario == "already_consumed" {
					tx.RowsAffected = 0
				}
				if scenario == "database_error" {
					tx.AddError(writeErr)
				}
			})
			code := "abcd1234"
			if scenario == "totp" {
				code = generateTOTP(secret, time.Now().Unix()/30)
			}
			backupUsed, err := consumeTwoFactorCode(db, 9, code)
			if !updated {
				t.Fatal("verification did not attempt persistence")
			}
			switch scenario {
			case "already_consumed":
				if !errors.Is(err, errInvalidTwoFactorCode) || backupUsed {
					t.Fatalf("lost CAS accepted: %v %v", backupUsed, err)
				}
			case "database_error":
				if !errors.Is(err, writeErr) || backupUsed {
					t.Fatalf("failed persistence accepted: %v %v", backupUsed, err)
				}
			default:
				if err != nil || backupUsed != (scenario == "backup") {
					t.Fatalf("verification failed: %v %v", backupUsed, err)
				}
			}
		})
	}
}
