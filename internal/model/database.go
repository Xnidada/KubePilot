package model

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDatabase 初始化数据库连接
// driver: postgres (默认), mysql, sqlite
// 注意: mysql 和 sqlite 驱动需要额外安装:
//
//	go get gorm.io/driver/mysql
//	go get gorm.io/driver/sqlite
func InitDatabase(driver, dsn string, maxIdle, maxOpen int) error {
	var dialector gorm.Dialector

	switch driver {
	case "mysql":
		// 需要安装: go get gorm.io/driver/mysql
		// dialector = mysql.Open(dsn)
		return fmt.Errorf("mysql driver not installed, run: go get gorm.io/driver/mysql")
	case "sqlite":
		// 需要安装: go get gorm.io/driver/sqlite
		// dialector = sqlite.Open(dsn)
		return fmt.Errorf("sqlite driver not installed, run: go get gorm.io/driver/sqlite")
	default: // postgres
		dialector = postgres.Open(dsn)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database (%s): %w", driver, err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if maxIdle <= 0 {
		maxIdle = 10
	}
	if maxOpen <= 0 {
		maxOpen = 100
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)

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
		&ChatConversation{},
		&ChatMessage{},
		&AgentAction{},
		&LLMConfig{},
		// 两步验证
		&UserTwoFactor{},
		// 集群巡检
		&InspectionRule{},
		&InspectionReport{},
		&InspectionResult{},
		// Event 转发
		&EventForwardRule{},
		&EventForwardLog{},
		// SSO/OAuth
		&OAuthConfig{},
		&OAuthUser{},
		// 成本配置
		&CostConfig{},
		// 任务调度
		&TaskQueue{},
		&Task{},
		&TaskLog{},
		&ResourceReservation{},
		&SchedulePolicy{},
	)
}

func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
