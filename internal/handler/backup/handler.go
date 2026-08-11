package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

// Handler 备份处理器
type Handler struct {
	db        *gorm.DB
	scheduler *Scheduler
}

// NewHandler 创建备份处理器
func NewHandler(db *gorm.DB, scheduler ...*Scheduler) *Handler {
	h := &Handler{db: db}
	if len(scheduler) > 0 {
		h.scheduler = scheduler[0]
	}
	return h
}

// SetScheduler attaches a cron scheduler (used by the backup module Start hook).
func (h *Handler) SetScheduler(scheduler *Scheduler) {
	h.scheduler = scheduler
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
	if !authz.EnsureScope(c, "backups", "create", req.ClusterID, "*") {
		return
	}

		if req.TTL == "" {
			req.TTL = "720h" // 30 天
		}

		if err := ValidateCron(req.Schedule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Schedule) == "" {
			response.BadRequest(c, "invalid cron schedule: empty cron expression")
			return
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

	if h.scheduler != nil {
		if err := h.scheduler.Add(schedule); err != nil {
			_ = h.db.Delete(&schedule).Error
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
	}

	response.Created(c, schedule)
}

// DeleteBackupSchedule 删除备份计划
func (h *Handler) DeleteBackupSchedule(c *gin.Context) {
	id := c.Param("id")
	var schedule model.BackupSchedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		response.NotFound(c, "schedule not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "delete", schedule.ClusterID, "*") {
		return
	}
	if err := h.db.Delete(&schedule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
		if h.scheduler != nil {
			h.scheduler.Remove(schedule.ID)
		}
		response.SuccessWithMessage(c, "schedule deleted", nil)
}

// UpdateBackupSchedule updates a backup schedule. Schedule: omit=no change, ""=clear cron.
func (h *Handler) UpdateBackupSchedule(c *gin.Context) {
	id := c.Param("id")
	var schedule model.BackupSchedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		response.NotFound(c, "schedule not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "create", schedule.ClusterID, "*") {
		return
	}

	var req struct {
		Name            string  `json:"name"`
		Namespaces      *[]string `json:"namespaces"`
		Resources       *[]string `json:"resources"`
		Schedule        *string `json:"schedule"`
		TTL             string  `json:"ttl"`
		StorageLocation string  `json:"storage_location"`
		Status          *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Namespaces != nil {
		b, _ := json.Marshal(*req.Namespaces)
		updates["namespaces"] = string(b)
	}
	if req.Resources != nil {
		b, _ := json.Marshal(*req.Resources)
		updates["resources"] = string(b)
	}
	if req.TTL != "" {
		updates["ttl"] = req.TTL
	}
	if req.StorageLocation != "" {
		updates["storage_location"] = req.StorageLocation
	}
	if req.Status != nil {
		st := strings.TrimSpace(*req.Status)
		if st != "active" && st != "paused" {
			response.BadRequest(c, "status must be active or paused")
			return
		}
		updates["status"] = st
	}
	if req.Schedule != nil {
		if err := ValidateCron(*req.Schedule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
		updates["schedule"] = *req.Schedule
		if strings.TrimSpace(*req.Schedule) == "" {
			// Clearing cron also pauses the schedule.
			updates["status"] = "paused"
		}
	}

	if len(updates) == 0 {
		response.Success(c, schedule)
		return
	}
	if err := h.db.Model(&schedule).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&schedule, schedule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		if err := h.scheduler.Sync(schedule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
	}
	response.Success(c, schedule)
}

// PauseBackupSchedule stops cron for a schedule without deleting it.
func (h *Handler) PauseBackupSchedule(c *gin.Context) {
	id := c.Param("id")
	var schedule model.BackupSchedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		response.NotFound(c, "schedule not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "create", schedule.ClusterID, "*") {
		return
	}
	if err := h.db.Model(&schedule).Update("status", "paused").Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&schedule, schedule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		h.scheduler.Remove(schedule.ID)
	}
	response.Success(c, schedule)
}

// ResumeBackupSchedule re-registers cron for a paused schedule.
func (h *Handler) ResumeBackupSchedule(c *gin.Context) {
	id := c.Param("id")
	var schedule model.BackupSchedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		response.NotFound(c, "schedule not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "create", schedule.ClusterID, "*") {
		return
	}
	if strings.TrimSpace(schedule.Schedule) == "" {
		response.BadRequest(c, "cannot resume: cron schedule is empty")
		return
	}
	if err := h.db.Model(&schedule).Update("status", "active").Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&schedule, schedule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		if err := h.scheduler.Add(schedule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
	}
	response.Success(c, schedule)
}

// ClearBackupCron clears the cron expression and pauses the schedule.
func (h *Handler) ClearBackupCron(c *gin.Context) {
	id := c.Param("id")
	var schedule model.BackupSchedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		response.NotFound(c, "schedule not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "create", schedule.ClusterID, "*") {
		return
	}
	if err := h.db.Model(&schedule).Updates(map[string]interface{}{
		"schedule": "",
		"status":   "paused",
	}).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&schedule, schedule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		h.scheduler.Remove(schedule.ID)
	}
	response.Success(c, schedule)
}

// CreateBackup 创建手动备份
func (h *Handler) CreateBackup(c *gin.Context) {
	var req struct {
		ClusterID       uint     `json:"cluster_id" binding:"required"`
		BackupName      string   `json:"backup_name" binding:"required"`
		Namespaces      []string `json:"namespaces"`
		Resources       []string `json:"resources"`
		TTL             string   `json:"ttl"`
		StorageLocation string   `json:"storage_location"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "backups", "create", req.ClusterID, "*") {
		return
	}

	available, err := VeleroAvailable(c.Request.Context(), req.ClusterID)
	if err != nil {
		response.InternalError(c, "failed to detect Velero: "+err.Error())
		return
	}
	if !available {
		response.BadRequest(c, "Velero is not installed in the target cluster; backup remains experimental until Velero CRDs are available")
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

	go h.executeBackup(&record, req.TTL, req.StorageLocation)

	response.Created(c, record)
}

// BackupCapability reports whether real Velero-backed backups are available.
func (h *Handler) BackupCapability(c *gin.Context) {
	clusterID, err := parseUintParam(c.Query("cluster_id"))
	if err != nil || clusterID == 0 {
		response.BadRequest(c, "cluster_id is required")
		return
	}
	if !authz.EnsureScope(c, "backups", "view", uint(clusterID), "*") {
		return
	}
	available, detectErr := VeleroAvailable(c.Request.Context(), uint(clusterID))
	msg := "Velero CRDs detected; backups will be created as real velero.io/v1 Backup objects"
	if detectErr != nil {
		msg = "failed to probe Velero: " + detectErr.Error()
	} else if !available {
		msg = "Velero is not installed; create/restore are blocked to avoid false success"
	}
	response.Success(c, gin.H{
		"cluster_id":       clusterID,
		"velero_available": available,
		"mode":             map[bool]string{true: "velero", false: "experimental_blocked"}[available],
		"message":          msg,
	})
}

func parseUintParam(raw string) (uint64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("empty")
	}
	var n uint64
	_, err := fmt.Sscanf(raw, "%d", &n)
	return n, err
}

// executeBackup creates a Velero Backup CR and waits for a terminal phase.
func (h *Handler) executeBackup(record *model.BackupRecord, ttl, storageLocation string) {
	record.Status = "in_progress"
	record.Phase = "New"
	h.db.Save(record)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var namespaces []string
	var resources []string
	_ = json.Unmarshal([]byte(record.Namespaces), &namespaces)
	_ = json.Unmarshal([]byte(record.Resources), &resources)

	available, err := VeleroAvailable(ctx, record.ClusterID)
	if err != nil || !available {
		record.Status = "failed"
		record.Phase = "FailedValidation"
		record.Errors = 1
		now := time.Now()
		record.CompletedAt = &now
		h.db.Save(record)
		return
	}

	if err := createVeleroBackup(ctx, record.ClusterID, record.BackupName, ttl, storageLocation, namespaces, resources); err != nil {
		record.Status = "failed"
		record.Phase = "Failed"
		record.Errors = 1
		now := time.Now()
		record.CompletedAt = &now
		h.db.Save(record)
		return
	}

	phase, waitErr := waitVeleroBackup(ctx, record.ClusterID, record.BackupName, 10*time.Minute)
	record.Phase = phase
	now := time.Now()
	record.CompletedAt = &now
	switch phase {
	case "Completed":
		record.Status = "completed"
		record.Errors = 0
	case "PartiallyFailed":
		record.Status = "completed"
		record.Warnings = 1
	default:
		record.Status = "failed"
		record.Errors = 1
		if waitErr != nil && phase == "" {
			record.Phase = "Timeout"
		}
	}
	h.db.Save(record)
}

// RunScheduledBackup creates a backup record from a schedule and executes it.
func (h *Handler) RunScheduledBackup(schedule *model.BackupSchedule) {
	now := time.Now()
	sid := schedule.ID
	record := model.BackupRecord{
		ScheduleID:  &sid,
		ClusterID:   schedule.ClusterID,
		BackupName:  fmt.Sprintf("%s-%d", schedule.Name, now.Unix()),
		Namespaces:  schedule.Namespaces,
		Resources:   schedule.Resources,
		Status:      "pending",
		StartedAt:   now,
	}
	if err := h.db.Create(&record).Error; err != nil {
		return
	}
	h.executeBackup(&record, schedule.TTL, schedule.StorageLocation)
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
	if !authz.EnsureScope(c, "backups", "view", record.ClusterID, "*") {
		return
	}
	response.Success(c, record)
}

// DeleteBackupRecord deletes a backup record and best-effort removes the Velero Backup CR.
func (h *Handler) DeleteBackupRecord(c *gin.Context) {
	id := c.Param("id")
	var record model.BackupRecord
	if err := h.db.First(&record, id).Error; err != nil {
		response.NotFound(c, "backup not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "delete", record.ClusterID, "*") {
		return
	}

	if record.Status == "pending" || record.Status == "in_progress" {
		response.BadRequest(c, "cannot delete backup while it is pending or in progress")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if available, err := VeleroAvailable(ctx, record.ClusterID); err == nil && available {
		if err := deleteVeleroBackup(ctx, record.ClusterID, record.BackupName); err != nil {
			response.InternalError(c, "failed to delete Velero Backup: "+err.Error())
			return
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("backup_id = ?", record.ID).Delete(&model.RestoreRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(&record).Error
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "backup deleted", nil)
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
	if !authz.EnsureScope(c, "backups", "execute", backup.ClusterID, "*") {
		return
	}
	if !authz.EnsureScope(c, "backups", "execute", req.ClusterID, "*") {
		return
	}
	if backup.Status != "completed" {
		response.BadRequest(c, "backup is not completed")
		return
	}
	available, err := VeleroAvailable(c.Request.Context(), req.ClusterID)
	if err != nil {
		response.InternalError(c, "failed to detect Velero: "+err.Error())
		return
	}
	if !available {
		response.BadRequest(c, "Velero is not installed in the target cluster; restore remains experimental until Velero CRDs are available")
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

	go h.executeRestore(&restore, backup.BackupName)

	response.Created(c, restore)
}

func (h *Handler) executeRestore(record *model.RestoreRecord, backupName string) {
	record.Status = "in_progress"
	h.db.Save(record)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var namespaces []string
	_ = json.Unmarshal([]byte(record.Namespaces), &namespaces)

	if err := createVeleroRestore(ctx, record.ClusterID, record.RestoreName, backupName, namespaces); err != nil {
		record.Status = "failed"
		record.Errors = 1
		now := time.Now()
		record.CompletedAt = &now
		h.db.Save(record)
		return
	}

	phase, waitErr := waitVeleroRestore(ctx, record.ClusterID, record.RestoreName, 10*time.Minute)
	now := time.Now()
	record.CompletedAt = &now
	switch phase {
	case "Completed":
		record.Status = "completed"
		record.Errors = 0
	case "PartiallyFailed":
		record.Status = "completed"
		record.Warnings = 1
	default:
		record.Status = "failed"
		record.Errors = 1
		if waitErr != nil {
			record.Errors = 1
		}
	}
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

// DeleteRestoreRecord deletes a restore record and best-effort removes the Velero Restore CR.
func (h *Handler) DeleteRestoreRecord(c *gin.Context) {
	id := c.Param("id")
	var record model.RestoreRecord
	if err := h.db.First(&record, id).Error; err != nil {
		response.NotFound(c, "restore not found")
		return
	}
	if !authz.EnsureScope(c, "backups", "delete", record.ClusterID, "*") {
		return
	}
	if record.Status == "pending" || record.Status == "in_progress" {
		response.BadRequest(c, "cannot delete restore while it is pending or in progress")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if available, err := VeleroAvailable(ctx, record.ClusterID); err == nil && available {
		if err := deleteVeleroRestore(ctx, record.ClusterID, record.RestoreName); err != nil {
			response.InternalError(c, "failed to delete Velero Restore: "+err.Error())
			return
		}
	}
	if err := h.db.Delete(&record).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "restore deleted", nil)
}
