package workload

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostHandler 成本分析处理器
type CostHandler struct {
	db *gorm.DB
}

// NewCostHandler 创建成本分析处理器
func NewCostHandler(db *gorm.DB) *CostHandler {
	return &CostHandler{db: db}
}

// GetCostConfig 获取成本配置
func (h *CostHandler) GetCostConfig(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var config model.CostConfig
	err = h.db.Where("cluster_id = ?", clusterID).First(&config).Error
	if err != nil {
		// 返回默认配置
		response.Success(c, gin.H{
			"cpu_per_unit": 0.032,
			"mem_per_unit": 0.004,
			"gpu_per_unit": 1.5,
			"currency":     "USD",
		})
		return
	}

	response.Success(c, config)
}

// SaveCostConfig 保存成本配置
func (h *CostHandler) SaveCostConfig(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		CPUPerUnit float64 `json:"cpu_per_unit"`
		MemPerUnit float64 `json:"mem_per_unit"`
		GPUPerUnit float64 `json:"gpu_per_unit"`
		Currency   string  `json:"currency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var config model.CostConfig
	err = h.db.Where("cluster_id = ?", clusterID).First(&config).Error
	if err != nil {
		// 创建新配置
		config = model.CostConfig{
			ClusterID:  uint(clusterID),
			CPUPerUnit: req.CPUPerUnit,
			MemPerUnit: req.MemPerUnit,
			GPUPerUnit: req.GPUPerUnit,
			Currency:   req.Currency,
		}
		if err := h.db.Create(&config).Error; err != nil {
			response.InternalError(c, err.Error())
			return
		}
	} else {
		// 更新配置
		config.CPUPerUnit = req.CPUPerUnit
		config.MemPerUnit = req.MemPerUnit
		config.GPUPerUnit = req.GPUPerUnit
		config.Currency = req.Currency
		if err := h.db.Save(&config).Error; err != nil {
			response.InternalError(c, err.Error())
			return
		}
	}

	response.Success(c, config)
}

// GetResourceCost 获取资源成本分析
func (h *CostHandler) GetResourceCost(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	// 获取成本配置
	var config model.CostConfig
	err = h.db.Where("cluster_id = ?", clusterID).First(&config).Error
	if err != nil {
		// 使用默认值
		config = model.CostConfig{
			CPUPerUnit: 0.032,
			MemPerUnit: 0.004,
			GPUPerUnit: 1.5,
			Currency:   "USD",
		}
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取所有命名空间
	namespaces, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type NamespaceCost struct {
		Namespace    string  `json:"namespace"`
		CPURequest   int64   `json:"cpu_request"`   // mCPU
		MemoryRequest int64  `json:"memory_request"` // MB
		PodCount     int     `json:"pod_count"`
		CPUCost      float64 `json:"cpu_cost"`
		MemoryCost   float64 `json:"memory_cost"`
		TotalCost    float64 `json:"total_cost"`
	}

	var costs []NamespaceCost
	totalCost := 0.0

	for _, ns := range namespaces.Items {
		// 跳过系统命名空间
		if ns.Name == "kube-system" || ns.Name == "kube-public" || ns.Name == "kube-node-lease" {
			continue
		}

		// 获取命名空间下的 Pods
		pods, err := client.Clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		var cpuTotal, memTotal int64
		podCount := len(pods.Items)

		for _, pod := range pods.Items {
			for _, container := range pod.Spec.Containers {
				if container.Resources.Requests != nil {
					if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
						cpuTotal += cpu.MilliValue()
					}
					if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
						memTotal += mem.Value() / (1024 * 1024) // 转换为 MB
					}
				}
			}
		}

		// 计算成本（按月估算，假设 730 小时/月）
		hoursPerMonth := 730.0
		cpuCost := float64(cpuTotal) / 1000.0 * config.CPUPerUnit * hoursPerMonth
		memCost := float64(memTotal) * config.MemPerUnit * hoursPerMonth
		total := cpuCost + memCost

		costs = append(costs, NamespaceCost{
			Namespace:     ns.Name,
			CPURequest:    cpuTotal,
			MemoryRequest: memTotal,
			PodCount:      podCount,
			CPUCost:       cpuCost,
			MemoryCost:    memCost,
			TotalCost:     total,
		})

		totalCost += total
	}

	response.Success(c, gin.H{
		"namespaces": costs,
		"total_cost": totalCost,
		"currency":   config.Currency,
		"config":     config,
	})
}
