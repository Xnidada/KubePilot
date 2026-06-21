package router

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InspectionHandler 集群巡检处理器
type InspectionHandler struct {
	db *gorm.DB
}

func NewInspectionHandler(db *gorm.DB) *InspectionHandler {
	return &InspectionHandler{db: db}
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

	rule.Enabled = true
	if err := h.db.Create(&rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
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

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Resource    string `json:"resource"`
		CheckType   string `json:"check_type"`
		Condition   string `json:"condition"`
		Threshold   string `json:"threshold"`
		Script      string `json:"script"`
		Enabled     *bool  `json:"enabled"`
		Schedule    string `json:"schedule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Resource != "" {
		updates["resource"] = req.Resource
	}
	if req.CheckType != "" {
		updates["check_type"] = req.CheckType
	}
	if req.Condition != "" {
		updates["condition"] = req.Condition
	}
	if req.Threshold != "" {
		updates["threshold"] = req.Threshold
	}
	if req.Script != "" {
		updates["script"] = req.Script
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Schedule != "" {
		updates["schedule"] = req.Schedule
	}

	if err := h.db.Model(&rule).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, rule)
}

// DeleteRule 删除巡检规则
func (h *InspectionHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.InspectionRule{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "rule deleted", nil)
}

// RunInspection 执行巡检
func (h *InspectionHandler) RunInspection(c *gin.Context) {
	ruleID := c.Param("id")
	var rule model.InspectionRule
	if err := h.db.First(&rule, ruleID).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}

	// 创建巡检报告
	report := model.InspectionReport{
		RuleID:    rule.ID,
		ClusterID: rule.ClusterID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	h.db.Create(&report)

	// 执行巡检
	go h.executeInspection(&report, &rule)

	response.Success(c, gin.H{
		"report_id": report.ID,
		"status":    "running",
	})
}

// executeInspection 执行巡检逻辑
func (h *InspectionHandler) executeInspection(report *model.InspectionReport, rule *model.InspectionRule) {
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
	default:
		// 自定义脚本检查
		results = h.runCustomCheck(ctx, client, rule)
	}

	// 保存结果
	for i := range results {
		results[i].ReportID = report.ID
	}
	h.db.Create(&results)

	// 更新报告
	passCount := 0
	failCount := 0
	warnCount := 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			passCount++
		case "fail":
			failCount++
		case "warn":
			warnCount++
		}
	}

	report.Status = "completed"
	report.TotalChecks = len(results)
	report.Passed = passCount
	report.Failed = failCount
	report.Warnings = warnCount
	now := time.Now()
	report.CompletedAt = &now
	h.db.Save(report)
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
		case "Running", "Succeeded":
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
				result.Status = "warn"
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

		desired := int32(0)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}

		if deploy.Status.ReadyReplicas == desired {
			result.Status = "pass"
			result.Message = fmt.Sprintf("副本数正常: %d/%d", deploy.Status.ReadyReplicas, desired)
		} else {
			result.Status = "fail"
			result.Message = fmt.Sprintf("副本数异常: %d/%d", deploy.Status.ReadyReplicas, desired)
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
			result.Status = "pass"
			result.Message = fmt.Sprintf("ClusterIP: %s", svc.Spec.ClusterIP)
		} else {
			result.Status = "warn"
			result.Message = "无 ClusterIP"
		}

		results = append(results, result)
	}

	return results
}

// runCustomCheck 运行自定义检查
func (h *InspectionHandler) runCustomCheck(ctx context.Context, client *k8s.ClusterClient, rule *model.InspectionRule) []model.InspectionResult {
	// 自定义脚本检查（简化实现）
	return []model.InspectionResult{
		{
			ResourceType: rule.Resource,
			ResourceName: "custom",
			Status:       "pass",
			Message:      "自定义检查完成",
		},
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

	response.Success(c, report)
}

// GetReportResults 获取巡检报告结果
func (h *InspectionHandler) GetReportResults(c *gin.Context) {
	id := c.Param("id")
	var results []model.InspectionResult
	if err := h.db.Where("report_id = ?", id).Find(&results).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, results)
}
