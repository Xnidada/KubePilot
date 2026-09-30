// Package testutil provides opt-in PostgreSQL integration test isolation.
package testutil

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Postgres never migrates the application's schema. Every test gets its own
// generated search_path and drops only that schema on completion.
func Postgres(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("KUBEPILOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set KUBEPILOT_TEST_POSTGRES_DSN for isolated PostgreSQL integration tests")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test PostgreSQL configuration")
	}
	admin := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("kubepilot_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	})
	cfg.RuntimeParams["search_path"] = schema
	cfg.RuntimeParams["application_name"] = schema
	sqlDB := stdlib.OpenDB(*cfg)
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() { sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// WaitForRowLock proves a competing statement is blocked, without relying on
// scheduler timing or fixed sleeps to reproduce the race.
func WaitForRowLock(t *testing.T, db *gorm.DB) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		if err := db.Raw("SELECT COUNT(*) FROM pg_stat_activity WHERE application_name = current_schema() AND wait_event_type = 'Lock'").Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("competing statement did not wait for the row lock")
}
