package auth

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/pkg/wsticket"
)

type WebSocketTicketHandler struct {
	manager *wsticket.Manager
}

func NewWebSocketTicketHandler(manager *wsticket.Manager) *WebSocketTicketHandler {
	return &WebSocketTicketHandler{manager: manager}
}

func (h *WebSocketTicketHandler) IssuePod(c *gin.Context) {
	h.issue(c, "pod", c.Param("ns"), c.Param("name"))
}

func (h *WebSocketTicketHandler) IssueNode(c *gin.Context) {
	h.issue(c, "node", "", c.Param("name"))
}

func (h *WebSocketTicketHandler) issue(c *gin.Context, kind, namespace, resourceName string) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid cluster id"})
		return
	}
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing user context"})
		return
	}

	ticket, ttl, err := h.manager.Issue(c.Request.Context(), wsticket.Claims{
		UserID:       userID.(uint),
		Kind:         kind,
		ClusterID:    uint(clusterID),
		Namespace:    namespace,
		ResourceName: resourceName,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to issue websocket ticket"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"ticket":     ticket,
			"expires_in": int(ttl.Seconds()),
		},
	})
}
