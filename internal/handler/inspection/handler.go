package inspection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InspectionHandler 集群巡检处理器
type InspectionHandler struct {
	db        *gorm.DB
	scheduler *Scheduler
}

func NewInspectionHandler(db *gorm.DB, scheduler ...*Scheduler) *InspectionHandler {
	h := &InspectionHandler{db: db}
	if len(scheduler) > 0 {
		h.scheduler = scheduler[0]
	}
	return h
}

// SetScheduler attaches the cron scheduler used by the inspection module.
func (h *InspectionHandler) SetScheduler(scheduler *Scheduler) {
	h.scheduler = scheduler
}

// ListRules 获取巡检规则列表
func (h *InspectionHandler) ListRules(c *gin.Context) {
	var rules []model.InspectionRule
	query := h.db.Order("created_at DESC")

	if clusterID := c.Query("cluster_id"); clusterID != "" {
		query = query.Where("cluster_id = ?", clusterID)
	}

	if err := query.Find(&rules).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, rules)
}

// CreateRule 创建巡检规则
func (h *InspectionHandler) CreateRule(c *gin.Context) {
	var rule model.InspectionRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "inspection", "create", rule.ClusterID, "*") {
		return
	}

	if err := ValidateCron(rule.Schedule); err != nil {
		response.BadRequest(c, "invalid cron schedule: "+err.Error())
		return
	}
	if reason := inspectionRuleError(&rule); reason != "" {
		response.BadRequest(c, reason)
		return
	}

	rule.Enabled = true
	if err := h.db.Create(&rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.scheduler != nil {
		if err := h.scheduler.Sync(rule); err != nil {
			_ = h.db.Delete(&rule).Error
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
	}

	response.Created(c, rule)
}

// GetRule 获取巡检规则详情
func (h *InspectionHandler) GetRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "view", rule.ClusterID, "*") {
		return
	}

	response.Success(c, rule)
}

// UpdateRule 更新巡检规则
func (h *InspectionHandler) UpdateRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "edit", rule.ClusterID, "*") {
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Resource    *string `json:"resource"`
		CheckType   *string `json:"check_type"`
		Condition   *string `json:"condition"`
		Threshold   *string `json:"threshold"`
		Script      *string `json:"script"`
		Enabled     *bool   `json:"enabled"`
		// Schedule: omit = no change; "" = clear (manual-only); non-empty = set cron.
		Schedule *string `json:"schedule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	candidate := rule
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Resource != nil {
		candidate.Resource = *req.Resource
		updates["resource"] = *req.Resource
	}
	if req.CheckType != nil {
		candidate.CheckType = *req.CheckType
		updates["check_type"] = *req.CheckType
	}
	if req.Condition != nil {
		candidate.Condition = *req.Condition
		updates["condition"] = *req.Condition
	}
	if req.Threshold != nil {
		candidate.Threshold = *req.Threshold
		updates["threshold"] = *req.Threshold
	}
	if req.Script != nil {
		candidate.Script = *req.Script
		updates["script"] = *req.Script
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Schedule != nil {
		if err := ValidateCron(*req.Schedule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
		updates["schedule"] = *req.Schedule
	}
	if reason := inspectionRuleError(&candidate); reason != "" {
		response.BadRequest(c, reason)
		return
	}

	if err := h.db.Model(&rule).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&rule, rule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		if err := h.scheduler.Sync(rule); err != nil {
			response.BadRequest(c, "invalid cron schedule: "+err.Error())
			return
		}
	}

	response.Success(c, rule)
}

// DeleteRule 删除巡检规则
func (h *InspectionHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "delete", rule.ClusterID, "*") {
		return
	}
	if h.scheduler != nil {
		h.scheduler.Remove(rule.ID)
	}
	if err := h.db.Delete(&rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "rule deleted", nil)
}

// ClearSchedule removes the cron expression so the rule becomes manual-only.
func (h *InspectionHandler) ClearSchedule(c *gin.Context) {
	id := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "edit", rule.ClusterID, "*") {
		return
	}
	if err := h.db.Model(&rule).Update("schedule", "").Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if err := h.db.First(&rule, rule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if h.scheduler != nil {
		h.scheduler.Remove(rule.ID)
	}
	response.Success(c, rule)
}

// RunInspection 执行巡检
func (h *InspectionHandler) RunInspection(c *gin.Context) {
	ruleID := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, ruleID).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "execute", rule.ClusterID, "*") {
		return
	}

	// 创建巡检报告
	report := model.InspectionReport{
		RuleID:    rule.ID,
		ClusterID: rule.ClusterID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := h.db.Create(&report).Error; err != nil {
		response.InternalError(c, "创建巡检报告失败: "+err.Error())
		return
	}

	// 执行巡检
	go h.executeInspection(&report, &rule)

	response.Success(c, gin.H{
		"report_id": report.ID,
		"status":    "running",
	})
}

// RunScheduledInspection creates a report and runs the rule asynchronously.
func (h *InspectionHandler) RunScheduledInspection(rule *model.InspectionRule) {
	report := model.InspectionReport{
		RuleID:    rule.ID,
		ClusterID: rule.ClusterID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := h.db.Create(&report).Error; err != nil {
		return
	}
	go h.executeInspection(&report, rule)
}

// executeInspection 执行巡检逻辑
func (h *InspectionHandler) executeInspection(report *model.InspectionReport, rule *model.InspectionRule) {
	if reason := inspectionRuleError(rule); reason != "" {
		report.Status = "failed"
		report.Error = reason
		now := time.Now()
		report.CompletedAt = &now
		h.db.Save(report)
		return
	}
	client, err := k8s.Manager.GetClient(rule.ClusterID)
	if err != nil {
		report.Status = "failed"
		report.Error = fmt.Sprintf("cluster connection failed: %v", err)
		now := time.Now()
		report.CompletedAt = &now
		h.db.Save(report)
		return
	}

	ctx := context.Background()
	results := []model.InspectionResult{}

	switch rule.Resource {
	case "node":
		results = h.checkNodes(ctx, client, rule)
	case "pod":
		results = h.checkPods(ctx, client, rule)
	case "deployment":
		results = h.checkDeployments(ctx, client, rule)
	case "service":
		results = h.checkServices(ctx, client, rule)
	}

	// 保存结果；空结果不能被当作一次通过的检查。
	for i := range results {
		results[i].ReportID = report.ID
	}
	report.Status, report.Passed, report.Failed, report.Warnings, report.Error = summarizeInspectionResults(results)
	report.TotalChecks = len(results)
	if len(results) > 0 {
		if err := h.db.Create(&results).Error; err != nil {
			report.Status = "failed"
			report.Error = fmt.Sprintf("保存巡检结果失败: %v", err)
		}
	}
	now := time.Now()
	report.CompletedAt = &now
	h.db.Save(report)
}

func summarizeInspectionResults(results []model.InspectionResult) (status string, passed, failed, warnings int, reason string) {
	if len(results) == 0 {
		return "failed", 0, 0, 0, "未发现可检查资源，无法判定巡检通过"
	}
	for _, result := range results {
		switch result.Status {
		case "pass":
			passed++
		case "warn":
			warnings++
		default:
			failed++
		}
	}
	if failed > 0 {
		return "failed", passed, failed, warnings, fmt.Sprintf("%d 项检查失败", failed)
	}
	return "completed", passed, failed, warnings, ""
}

// checkNodes 检查节点状态
func (h *InspectionHandler) checkNodes(ctx context.Context, client *k8s.ClusterClient, rule *model.InspectionRule) []model.InspectionResult {
	var results []model.InspectionResult

	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, model.InspectionResult{
			ResourceType: "node",
			ResourceName: "all",
			Status:       "fail",
			Message:      fmt.Sprintf("获取节点列表失败: %v", err),
		})
		return results
	}

	for _, node := range nodes.Items {
		result := model.InspectionResult{
			ResourceType: "node",
			ResourceName: node.Name,
		}

		// 检查节点状态
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				ready = cond.Status == "True"
				break
			}
		}

		if ready {
			result.Status = "pass"
			result.Message = "节点状态正常"
		} else {
			result.Status = "fail"
			result.Message = "节点状态异常: NotReady"
		}

		results = append(results, result)
	}

	return results
}

// checkPods 检查 Pod 状态
func (h *InspectionHandler) checkPods(ctx context.Context, client *k8s.ClusterClient, rule *model.InspectionRule) []model.InspectionResult {
	var results []model.InspectionResult

	pods, err := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, model.InspectionResult{
			ResourceType: "pod",
			ResourceName: "all",
			Status:       "fail",
			Message:      fmt.Sprintf("获取 Pod 列表失败: %v", err),
		})
		return results
	}

	for _, pod := range pods.Items {
		result := model.InspectionResult{
			ResourceType: "pod",
			ResourceName: pod.Name,
			Namespace:    pod.Namespace,
		}

		switch pod.Status.Phase {
		case "Running":
			ready := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
				}
			}
			if len(pod.Status.ContainerStatuses) == 0 {
				ready = false
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
				}
			}
			if ready {
				result.Status = "pass"
				result.Message = "Pod 运行中且容器已就绪"
			} else {
				result.Status = "fail"
				result.Message = "Pod 运行中但容器未就绪"
			}
		case "Succeeded":
			result.Status = "pass"
			result.Message = fmt.Sprintf("Pod 状态: %s", pod.Status.Phase)
		case "Pending":
			result.Status = "warn"
			result.Message = "Pod 处于 Pending 状态"
		default:
			result.Status = "fail"
			result.Message = fmt.Sprintf("Pod 状态异常: %s", pod.Status.Phase)
		}

		// 检查重启次数
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 {
				if result.Status == "pass" {
					result.Status = "warn"
				}
				result.Message += fmt.Sprintf(", 容器 %s 重启 %d 次", cs.Name, cs.RestartCount)
			}
		}

		results = append(results, result)
	}

	return results
}

// checkDeployments 检查 Deployment 状态
func (h *InspectionHandler) checkDeployments(ctx context.Context, client *k8s.ClusterClient, rule *model.InspectionRule) []model.InspectionResult {
	var results []model.InspectionResult

	deploys, err := client.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, model.InspectionResult{
			ResourceType: "deployment",
			ResourceName: "all",
			Status:       "fail",
			Message:      fmt.Sprintf("获取 Deployment 列表失败: %v", err),
		})
		return results
	}

	for _, deploy := range deploys.Items {
		result := model.InspectionResult{
			ResourceType: "deployment",
			ResourceName: deploy.Name,
			Namespace:    deploy.Namespace,
		}

		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}

		if desired == 0 {
			result.Status = "warn"
			result.Message = "Deployment 已缩容至 0，未验证工作负载可用性"
		} else if deploy.Status.ObservedGeneration >= deploy.Generation && deploy.Status.ReadyReplicas == desired && deploy.Status.AvailableReplicas == desired && deploy.Status.UpdatedReplicas == desired {
			result.Status = "pass"
			result.Message = fmt.Sprintf("最新版本副本已就绪且可用: %d/%d", deploy.Status.ReadyReplicas, desired)
		} else {
			result.Status = "fail"
			result.Message = fmt.Sprintf("部署未就绪: 就绪 %d、可用 %d、已更新 %d、期望 %d", deploy.Status.ReadyReplicas, deploy.Status.AvailableReplicas, deploy.Status.UpdatedReplicas, desired)
		}

		results = append(results, result)
	}

	return results
}

// checkServices 检查 Service 状态
func (h *InspectionHandler) checkServices(ctx context.Context, client *k8s.ClusterClient, rule *model.InspectionRule) []model.InspectionResult {
	var results []model.InspectionResult

	services, err := client.Clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		results = append(results, model.InspectionResult{
			ResourceType: "service",
			ResourceName: "all",
			Status:       "fail",
			Message:      fmt.Sprintf("获取 Service 列表失败: %v", err),
		})
		return results
	}

	for _, svc := range services.Items {
		result := model.InspectionResult{
			ResourceType: "service",
			ResourceName: svc.Name,
			Namespace:    svc.Namespace,
		}

		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			result.Status = "warn"
			result.Message = fmt.Sprintf("ClusterIP 配置存在: %s；未验证后端可用性", svc.Spec.ClusterIP)
		} else {
			result.Status = "warn"
			result.Message = "无 ClusterIP（可能为 Headless/ExternalName）；未验证后端可用性"
		}

		results = append(results, result)
	}

	return results
}

// inspectionRuleError keeps unsupported checks from being saved or reported as passing.
func inspectionRuleError(rule *model.InspectionRule) string {
	if strings.TrimSpace(rule.Script) != "" || rule.CheckType == "custom" {
		return "自定义脚本巡检尚未实现，未执行检查"
	}
	if strings.TrimSpace(rule.Condition) != "" || strings.TrimSpace(rule.Threshold) != "" {
		return "条件/阈值巡检尚未实现，未执行检查"
	}
	if rule.CheckType != "" && rule.CheckType != "status" {
		return fmt.Sprintf("检查类型 %q 尚未实现，未执行检查", rule.CheckType)
	}
	switch rule.Resource {
	case "node", "pod", "deployment", "service":
		return ""
	default:
		return fmt.Sprintf("检查资源 %q 尚未实现，未执行检查", rule.Resource)
	}
}

// ListReports 获取巡检报告列表
func (h *InspectionHandler) ListReports(c *gin.Context) {
	var reports []model.InspectionReport
	query := h.db.Order("created_at DESC")

	if clusterID := c.Query("cluster_id"); clusterID != "" {
		query = query.Where("cluster_id = ?", clusterID)
	}

	if ruleID := c.Query("rule_id"); ruleID != "" {
		query = query.Where("rule_id = ?", ruleID)
	}

	if err := query.Find(&reports).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, reports)
}

// GetReport 获取巡检报告详情
func (h *InspectionHandler) GetReport(c *gin.Context) {
	id := c.Param("id")
	var report model.InspectionReport
	if err := h.db.First(&report, id).Error; err != nil {
		response.NotFound(c, "report not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "view", report.ClusterID, "*") {
		return
	}

	response.Success(c, report)
}

// GetReportResults 获取巡检报告结果
func (h *InspectionHandler) GetReportResults(c *gin.Context) {
	id := c.Param("id")
	var report model.InspectionReport
	if err := h.db.First(&report, id).Error; err != nil {
		response.NotFound(c, "report not found")
		return
	}
	if !authz.EnsureScope(c, "inspection", "view", report.ClusterID, "*") {
		return
	}
	var results []model.InspectionResult
	if err := h.db.Where("report_id = ?", id).Find(&results).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, results)
}
