package workload

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListNetworkPolicies 获取 NetworkPolicy 列表
func (h *Handler) ListNetworkPolicies(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var policies *networkingv1.NetworkPolicyList
	if namespace == "" {
		policies, err = client.Clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	} else {
		policies, err = client.Clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type PolicyInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Pods      []string `json:"pods"`
		PolicyTypes []string `json:"policy_types"`
		Age       string   `json:"age"`
	}

	result := make([]PolicyInfo, 0, len(policies.Items))
	for _, np := range policies.Items {
		pods := make([]string, 0)
		for k, v := range np.Spec.PodSelector.MatchLabels {
			pods = append(pods, fmt.Sprintf("%s=%s", k, v))
		}

		policyTypes := make([]string, 0)
		for _, pt := range np.Spec.PolicyTypes {
			policyTypes = append(policyTypes, string(pt))
		}

		result = append(result, PolicyInfo{
			Name:        np.Name,
			Namespace:   np.Namespace,
			Pods:        pods,
			PolicyTypes: policyTypes,
			Age:         timeSince(np.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// GetNetworkPolicy 获取 NetworkPolicy 详情
func (h *Handler) GetNetworkPolicy(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	np, err := client.Clientset.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "network policy not found")
		return
	}

	response.Success(c, np)
}

// CreateNetworkPolicy 创建 NetworkPolicy
func (h *Handler) CreateNetworkPolicy(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Name      string `json:"name" binding:"required"`
		Namespace string `json:"namespace" binding:"required"`
		PodSelector map[string]string `json:"pod_selector"`
		PolicyTypes []string `json:"policy_types"`
		Ingress     []networkingv1.NetworkPolicyIngressRule `json:"ingress"`
		Egress      []networkingv1.NetworkPolicyEgressRule  `json:"egress"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: req.PodSelector,
			},
			Ingress: req.Ingress,
			Egress:  req.Egress,
		},
	}

	// 设置 PolicyTypes
	for _, pt := range req.PolicyTypes {
		np.Spec.PolicyTypes = append(np.Spec.PolicyTypes, networkingv1.PolicyType(pt))
	}

	ctx := context.Background()
	result, err := client.Clientset.NetworkingV1().NetworkPolicies(req.Namespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// DeleteNetworkPolicy 删除 NetworkPolicy
func (h *Handler) DeleteNetworkPolicy(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "network policy deleted", nil)
}
