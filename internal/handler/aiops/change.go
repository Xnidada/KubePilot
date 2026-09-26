package aiops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	aiopsService "github.com/kubepilot/kubepilot/internal/service/aiops"
	"gorm.io/gorm"
)

func (h *Handler) productionAction(clusterID uint) (bool, error) {
	var cluster model.Cluster
	if err := h.db.Select("id", "environment").First(&cluster, clusterID).Error; err != nil {
		return false, err
	}
	return cluster.Environment == "" || cluster.Environment == "production", nil
}

func (h *Handler) actionEvidence(action model.AgentAction) string {
	if action.ConversationID == nil {
		return `{"source":"manual"}`
	}
	var traces []model.AgentToolTrace
	if err := h.db.Where("user_id = ? AND cluster_id = ? AND conversation_id = ?", action.UserID, action.ClusterID, *action.ConversationID).Order("id DESC").Limit(20).Find(&traces).Error; err != nil {
		return `{"source":"agent","trace":"unavailable"}`
	}
	var payload struct {
		PendingIDs []uint `json:"pending_ids"`
		Tools      []struct {
			Name    string `json:"name"`
			Args    string `json:"args"`
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
		} `json:"tools"`
	}
	var traceID uint
	for _, trace := range traces {
		payload.PendingIDs = nil
		payload.Tools = nil
		if json.Unmarshal([]byte(trace.Payload), &payload) != nil {
			continue
		}
		for _, id := range payload.PendingIDs {
			if id == action.ID {
				traceID = trace.ID
				break
			}
		}
		if traceID != 0 {
			break
		}
	}
	if traceID == 0 {
		return `{"source":"agent","trace":"unavailable"}`
	}
	type evidenceTool struct {
		Name         string `json:"name"`
		ArgsSHA256   string `json:"args_sha256"`
		ResultSHA256 string `json:"result_sha256"`
	}
	tools := make([]evidenceTool, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		if tool.IsError || !diagnosticTool(tool.Name) {
			continue
		}
		sum := sha256.Sum256([]byte(tool.Result))
		argsSum := sha256.Sum256([]byte(tool.Args))
		tools = append(tools, evidenceTool{tool.Name, hex.EncodeToString(argsSum[:]), hex.EncodeToString(sum[:])})
	}
	b, _ := json.Marshal(gin.H{"source": "agent", "trace_id": traceID, "tools": tools})
	return string(b)
}

func diagnosticTool(name string) bool {
	switch name {
	case "list_resources", "get_resource", "get_events", "get_pod_logs", "describe_resource", "diagnose_workload", "diagnose_service":
		return true
	default:
		return false
	}
}

func (h *Handler) submitActionForApproval(action model.AgentAction, actorID uint) error {
	var params aiopsService.StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return fmt.Errorf("invalid staged change: %w", err)
	}
	if !aiopsService.ProductionAutoRollbackSupported(params.Action) {
		return fmt.Errorf("production automation for %s has no safe automatic rollback; use a separate change runbook", params.Action)
	}
	if len(params.HostPathMounts) > 0 || len(params.EnvVars) > 0 {
		return fmt.Errorf("production automation does not allow hostPath mounts or environment value changes")
	}
	evidence := h.actionEvidence(action)
	if action.ConversationID != nil {
		var data struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if json.Unmarshal([]byte(evidence), &data) != nil || len(data.Tools) == 0 {
			return fmt.Errorf("agent must collect live diagnostic evidence before submitting a production change")
		}
	}
	return h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentAction{}).Where("id = ? AND status = ? AND user_id = ?", action.ID, "pending", actorID).
			Updates(map[string]any{"status": "approval_pending", "evidence": evidence})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("action is no longer pending")
		}
		return tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: actorID, Event: "submitted", Detail: "production change submitted for independent approval"}).Error
	})
}

func (h *Handler) AgentListChanges(c *gin.Context) {
	userID := c.GetUint("user_id")
	var own []model.AgentAction
	if err := h.db.Where("user_id = ?", userID).Order("id DESC").Limit(100).Find(&own).Error; err != nil {
		response.InternalError(c, "failed to list changes")
		return
	}
	var pending []model.AgentAction
	if err := h.db.Where("status = ? AND user_id <> ?", "approval_pending", userID).Order("id DESC").Limit(100).Find(&pending).Error; err != nil {
		response.InternalError(c, "failed to list approvals")
		return
	}
	approved := make([]gin.H, 0, len(pending))
	for _, action := range pending {
		if h.canReviewChange(c, action) {
			approved = append(approved, publicChange(action))
		}
	}
	myChanges := make([]gin.H, 0, len(own))
	for _, action := range own {
		myChanges = append(myChanges, publicChange(action))
	}
	response.Success(c, gin.H{"mine": myChanges, "awaiting_my_approval": approved})
}

func (h *Handler) canReviewChange(c *gin.Context, action model.AgentAction) bool {
	userID := c.GetUint("user_id")
	if userID == 0 || userID == action.UserID {
		return false
	}
	var role model.Role
	if err := h.db.First(&role, c.GetUint("role_id")).Error; err != nil {
		return false
	}
	if role.IsSystem || role.Name == "admin" {
		return true
	}
	ns := action.Namespace
	if ns == "" {
		ns = "*"
	}
	ok, err := authz.NewGORMGrantResolver(h.db).Authorize(c.Request.Context(), userID, action.ClusterID, ns, "write")
	return err == nil && ok
}

func (h *Handler) AgentDecideChange(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("actionId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid action id")
		return
	}
	var action model.AgentAction
	if err := h.db.First(&action, id).Error; err != nil {
		response.NotFound(c, "action not found")
		return
	}
	if action.Status != "approval_pending" {
		response.BadRequest(c, "action is not awaiting approval")
		return
	}
	if !h.canReviewChange(c, action) {
		response.Forbidden(c, "independent approver with cluster write access required")
		return
	}
	var req struct {
		Decision string `json:"decision" binding:"required,oneof=approve reject"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid decision")
		return
	}
	status, event := "approved", "approved"
	if req.Decision == "reject" {
		status, event = "rejected", "rejected"
	}
	actorID := c.GetUint("user_id")
	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": status}
		if status == "approved" {
			updates["approved_by"] = actorID
			updates["approved_at"] = now
		}
		result := tx.Model(&model.AgentAction{}).Where("id = ? AND status = ?", action.ID, "approval_pending").Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("action was already decided")
		}
		return tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: actorID, Event: event, Detail: req.Reason}).Error
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"action_id": action.ID, "status": status})
}

func (h *Handler) AgentCancelChange(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("actionId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid action id")
		return
	}
	actorID := c.GetUint("user_id")
	err = h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentAction{}).
			Where("id = ? AND user_id = ? AND status IN ?", id, actorID, []string{"pending", "approval_pending", "approved"}).
			Update("status", "cancelled")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("change is not cancellable or not owned by requester")
		}
		return tx.Create(&model.AgentActionAudit{ActionID: uint(id), ActorID: actorID, Event: "cancelled", Detail: "requester cancelled staged change"}).Error
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"action_id": id, "status": "cancelled"})
}

func (h *Handler) AgentExportChange(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("actionId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid action id")
		return
	}
	var action model.AgentAction
	if err := h.db.First(&action, id).Error; err != nil {
		response.NotFound(c, "action not found")
		return
	}
	actorID := c.GetUint("user_id")
	if actorID != action.UserID && !h.canReviewChange(c, action) && (action.ApprovedBy == nil || *action.ApprovedBy != actorID) {
		response.Forbidden(c, "change not visible")
		return
	}
	var audits []model.AgentActionAudit
	if err := h.db.Where("action_id = ?", action.ID).Order("id ASC").Find(&audits).Error; err != nil {
		response.InternalError(c, "failed to read audit")
		return
	}
	sum := sha256.Sum256([]byte(action.Parameters))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=kubepilot-change-%d.json", action.ID))
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"change": publicChange(action), "parameters_sha256": hex.EncodeToString(sum[:]), "audit": audits})
}

func publicChange(a model.AgentAction) gin.H {
	return gin.H{
		"id": a.ID, "user_id": a.UserID, "cluster_id": a.ClusterID, "conversation_id": a.ConversationID,
		"action": stagedActionKind(a), "resource_name": a.ResourceName, "namespace": a.Namespace,
		"status": a.Status, "dry_run": a.DryRunResult, "evidence": a.Evidence,
		"resource_uid": a.ResourceUID, "base_generation": a.BaseGeneration,
		"result": a.Result, "observation": a.Observation, "rollback_result": a.RollbackResult,
		"approved_by": a.ApprovedBy, "approved_at": a.ApprovedAt,
		"created_at": a.CreatedAt, "executed_at": a.ExecutedAt,
	}
}

func stagedActionKind(action model.AgentAction) string {
	var params struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(action.Parameters), &params) == nil && params.Action != "" {
		return params.Action
	}
	return action.ResourceType
}

func (h *Handler) recordActionOutcome(action *model.AgentAction, actorID uint, event, detail string) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(action).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: actorID, Event: event, Detail: detail}).Error; err != nil {
			return err
		}
		if action.Observation != "" {
			if err := tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: actorID, Event: "observed", Detail: action.Observation}).Error; err != nil {
				return err
			}
		}
		if action.RollbackResult != "" {
			if err := tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: actorID, Event: "rollback", Detail: action.RollbackResult}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
