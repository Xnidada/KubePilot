package backup

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

// Handler 备份处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建备份处理器
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// ListBackupSchedules 获取备份计划列表
func (h *Handler) ListBackupSchedules(c *gin.Context) {
	var schedules []model.BackupSchedule
	if err := h.db.Preload("Cluster").Order("created_at DESC").Find(&schedules).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, schedules)
}

// CreateBackupSchedule 创建备份计划
func (h *Handler) CreateBackupSchedule(c *gin.Context) {
	var req struct {
		Name            string   `json:"name" binding:"required"`
		ClusterID       uint     `json:"cluster_id" binding:"required"`
		Namespaces      []string `json:"namespaces"`
		Resources       []string `json:"resources"`
		Schedule        string   `json:"schedule" binding:"required"`
		TTL             string   `json:"ttl"`
		StorageLocation string   `json:"storage_location"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.TTL == "" {
		req.TTL = "720h" // 30 天
	}

	namespacesJSON, _ := json.Marshal(req.Namespaces)
	resourcesJSON, _ := json.Marshal(req.Resources)

	schedule := model.BackupSchedule{
		Name:            req.Name,
		ClusterID:       req.ClusterID,
		Namespaces:      string(namespacesJSON),
		Resources:       string(resourcesJSON),
		Schedule:        req.Schedule,
		TTL:             req.TTL,
		StorageLocation: req.StorageLocation,
		Status:          "active",
	}

	if err := h.db.Create(&schedule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, schedule)
}

// DeleteBackupSchedule 删除备份计划
func (h *Handler) DeleteBackupSchedule(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.BackupSchedule{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "schedule deleted", nil)
}

// CreateBackup 创建手动备份
func (h *Handler) CreateBackup(c *gin.Context) {
	var req struct {
		ClusterID  uint     `json:"cluster_id" binding:"required"`
		BackupName string   `json:"backup_name" binding:"required"`
		Namespaces []string `json:"namespaces"`
		Resources  []string `json:"resources"`
		TTL        string   `json:"ttl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.TTL == "" {
		req.TTL = "720h"
	}

	namespacesJSON, _ := json.Marshal(req.Namespaces)
	resourcesJSON, _ := json.Marshal(req.Resources)

	now := time.Now()
	record := model.BackupRecord{
		ClusterID:   req.ClusterID,
		BackupName:  req.BackupName,
		Namespaces:  string(namespacesJSON),
		Resources:   string(resourcesJSON),
		Status:      "pending",
		StartedAt:   now,
	}

	if err := h.db.Create(&record).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 异步执行备份
	go h.executeBackup(&record, req.TTL)

	response.Created(c, record)
}

// executeBackup 执行备份（模拟）
func (h *Handler) executeBackup(record *model.BackupRecord, ttl string) {
	// 更新状态为执行中
	record.Status = "in_progress"
	h.db.Save(record)

	// 模拟备份过程
	time.Sleep(5 * time.Second)

	// 这里应该调用 Velero API 创建备份
	// 目前模拟成功
	record.Status = "completed"
	record.VolumeSnapshots = 0
	record.Errors = 0
	record.Warnings = 0
	now := time.Now()
	record.CompletedAt = &now
	h.db.Save(record)
}

// ListBackupRecords 获取备份记录列表
func (h *Handler) ListBackupRecords(c *gin.Context) {
	var records []model.BackupRecord
	if err := h.db.Preload("Cluster").Preload("Schedule").Order("created_at DESC").Limit(100).Find(&records).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, records)
}

// GetBackupRecord 获取备份记录详情
func (h *Handler) GetBackupRecord(c *gin.Context) {
	id := c.Param("id")
	var record model.BackupRecord
	if err := h.db.Preload("Cluster").Preload("Schedule").First(&record, id).Error; err != nil {
		response.NotFound(c, "backup not found")
		return
	}
	response.Success(c, record)
}

// CreateRestore 创建恢复
func (h *Handler) CreateRestore(c *gin.Context) {
	var req struct {
		BackupID    uint     `json:"backup_id" binding:"required"`
		ClusterID   uint     `json:"cluster_id" binding:"required"`
		Namespaces  []string `json:"namespaces"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var backup model.BackupRecord
	if err := h.db.First(&backup, req.BackupID).Error; err != nil {
		response.NotFound(c, "backup not found")
		return
	}

	namespacesJSON, _ := json.Marshal(req.Namespaces)

	now := time.Now()
	restore := model.RestoreRecord{
		BackupID:    req.BackupID,
		ClusterID:   req.ClusterID,
		RestoreName: fmt.Sprintf("restore-%d", now.Unix()),
		Namespaces:  string(namespacesJSON),
		Status:      "pending",
		StartedAt:   now,
	}

	if err := h.db.Create(&restore).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 异步执行恢复
	go h.executeRestore(&restore)

	response.Created(c, restore)
}

// executeRestore 执行恢复（模拟）
func (h *Handler) executeRestore(record *model.RestoreRecord) {
	record.Status = "in_progress"
	h.db.Save(record)

	// 模拟恢复过程
	time.Sleep(5 * time.Second)

	record.Status = "completed"
	record.Errors = 0
	record.Warnings = 0
	now := time.Now()
	record.CompletedAt = &now
	h.db.Save(record)
}

// ListRestoreRecords 获取恢复记录列表
func (h *Handler) ListRestoreRecords(c *gin.Context) {
	var records []model.RestoreRecord
	if err := h.db.Preload("Backup").Preload("Cluster").Order("created_at DESC").Limit(100).Find(&records).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, records)
}
