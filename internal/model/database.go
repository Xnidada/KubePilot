package model

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDatabase(dsn string) error {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	return nil
}

func AutoMigrate() error {
	return DB.AutoMigrate(
		&User{},
		&Role{},
		&Cluster{},
		&ClusterNode{},
		&Namespace{},
		&UserCluster{},
		&AuditLog{},
		&AlertRule{},
		&AlertHistory{},
		&NotificationChannel{},
		&AppTemplate{},
		&AppDeployment{},
		&ChartRepository{},
	)
}

func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
