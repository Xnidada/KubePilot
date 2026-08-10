package model

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDatabase 初始化数据库连接
// driver 当前仅支持 postgres。
func InitDatabase(driver, dsn string, maxIdle, maxOpen int) error {
	var dialector gorm.Dialector

	switch driver {
	case "", "postgres":
		dialector = postgres.Open(dsn)
	default:
		return fmt.Errorf("unsupported database driver %q; only postgres is available", driver)
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
	if err := DB.AutoMigrate(
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
		// 多租户
		&Tenant{},
		&TenantNamespace{},
		&TenantMember{},
		// 备份
		&BackupSchedule{},
		&BackupRecord{},
		&RestoreRecord{},
		// Webhook
		&WebhookConfig{},
		&WebhookLog{},
	); err != nil {
		return err
	}

	// Audit logs are historical records and must survive user/cluster deletion.
	// They also need to record attempted access to IDs that never existed.
	for _, constraint := range []string{"fk_audit_logs_user", "fk_audit_logs_cluster"} {
		if DB.Migrator().HasConstraint(&AuditLog{}, constraint) {
			if err := DB.Migrator().DropConstraint(&AuditLog{}, constraint); err != nil {
				return fmt.Errorf("drop audit log constraint %s: %w", constraint, err)
			}
		}
	}
	return nil
}

func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
