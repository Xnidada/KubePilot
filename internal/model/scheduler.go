package model

import (
	"time"
)

// TaskQueue 任务队列
type TaskQueue struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"uniqueIndex;size:64;not null"`
	DisplayName string `json:"display_name" gorm:"size:128"`
	Description string `json:"description" gorm:"size:512"`

	// 队列配置
	Priority int `json:"priority" gorm:"default:0"`     // 队列优先级
	Weight   int `json:"weight" gorm:"default:1"`       // 资源分配权重

	// 资源配额
	MaxCPU    string `json:"max_cpu" gorm:"size:32"`    // 最大 CPU
	MaxMemory string `json:"max_memory" gorm:"size:32"` // 最大内存
	MaxGPU    int    `json:"max_gpu"`                    // 最大 GPU 数量
	MaxTasks  int    `json:"max_tasks" gorm:"default:100"` // 最大任务数

	// 调度策略
	Policy     string `json:"policy" gorm:"size:32;default:'fifo'"` // fifo, priority, fair
	Preemption bool   `json:"preemption" gorm:"default:false"`      // 是否允许抢占

	// 状态
	Status    string `json:"status" gorm:"size:20;default:'active'"` // active, paused, disabled
	TaskCount int    `json:"task_count" gorm:"-"`                    // 运行中任务数

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TaskQueue) TableName() string {
	return "task_queues"
}

// Task 任务定义
type Task struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	TaskID string `json:"task_id" gorm:"column:task_uid;uniqueIndex;size:64;not null"` // 任务唯一标识
	Name   string `json:"name" gorm:"size:128;not null"`
	// 任务归属
	UserID    uint `json:"user_id" gorm:"index;not null"`
	User      User `json:"user" gorm:"foreignKey:UserID"`
	QueueID   uint `json:"queue_id" gorm:"index;not null"`
	Queue     TaskQueue `json:"queue" gorm:"foreignKey:QueueID"`
	ClusterID uint `json:"cluster_id" gorm:"index;not null"`
	Cluster   Cluster `json:"cluster" gorm:"foreignKey:ClusterID"`

	// 任务类型
	TaskType string `json:"task_type" gorm:"size:32;not null"` // job, cronjob, volcano, mpi, pytorch
	Priority int    `json:"priority" gorm:"default:0"`         // 任务优先级 0-1000

	// 资源需求
	CPU     string `json:"cpu" gorm:"size:32"`
	Memory  string `json:"memory" gorm:"size:32"`
	GPU     int    `json:"gpu"`
	GPUType string `json:"gpu_type" gorm:"size:32"`

	// Gang Scheduling 配置
	MinReplicas int `json:"min_replicas" gorm:"default:1"`
	Replicas    int `json:"replicas" gorm:"default:1"`

	// 任务配置
	Image      string `json:"image" gorm:"size:256"`
	Command    string `json:"command" gorm:"type:text"`
	Args       string `json:"args" gorm:"type:text"`
	EnvVars    string `json:"env_vars" gorm:"type:text"`
	VolumeMounts string `json:"volume_mounts" gorm:"type:text"`

	// 调度约束
	NodeSelector string `json:"node_selector" gorm:"type:text"`
	Tolerations  string `json:"tolerations" gorm:"type:text"`

	// 超时与重试
	Timeout    int `json:"timeout" gorm:"default:3600"`
	MaxRetry   int `json:"max_retry" gorm:"default:3"`
	RetryCount int `json:"retry_count"`

	// 状态
	Status  string `json:"status" gorm:"size:32;default:'pending'"` // pending, queued, running, succeeded, failed, cancelled
	Message string `json:"message" gorm:"type:text"`

	// K8S 资源引用
	Namespace  string `json:"namespace" gorm:"size:64;default:'default'"`
	K8SJobName string `json:"k8s_job_name" gorm:"size:128"`

	// 时间戳
	SubmittedAt *time.Time `json:"submitted_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Task) TableName() string {
	return "tasks"
}

// TaskLog 任务日志
type TaskLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"task_id" gorm:"column:task_ref_id;index"`
	Level     string    `json:"level" gorm:"size:20"` // info, warn, error
	Message   string    `json:"message" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

func (TaskLog) TableName() string {
	return "task_logs"
}

// ResourceReservation 资源预留
type ResourceReservation struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"size:128;not null"`
	UserID    uint   `json:"user_id" gorm:"index;not null"`
	User      User   `json:"user" gorm:"foreignKey:UserID"`
	QueueID   uint   `json:"queue_id" gorm:"index;not null"`
	Queue     TaskQueue `json:"queue" gorm:"foreignKey:QueueID"`
	ClusterID uint   `json:"cluster_id" gorm:"index;not null"`
	Cluster   Cluster `json:"cluster" gorm:"foreignKey:ClusterID"`

	// 预留资源
	CPU     string `json:"cpu" gorm:"size:32"`
	Memory  string `json:"memory" gorm:"size:32"`
	GPU     int    `json:"gpu"`
	GPUType string `json:"gpu_type" gorm:"size:32"`

	// 预留策略
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Recurring bool      `json:"recurring"`
	CronExpr  string    `json:"cron_expr" gorm:"size:64"`

	// 节点绑定
	NodeName     string `json:"node_name" gorm:"size:128"`
	NodeSelector string `json:"node_selector" gorm:"type:text"`

	// 状态
	Status string `json:"status" gorm:"size:20;default:'active'"` // active, expired, cancelled

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ResourceReservation) TableName() string {
	return "resource_reservations"
}

// SchedulePolicy 调度策略
type SchedulePolicy struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"uniqueIndex;size:64;not null"`
	Description string `json:"description" gorm:"size:256"`

	// 策略类型
	Type   string `json:"type" gorm:"size:32;not null"` // priority, fair, gang, binpack, spread
	Config string `json:"config" gorm:"type:text"`       // JSON 配置

	// 适用范围
	ClusterID *uint `json:"cluster_id"`
	QueueID   *uint `json:"queue_id"`

	IsDefault bool      `json:"is_default" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SchedulePolicy) TableName() string {
	return "schedule_policies"
}
