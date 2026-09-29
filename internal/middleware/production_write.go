package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

const productionWriteMessage = "生产集群已开启双人审批；直接写入已禁用，请使用 Agent 暂存、预览与审批流程"

// ProductionWriteGate blocks direct Kubernetes mutations while the optional
// production two-person gate is enabled. Agent confirmation has its own gate.
func ProductionWriteGate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !directClusterMutation(c.Request.Method, c.FullPath()) {
			c.Next()
			return
		}
		clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil || clusterID == 0 {
			response.BadRequest(c, "invalid cluster id")
			c.Abort()
			return
		}
		if !AllowProductionWrite(c, db, uint(clusterID)) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// AllowProductionWrite also guards routes whose cluster ID comes from a body
// or a stored task, where the path middleware cannot resolve the target.
func AllowProductionWrite(c *gin.Context, db *gorm.DB, clusterID uint) bool {
	blocked, err := model.ProductionWriteBlocked(db, clusterID)
	if err != nil {
		response.InternalError(c, "failed to check production write policy")
		return false
	}
	if blocked {
		response.Error(c, http.StatusConflict, productionWriteMessage)
		return false
	}
	return true
}

func directClusterMutation(method, path string) bool {
	if strings.HasPrefix(path, "/api/v1/ws/terminal/") || strings.HasPrefix(path, "/api/v1/ws/node-shell/") ||
		strings.HasPrefix(path, "/api/v1/ws/tickets/pod/") || strings.HasPrefix(path, "/api/v1/ws/tickets/node/") {
		return true // interactive shells can mutate the cluster even over GET
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return strings.HasPrefix(path, "/api/v1/clusters/:id/workloads/") || strings.HasPrefix(path, "/api/v1/ops/:id/")
}
