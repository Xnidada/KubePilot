package workload

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

// ListCustomResources 获取自定义资源列表
func (h *Handler) ListCustomResources(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	restClient := client.Clientset.Discovery().RESTClient()
	var result []byte

	if namespace != "" {
		result, err = restClient.Get().
			AbsPath("/apis", group, version, "namespaces", namespace, resource).
			DoRaw(context.Background())
	} else {
		result, err = restClient.Get().
			AbsPath("/apis", group, version, resource).
			DoRaw(context.Background())
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal(result, &listResult); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, listResult)
}

// GetCustomResource 获取自定义资源详情
func (h *Handler) GetCustomResource(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	name := c.Param("name")
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	restClient := client.Clientset.Discovery().RESTClient()
	var result []byte

	if namespace != "" {
		result, err = restClient.Get().
			AbsPath("/apis", group, version, "namespaces", namespace, resource, name).
			DoRaw(context.Background())
	} else {
		result, err = restClient.Get().
			AbsPath("/apis", group, version, resource, name).
			DoRaw(context.Background())
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var resourceResult map[string]interface{}
	if err := json.Unmarshal(result, &resourceResult); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, resourceResult)
}

// CreateCustomResource 创建自定义资源
func (h *Handler) CreateCustomResource(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取命名空间
	namespace := ""
	if ns, ok := req["metadata"].(map[string]interface{}); ok {
		if nsVal, ok := ns["namespace"].(string); ok {
			namespace = nsVal
		}
	}

	restClient := client.Clientset.Discovery().RESTClient()
	body, _ := json.Marshal(req)

	var result []byte
	if namespace != "" {
		result, err = restClient.Post().
			AbsPath("/apis", group, version, "namespaces", namespace, resource).
			Body(body).
			DoRaw(context.Background())
	} else {
		result, err = restClient.Post().
			AbsPath("/apis", group, version, resource).
			Body(body).
			DoRaw(context.Background())
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var created map[string]interface{}
	if err := json.Unmarshal(result, &created); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, created)
}

// UpdateCustomResource 更新自定义资源
func (h *Handler) UpdateCustomResource(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	name := c.Param("name")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 获取命名空间
	namespace := ""
	if ns, ok := req["metadata"].(map[string]interface{}); ok {
		if nsVal, ok := ns["namespace"].(string); ok {
			namespace = nsVal
		}
	}

	restClient := client.Clientset.Discovery().RESTClient()
	body, _ := json.Marshal(req)

	var result []byte
	if namespace != "" {
		result, err = restClient.Put().
			AbsPath("/apis", group, version, "namespaces", namespace, resource, name).
			Body(body).
			DoRaw(context.Background())
	} else {
		result, err = restClient.Put().
			AbsPath("/apis", group, version, resource, name).
			Body(body).
			DoRaw(context.Background())
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var updated map[string]interface{}
	if err := json.Unmarshal(result, &updated); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, updated)
}

// DeleteCustomResource 删除自定义资源
func (h *Handler) DeleteCustomResource(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	name := c.Param("name")
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	restClient := client.Clientset.Discovery().RESTClient()

	if namespace != "" {
		_, err = restClient.Delete().
			AbsPath("/apis", group, version, "namespaces", namespace, resource, name).
			DoRaw(context.Background())
	} else {
		_, err = restClient.Delete().
			AbsPath("/apis", group, version, resource, name).
			DoRaw(context.Background())
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "resource deleted", nil)
}
