package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/service/auth"
	"gorm.io/gorm"
)

type Handler struct {
	service *auth.Service
	db      *gorm.DB
	cache   cache.Cache
}

func NewHandler(service *auth.Service, db *gorm.DB, cacheInstance ...cache.Cache) *Handler {
	h := &Handler{service: service, db: db}
	if len(cacheInstance) > 0 {
		h.cache = cacheInstance[0]
	}
	return h
}

const twoFAPendingTTL = 5 * time.Minute

func (h *Handler) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	user, err := h.service.Authenticate(req.Username, req.Password)
	if err != nil {
		// 记录登录失败
		h.recordLoginLog(0, req.Username, c.ClientIP(), c.Request.UserAgent(), false)
		response.Unauthorized(c, err.Error())
		return
	}

	// 记录登录成功
	h.recordLoginLog(user.ID, user.Username, c.ClientIP(), c.Request.UserAgent(), true)

	// 检查是否需要两步验证（验证通过后再发 JWT）
	if CheckTwoFactorRequired(h.db, user.ID) {
		pending, err := h.issueTwoFAPending(c.Request.Context(), user.ID)
		if err != nil {
			response.InternalError(c, "failed to create 2FA session")
			return
		}
		response.Success(c, gin.H{
			"require_2fa":   true,
			"pending_token": pending,
			"message":       "需要两步验证",
		})
		return
	}

	result, err := h.service.GenerateTokenForUser(user.ID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// recordLoginLog 异步写入登入日志
func (h *Handler) recordLoginLog(userID uint, username, ip, userAgent string, success bool) {
	if h.db == nil {
		return
	}
	log := model.LoginLog{
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		CreatedAt: time.Now(),
	}
	go func() {
		_ = h.db.Create(&log).Error
	}()
}

func (h *Handler) issueTwoFAPending(ctx context.Context, userID uint) (string, error) {
	if h.cache == nil {
		return "", fmt.Errorf("cache not configured")
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	key := "2fa:pending:" + token
	if err := h.cache.Set(ctx, key, strconv.FormatUint(uint64(userID), 10), twoFAPendingTTL); err != nil {
		return "", err
	}
	return token, nil
}

func (h *Handler) Register(c *gin.Context) {
	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.Register(&req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Created(c, result)
}

func (h *Handler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")

	result, err := h.service.GetUserByID(userID.(uint))
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, result)
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if err := h.service.ChangePassword(userID.(uint), req.OldPassword, req.NewPassword); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "password changed successfully", nil)
}
