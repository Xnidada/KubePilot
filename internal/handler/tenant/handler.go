package tenant

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Handler 租户处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建租户处理器
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// ListTenants 获取租户列表
func (h *Handler) ListTenants(c *gin.Context) {
	var tenants []model.Tenant
	if err := h.db.Order("created_at DESC").Find(&tenants).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 统计每个租户的成员数和命名空间数
	result := make([]gin.H, 0, len(tenants))
	for _, t := range tenants {
		var memberCount, nsCount int64
		h.db.Model(&model.TenantMember{}).Where("tenant_id = ?", t.ID).Count(&memberCount)
		h.db.Model(&model.TenantNamespace{}).Where("tenant_id = ?", t.ID).Count(&nsCount)

		result = append(result, gin.H{
			"id":             t.ID,
			"name":           t.Name,
			"display_name":   t.DisplayName,
			"description":    t.Description,
			"max_cpu":        t.MaxCPU,
			"max_memory":     t.MaxMemory,
			"max_gpu":        t.MaxGPU,
			"max_namespaces": t.MaxNamespaces,
			"max_pods":       t.MaxPods,
			"status":         t.Status,
			"member_count":   memberCount,
			"namespace_count": nsCount,
			"created_at":     t.CreatedAt,
		})
	}

	response.Success(c, result)
}

// CreateTenant 创建租户
func (h *Handler) CreateTenant(c *gin.Context) {
	var req struct {
		Name          string `json:"name" binding:"required"`
		DisplayName   string `json:"display_name"`
		Description   string `json:"description"`
		MaxCPU        string `json:"max_cpu"`
		MaxMemory     string `json:"max_memory"`
		MaxGPU        int    `json:"max_gpu"`
		MaxNamespaces int    `json:"max_namespaces"`
		MaxPods       int    `json:"max_pods"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.MaxNamespaces == 0 {
		req.MaxNamespaces = 5
	}
	if req.MaxPods == 0 {
		req.MaxPods = 50
	}

	tenant := model.Tenant{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		MaxCPU:        req.MaxCPU,
		MaxMemory:     req.MaxMemory,
		MaxGPU:        req.MaxGPU,
		MaxNamespaces: req.MaxNamespaces,
		MaxPods:       req.MaxPods,
		Status:        "active",
	}

	if err := h.db.Create(&tenant).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 添加创建者为 owner
	userID, _ := c.Get("user_id")
	member := model.TenantMember{
		TenantID: tenant.ID,
		UserID:   userID.(uint),
		Role:     "owner",
	}
	h.db.Create(&member)

	response.Created(c, tenant)
}

// GetTenant 获取租户详情
func (h *Handler) GetTenant(c *gin.Context) {
	id := c.Param("id")
	var tenant model.Tenant
	if err := h.db.First(&tenant, id).Error; err != nil {
		response.NotFound(c, "tenant not found")
		return
	}

	// 获取成员
	var members []model.TenantMember
	h.db.Where("tenant_id = ?", tenant.ID).Preload("User").Find(&members)

	// 获取命名空间
	var namespaces []model.TenantNamespace
	h.db.Where("tenant_id = ?", tenant.ID).Preload("Cluster").Find(&namespaces)

	response.Success(c, gin.H{
		"tenant":     tenant,
		"members":    members,
		"namespaces": namespaces,
	})
}

// UpdateTenant 更新租户
func (h *Handler) UpdateTenant(c *gin.Context) {
	id := c.Param("id")
	var tenant model.Tenant
	if err := h.db.First(&tenant, id).Error; err != nil {
		response.NotFound(c, "tenant not found")
		return
	}

	var req struct {
		DisplayName   *string `json:"display_name"`
		Description   *string `json:"description"`
		MaxCPU        string  `json:"max_cpu"`
		MaxMemory     string  `json:"max_memory"`
		MaxGPU        *int    `json:"max_gpu"`
		MaxNamespaces *int    `json:"max_namespaces"`
		MaxPods       *int    `json:"max_pods"`
		Status        string  `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.DisplayName != nil {
		tenant.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		tenant.Description = *req.Description
	}
	if req.MaxCPU != "" {
		tenant.MaxCPU = req.MaxCPU
	}
	if req.MaxMemory != "" {
		tenant.MaxMemory = req.MaxMemory
	}
	if req.MaxGPU != nil {
		tenant.MaxGPU = *req.MaxGPU
	}
	if req.MaxNamespaces != nil {
		tenant.MaxNamespaces = *req.MaxNamespaces
	}
	if req.MaxPods != nil {
		tenant.MaxPods = *req.MaxPods
	}
	if req.Status != "" {
		tenant.Status = req.Status
	}

	if err := h.db.Save(&tenant).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, tenant)
}

// DeleteTenant 删除租户
func (h *Handler) DeleteTenant(c *gin.Context) {
	id := c.Param("id")

	// 检查是否有命名空间
	var nsCount int64
	h.db.Model(&model.TenantNamespace{}).Where("tenant_id = ?", id).Count(&nsCount)
	if nsCount > 0 {
		response.BadRequest(c, "cannot delete tenant with existing namespaces")
		return
	}

	// 删除成员
	h.db.Where("tenant_id = ?", id).Delete(&model.TenantMember{})

	// 删除租户
	if err := h.db.Delete(&model.Tenant{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "tenant deleted", nil)
}

// AddTenantMember 添加租户成员
func (h *Handler) AddTenantMember(c *gin.Context) {
	tenantID := c.Param("id")

	var req struct {
		UserID uint   `json:"user_id" binding:"required"`
		Role   string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 检查是否已是成员
	var existing model.TenantMember
	if err := h.db.Where("tenant_id = ? AND user_id = ?", tenantID, req.UserID).First(&existing).Error; err == nil {
		response.BadRequest(c, "user is already a member")
		return
	}

	member := model.TenantMember{
		TenantID: parseUint(tenantID),
		UserID:   req.UserID,
		Role:     req.Role,
	}

	if err := h.db.Create(&member).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, member)
}

// RemoveTenantMember 移除租户成员
func (h *Handler) RemoveTenantMember(c *gin.Context) {
	tenantID := c.Param("id")
	userID := c.Param("userId")

	if err := h.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&model.TenantMember{}).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "member removed", nil)
}

// CreateTenantNamespace 创建租户命名空间
func (h *Handler) CreateTenantNamespace(c *gin.Context) {
	tenantID := c.Param("id")

	var req struct {
		ClusterID uint   `json:"cluster_id" binding:"required"`
		Name      string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 检查租户命名空间数量限制
	var tenant model.Tenant
	if err := h.db.First(&tenant, tenantID).Error; err != nil {
		response.NotFound(c, "tenant not found")
		return
	}

	var nsCount int64
	h.db.Model(&model.TenantNamespace{}).Where("tenant_id = ?", tenantID).Count(&nsCount)
	if int(nsCount) >= tenant.MaxNamespaces {
		response.BadRequest(c, "namespace limit reached")
		return
	}

	// 创建 K8S 命名空间
	client, err := k8s.Manager.GetClient(req.ClusterID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				"kubepilot/tenant": tenant.Name,
			},
		},
	}

	_, err = client.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, "failed to create namespace: "+err.Error())
		return
	}

	// 创建资源配额
	if tenant.MaxCPU != "" || tenant.MaxMemory != "" || tenant.MaxPods > 0 {
		quota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-quota", tenant.Name),
				Namespace: req.Name,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{},
			},
		}
		if tenant.MaxCPU != "" {
			quota.Spec.Hard[corev1.ResourceRequestsCPU] = resource.MustParse(tenant.MaxCPU)
		}
		if tenant.MaxMemory != "" {
			quota.Spec.Hard[corev1.ResourceRequestsMemory] = resource.MustParse(tenant.MaxMemory)
		}
		if tenant.MaxPods > 0 {
			quota.Spec.Hard[corev1.ResourcePods] = resource.MustParse(fmt.Sprintf("%d", tenant.MaxPods))
		}
		client.Clientset.CoreV1().ResourceQuotas(req.Name).Create(ctx, quota, metav1.CreateOptions{})
	}

	// 保存到数据库
	tenantNs := model.TenantNamespace{
		TenantID:  parseUint(tenantID),
		ClusterID: req.ClusterID,
		Name:      req.Name,
	}

	if err := h.db.Create(&tenantNs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, tenantNs)
}

// DeleteTenantNamespace 删除租户命名空间
func (h *Handler) DeleteTenantNamespace(c *gin.Context) {
	tenantID := c.Param("id")
	nsID := c.Param("nsId")

	var ns model.TenantNamespace
	if err := h.db.Where("id = ? AND tenant_id = ?", nsID, tenantID).First(&ns).Error; err != nil {
		response.NotFound(c, "namespace not found")
		return
	}

	// 删除 K8S 命名空间
	client, err := k8s.Manager.GetClient(ns.ClusterID)
	if err == nil {
		ctx := context.Background()
		client.Clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
	}

	// 删除数据库记录
	h.db.Delete(&ns)

	response.SuccessWithMessage(c, "namespace deleted", nil)
}

func parseUint(s string) uint {
	id, _ := strconv.ParseUint(s, 10, 32)
	return uint(id)
}
