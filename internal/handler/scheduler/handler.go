package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Handler 调度器处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建调度器处理器
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// ==================== 队列管理 ====================

// ListQueues 获取队列列表
func (h *Handler) ListQueues(c *gin.Context) {
	var queues []model.TaskQueue
	if err := h.db.Order("priority DESC, created_at DESC").Find(&queues).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 统计每个队列的任务数
	result := make([]gin.H, 0, len(queues))
	for _, q := range queues {
		var runningCount, pendingCount int64
		h.db.Model(&model.Task{}).Where("queue_id = ? AND status = ?", q.ID, "running").Count(&runningCount)
		h.db.Model(&model.Task{}).Where("queue_id = ? AND status IN (?)", q.ID, []string{"pending", "queued"}).Count(&pendingCount)

		result = append(result, gin.H{
			"id":          q.ID,
			"name":        q.Name,
			"display_name": q.DisplayName,
			"description": q.Description,
			"priority":    q.Priority,
			"weight":      q.Weight,
			"max_cpu":     q.MaxCPU,
			"max_memory":  q.MaxMemory,
			"max_gpu":     q.MaxGPU,
			"max_tasks":   q.MaxTasks,
			"policy":      q.Policy,
			"preemption":  q.Preemption,
			"status":      q.Status,
			"running_tasks": runningCount,
			"pending_tasks": pendingCount,
			"created_at":  q.CreatedAt,
		})
	}

	response.Success(c, result)
}

// CreateQueue 创建队列
func (h *Handler) CreateQueue(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		Weight      int    `json:"weight"`
		MaxCPU      string `json:"max_cpu"`
		MaxMemory   string `json:"max_memory"`
		MaxGPU      int    `json:"max_gpu"`
		MaxTasks    int    `json:"max_tasks"`
		Policy      string `json:"policy"`
		Preemption  bool   `json:"preemption"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 设置默认值
	if req.Weight == 0 {
		req.Weight = 1
	}
	if req.MaxTasks == 0 {
		req.MaxTasks = 100
	}
	if req.Policy == "" {
		req.Policy = "fifo"
	}

	queue := model.TaskQueue{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Priority:    req.Priority,
		Weight:      req.Weight,
		MaxCPU:      req.MaxCPU,
		MaxMemory:   req.MaxMemory,
		MaxGPU:      req.MaxGPU,
		MaxTasks:    req.MaxTasks,
		Policy:      req.Policy,
		Preemption:  req.Preemption,
		Status:      "active",
	}

	if err := h.db.Create(&queue).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, queue)
}

// GetQueue 获取队列详情
func (h *Handler) GetQueue(c *gin.Context) {
	id := c.Param("id")
	var queue model.TaskQueue
	if err := h.db.First(&queue, id).Error; err != nil {
		response.NotFound(c, "queue not found")
		return
	}

	// 统计任务数
	var runningCount, pendingCount, totalCount int64
	h.db.Model(&model.Task{}).Where("queue_id = ? AND status = ?", queue.ID, "running").Count(&runningCount)
	h.db.Model(&model.Task{}).Where("queue_id = ? AND status IN (?)", queue.ID, []string{"pending", "queued"}).Count(&pendingCount)
	h.db.Model(&model.Task{}).Where("queue_id = ?", queue.ID).Count(&totalCount)

	result := gin.H{
		"queue":         queue,
		"running_tasks": runningCount,
		"pending_tasks": pendingCount,
		"total_tasks":   totalCount,
	}

	response.Success(c, result)
}

// UpdateQueue 更新队列
func (h *Handler) UpdateQueue(c *gin.Context) {
	id := c.Param("id")
	var queue model.TaskQueue
	if err := h.db.First(&queue, id).Error; err != nil {
		response.NotFound(c, "queue not found")
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Priority    *int   `json:"priority"`
		Weight      *int   `json:"weight"`
		MaxCPU      string `json:"max_cpu"`
		MaxMemory   string `json:"max_memory"`
		MaxGPU      *int   `json:"max_gpu"`
		MaxTasks    *int   `json:"max_tasks"`
		Policy      string `json:"policy"`
		Preemption  *bool  `json:"preemption"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 更新字段
	if req.DisplayName != "" {
		queue.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		queue.Description = req.Description
	}
	if req.Priority != nil {
		queue.Priority = *req.Priority
	}
	if req.Weight != nil {
		queue.Weight = *req.Weight
	}
	if req.MaxCPU != "" {
		queue.MaxCPU = req.MaxCPU
	}
	if req.MaxMemory != "" {
		queue.MaxMemory = req.MaxMemory
	}
	if req.MaxGPU != nil {
		queue.MaxGPU = *req.MaxGPU
	}
	if req.MaxTasks != nil {
		queue.MaxTasks = *req.MaxTasks
	}
	if req.Policy != "" {
		queue.Policy = req.Policy
	}
	if req.Preemption != nil {
		queue.Preemption = *req.Preemption
	}
	if req.Status != "" {
		queue.Status = req.Status
	}

	if err := h.db.Save(&queue).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, queue)
}

// DeleteQueue 删除队列
func (h *Handler) DeleteQueue(c *gin.Context) {
	id := c.Param("id")

	// 检查是否有运行中的任务
	var count int64
	h.db.Model(&model.Task{}).Where("queue_id = ? AND status IN (?)", id, []string{"pending", "queued", "running"}).Count(&count)
	if count > 0 {
		response.BadRequest(c, "cannot delete queue with active tasks")
		return
	}

	if err := h.db.Delete(&model.TaskQueue{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "queue deleted", nil)
}

// ==================== 任务管理 ====================

// ListTasks 获取任务列表
func (h *Handler) ListTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	queueID := c.Query("queue_id")
	status := c.Query("status")
	userID := c.Query("user_id")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	query := h.db.Model(&model.Task{})

	if queueID != "" {
		query = query.Where("queue_id = ?", queueID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	var total int64
	query.Count(&total)

	var tasks []model.Task
	if err := query.Preload("Queue").Preload("User").
		Order("priority DESC, created_at DESC").
		Offset((page - 1) * size).Limit(size).
		Find(&tasks).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.PageSuccess(c, tasks, total, page, size)
}

// CreateTask 提交任务
func (h *Handler) CreateTask(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		Name        string            `json:"name" binding:"required"`
		QueueID     uint              `json:"queue_id" binding:"required"`
		ClusterID   uint              `json:"cluster_id" binding:"required"`
		TaskType    string            `json:"task_type" binding:"required"`
		Priority    int               `json:"priority"`
		CPU         string            `json:"cpu"`
		Memory      string            `json:"memory"`
		GPU         int               `json:"gpu"`
		GPUType     string            `json:"gpu_type"`
		Replicas    int               `json:"replicas"`
		MinReplicas int               `json:"min_replicas"`
		Image       string            `json:"image" binding:"required"`
		Command     []string          `json:"command"`
		Args        []string          `json:"args"`
		EnvVars     map[string]string `json:"env_vars"`
		Namespace   string            `json:"namespace"`
		Timeout     int               `json:"timeout"`
		MaxRetry    int               `json:"max_retry"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 设置默认值
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.MinReplicas == 0 {
		req.MinReplicas = 1
	}
	if req.Timeout == 0 {
		req.Timeout = 3600
	}
	if req.MaxRetry == 0 {
		req.MaxRetry = 3
	}
	if req.CPU == "" {
		req.CPU = "100m"
	}
	if req.Memory == "" {
		req.Memory = "128Mi"
	}

	// 验证队列存在
	var queue model.TaskQueue
	if err := h.db.First(&queue, req.QueueID).Error; err != nil {
		response.BadRequest(c, "queue not found")
		return
	}

	// 检查队列任务数限制
	var taskCount int64
	h.db.Model(&model.Task{}).Where("queue_id = ? AND status IN (?)", req.QueueID, []string{"pending", "queued", "running"}).Count(&taskCount)
	if int(taskCount) >= queue.MaxTasks {
		response.BadRequest(c, "queue task limit reached")
		return
	}

	// 生成任务 ID
	taskID := fmt.Sprintf("task-%s-%d", req.Name, time.Now().UnixNano())

	// 序列化命令和参数
	commandJSON, _ := json.Marshal(req.Command)
	argsJSON, _ := json.Marshal(req.Args)
	envJSON, _ := json.Marshal(req.EnvVars)

	// 创建任务
	now := time.Now()
	task := model.Task{
		TaskID:      taskID,
		Name:        req.Name,
		UserID:      userID.(uint),
		QueueID:     req.QueueID,
		ClusterID:   req.ClusterID,
		TaskType:    req.TaskType,
		Priority:    req.Priority,
		CPU:         req.CPU,
		Memory:      req.Memory,
		GPU:         req.GPU,
		GPUType:     req.GPUType,
		MinReplicas: req.MinReplicas,
		Replicas:    req.Replicas,
		Image:       req.Image,
		Command:     string(commandJSON),
		Args:        string(argsJSON),
		EnvVars:     string(envJSON),
		Namespace:   req.Namespace,
		Timeout:     req.Timeout,
		MaxRetry:    req.MaxRetry,
		Status:      "pending",
		SubmittedAt: &now,
	}

	if err := h.db.Create(&task).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 记录日志
	h.addTaskLog(task.ID, "info", "任务已提交")

	// 异步执行任务
	go h.executeTask(&task)

	response.Created(c, task)
}

// GetTask 获取任务详情
func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	var task model.Task
	if err := h.db.Preload("Queue").Preload("User").First(&task, id).Error; err != nil {
		response.NotFound(c, "task not found")
		return
	}

	// 获取最近日志
	var logs []model.TaskLog
	h.db.Table("task_logs").Where("task_ref_id = ?", task.ID).Order("created_at DESC").Limit(50).Find(&logs)

	result := gin.H{
		"task": task,
		"logs": logs,
	}

	response.Success(c, result)
}

// CancelTask 取消任务
func (h *Handler) CancelTask(c *gin.Context) {
	id := c.Param("id")
	var task model.Task
	if err := h.db.First(&task, id).Error; err != nil {
		response.NotFound(c, "task not found")
		return
	}

	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		response.BadRequest(c, "task already completed")
		return
	}

	// 如果任务正在运行，删除 K8S Job
	if task.Status == "running" && task.K8SJobName != "" {
		client, err := k8s.Manager.GetClient(task.ClusterID)
		if err == nil {
			client.Clientset.BatchV1().Jobs(task.Namespace).Delete(ctx, task.K8SJobName, metav1.DeleteOptions{})
		}
	}

	// 更新状态
	task.Status = "cancelled"
	now := time.Now()
	task.CompletedAt = &now
	h.db.Save(&task)

	h.addTaskLog(task.ID, "info", "任务已取消")

	response.SuccessWithMessage(c, "task cancelled", nil)
}

// RetryTask 重试任务
func (h *Handler) RetryTask(c *gin.Context) {
	id := c.Param("id")
	var task model.Task
	if err := h.db.First(&task, id).Error; err != nil {
		response.NotFound(c, "task not found")
		return
	}

	if task.Status != "failed" && task.Status != "cancelled" {
		response.BadRequest(c, "only failed or cancelled tasks can be retried")
		return
	}

	if task.RetryCount >= task.MaxRetry {
		response.BadRequest(c, "max retry count reached")
		return
	}

	// 重置状态
	task.Status = "pending"
	task.Message = ""
	task.RetryCount++
	task.StartedAt = nil
	task.CompletedAt = nil
	h.db.Save(&task)

	h.addTaskLog(task.ID, "info", fmt.Sprintf("任务重试 (%d/%d)", task.RetryCount, task.MaxRetry))

	// 异步执行任务
	go h.executeTask(&task)

	response.SuccessWithMessage(c, "task retrying", nil)
}

// GetTaskLogs 获取任务日志
func (h *Handler) GetTaskLogs(c *gin.Context) {
	id := c.Param("id")
	var task model.Task
	if err := h.db.First(&task, id).Error; err != nil {
		response.NotFound(c, "task not found")
		return
	}

	var logs []model.TaskLog
	h.db.Table("task_logs").Where("task_ref_id = ?", task.ID).Order("created_at DESC").Limit(200).Find(&logs)

	// 如果任务正在运行，尝试获取 K8S 日志
	if task.Status == "running" && task.K8SJobName != "" {
		client, err := k8s.Manager.GetClient(task.ClusterID)
		if err == nil {
			pods, _ := client.Clientset.CoreV1().Pods(task.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("job-name=%s", task.K8SJobName),
			})
			if pods != nil && len(pods.Items) > 0 {
				pod := pods.Items[0]
				req := client.Clientset.CoreV1().Pods(task.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
					TailLines: int64Ptr(100),
				})
				stream, err := req.Stream(ctx)
				if err == nil {
					defer stream.Close()
					// 读取日志...
				}
			}
		}
	}

	response.Success(c, logs)
}

// ==================== 资源预留 ====================

// ListReservations 获取预留列表
func (h *Handler) ListReservations(c *gin.Context) {
	var reservations []model.ResourceReservation
	query := h.db.Order("created_at DESC")

	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Find(&reservations).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, reservations)
}

// CreateReservation 创建预留
func (h *Handler) CreateReservation(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		Name        string    `json:"name" binding:"required"`
		QueueID     uint      `json:"queue_id" binding:"required"`
		ClusterID   uint      `json:"cluster_id" binding:"required"`
		CPU         string    `json:"cpu"`
		Memory      string    `json:"memory"`
		GPU         int       `json:"gpu"`
		GPUType     string    `json:"gpu_type"`
		StartTime   time.Time `json:"start_time" binding:"required"`
		EndTime     time.Time `json:"end_time" binding:"required"`
		Recurring   bool      `json:"recurring"`
		CronExpr    string    `json:"cron_expr"`
		NodeName    string    `json:"node_name"`
		NodeSelector map[string]string `json:"node_selector"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.EndTime.Before(req.StartTime) {
		response.BadRequest(c, "end_time must be after start_time")
		return
	}

	nodeSelectorJSON, _ := json.Marshal(req.NodeSelector)

	reservation := model.ResourceReservation{
		Name:         req.Name,
		UserID:       userID.(uint),
		QueueID:      req.QueueID,
		ClusterID:    req.ClusterID,
		CPU:          req.CPU,
		Memory:       req.Memory,
		GPU:          req.GPU,
		GPUType:      req.GPUType,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		Recurring:    req.Recurring,
		CronExpr:     req.CronExpr,
		NodeName:     req.NodeName,
		NodeSelector: string(nodeSelectorJSON),
		Status:       "active",
	}

	if err := h.db.Create(&reservation).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, reservation)
}

// DeleteReservation 取消预留
func (h *Handler) DeleteReservation(c *gin.Context) {
	id := c.Param("id")
	var reservation model.ResourceReservation
	if err := h.db.First(&reservation, id).Error; err != nil {
		response.NotFound(c, "reservation not found")
		return
	}

	reservation.Status = "cancelled"
	h.db.Save(&reservation)

	response.SuccessWithMessage(c, "reservation cancelled", nil)
}

// ==================== 内部方法 ====================

// executeTask 执行任务
func (h *Handler) executeTask(task *model.Task) {
	ctx := context.Background()

	// 更新状态为 queued
	task.Status = "queued"
	h.db.Save(task)
	h.addTaskLog(task.ID, "info", "任务已进入队列")

	// 获取集群客户端
	client, err := k8s.Manager.GetClient(task.ClusterID)
	if err != nil {
		task.Status = "failed"
		task.Message = fmt.Sprintf("集群连接失败: %v", err)
		now := time.Now()
		task.CompletedAt = &now
		h.db.Save(task)
		h.addTaskLog(task.ID, "error", task.Message)
		return
	}

	// 解析命令和参数
	var command, args []string
	json.Unmarshal([]byte(task.Command), &command)
	json.Unmarshal([]byte(task.Args), &args)

	// 解析环境变量
	var envVars map[string]string
	json.Unmarshal([]byte(task.EnvVars), &envVars)

	// 构建环境变量列表
	envList := make([]corev1.EnvVar, 0)
	for k, v := range envVars {
		envList = append(envList, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// 创建 K8S Job
	jobName := task.TaskID
	backoffLimit := int32(task.MaxRetry)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: task.Namespace,
			Labels: map[string]string{
				"kubepilot/task-id": task.TaskID,
				"kubepilot/queue":   fmt.Sprintf("%d", task.QueueID),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "task",
							Image:   task.Image,
							Command: command,
							Args:    args,
							Env:     envList,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(task.CPU),
									corev1.ResourceMemory: resource.MustParse(task.Memory),
								},
							},
						},
					},
				},
			},
		},
	}

	// 添加 GPU 资源
	if task.GPU > 0 {
		gpuType := task.GPUType
		if gpuType == "" {
			gpuType = "nvidia.com/gpu"
		}
		job.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceName(gpuType)] = resource.MustParse(fmt.Sprintf("%d", task.GPU))
	}

	// 创建 Job
	_, err = client.Clientset.BatchV1().Jobs(task.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		task.Status = "failed"
		task.Message = fmt.Sprintf("创建 Job 失败: %v", err)
		now := time.Now()
		task.CompletedAt = &now
		h.db.Save(task)
		h.addTaskLog(task.ID, "error", task.Message)
		return
	}

	// 更新状态
	task.Status = "running"
	task.K8SJobName = jobName
	now := time.Now()
	task.StartedAt = &now
	h.db.Save(task)
	h.addTaskLog(task.ID, "info", "任务已开始执行")

	// 监控任务状态
	go h.monitorTask(task)
}

// monitorTask 监控任务状态
func (h *Handler) monitorTask(task *model.Task) {
	ctx := context.Background()
	client, err := k8s.Manager.GetClient(task.ClusterID)
	if err != nil {
		return
	}

	// 等待任务完成
	for {
		time.Sleep(10 * time.Second)

		job, err := client.Clientset.BatchV1().Jobs(task.Namespace).Get(ctx, task.K8SJobName, metav1.GetOptions{})
		if err != nil {
			// Job 可能已被删除
			task.Status = "failed"
			task.Message = "Job 已被删除"
			now := time.Now()
			task.CompletedAt = &now
			h.db.Save(task)
			h.addTaskLog(task.ID, "error", "Job 已被删除")
			return
		}

		// 检查是否完成
		if job.Status.Succeeded > 0 {
			task.Status = "succeeded"
			task.Message = "任务执行成功"
			now := time.Now()
			task.CompletedAt = &now
			h.db.Save(task)
			h.addTaskLog(task.ID, "info", "任务执行成功")
			return
		}

		// 检查是否失败
		if job.Status.Failed > 0 {
			task.Status = "failed"
			task.Message = "任务执行失败"
			now := time.Now()
			task.CompletedAt = &now
			h.db.Save(task)
			h.addTaskLog(task.ID, "error", "任务执行失败")
			return
		}

		// 检查超时
		if task.StartedAt != nil && time.Since(*task.StartedAt) > time.Duration(task.Timeout)*time.Second {
			// 删除 Job
			client.Clientset.BatchV1().Jobs(task.Namespace).Delete(ctx, task.K8SJobName, metav1.DeleteOptions{})
			task.Status = "failed"
			task.Message = "任务执行超时"
			now := time.Now()
			task.CompletedAt = &now
			h.db.Save(task)
			h.addTaskLog(task.ID, "error", "任务执行超时")
			return
		}
	}
}

// addTaskLog 添加任务日志
func (h *Handler) addTaskLog(taskID uint, level, message string) {
	log := model.TaskLog{
		TaskID:  taskID,
		Level:   level,
		Message: message,
	}
	h.db.Table("task_logs").Create(&log)
}

// int64Ptr 返回 int64 指针
func int64Ptr(n int64) *int64 {
	return &n
}

// 全局 context（用于异步任务）
var ctx = context.Background()
