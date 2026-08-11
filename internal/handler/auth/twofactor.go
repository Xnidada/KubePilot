package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	authService "github.com/kubepilot/kubepilot/internal/service/auth"
	"gorm.io/gorm"
)

// TwoFactorHandler 两步验证处理器
type TwoFactorHandler struct {
	db          *gorm.DB
	authService *authService.Service
	cache       cache.Cache
}

// NewTwoFactorHandler 创建两步验证处理器
func NewTwoFactorHandler(db *gorm.DB, authSvc *authService.Service, cacheInstance cache.Cache) *TwoFactorHandler {
	return &TwoFactorHandler{db: db, authService: authSvc, cache: cacheInstance}
}

// SetupRequest 初始化两步验证请求
type SetupResponse struct {
	Secret    string   `json:"secret"`
	QRCodeURL string   `json:"qr_code_url"`
	BackupCodes []string `json:"backup_codes"`
}

// Setup 初始化两步验证设置
func (h *TwoFactorHandler) Setup(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid := userID.(uint)

	// 检查是否已配置
	var existing model.UserTwoFactor
	if err := h.db.Where("user_id = ?", uid).First(&existing).Error; err == nil {
		if existing.IsEnabled {
			response.BadRequest(c, "两步验证已启用")
			return
		}
	}

	// 生成 TOTP secret
	secret := generateSecret()

	// 生成备份码
	backupCodes := generateBackupCodes()
	backupCodesJSON, _ := json.Marshal(backupCodes)

	// 获取用户名用于 QR code
	var user model.User
	h.db.First(&user, uid)

	// 生成 QR code URL (otpauth://totp/...)
	qrCodeURL := fmt.Sprintf("otpauth://totp/KubePilot:%s?secret=%s&issuer=KubePilot",
		user.Username, secret)

	// 保存或更新配置
	tf := model.UserTwoFactor{
		UserID:      uid,
		Secret:      secret,
		IsEnabled:   false,
		BackupCodes: string(backupCodesJSON),
	}

	h.db.Where("user_id = ?", uid).Assign(tf).FirstOrCreate(&tf)

	response.Success(c, SetupResponse{
		Secret:      secret,
		QRCodeURL:   qrCodeURL,
		BackupCodes: backupCodes,
	})
}

// VerifyRequest 验证请求
type VerifyRequest struct {
	Code string `json:"code" binding:"required"`
}

// VerifyAndEnable 验证并启用两步验证
func (h *TwoFactorHandler) VerifyAndEnable(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid := userID.(uint)

	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var tf model.UserTwoFactor
	if err := h.db.Where("user_id = ?", uid).First(&tf).Error; err != nil {
		response.NotFound(c, "请先初始化两步验证")
		return
	}

	// 验证 TOTP code
	if !validateTOTP(tf.Secret, req.Code) {
		response.BadRequest(c, "验证码错误")
		return
	}

	// 启用两步验证
	tf.IsEnabled = true
	h.db.Save(&tf)

	response.SuccessWithMessage(c, "两步验证已启用", nil)
}

// Disable 禁用两步验证
func (h *TwoFactorHandler) Disable(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid := userID.(uint)

	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var tf model.UserTwoFactor
	if err := h.db.Where("user_id = ?", uid).First(&tf).Error; err != nil {
		response.NotFound(c, "两步验证未配置")
		return
	}

	// 验证当前 TOTP code
	if !validateTOTP(tf.Secret, req.Code) {
		response.BadRequest(c, "验证码错误")
		return
	}

	// 禁用
	tf.IsEnabled = false
	h.db.Save(&tf)

	response.SuccessWithMessage(c, "两步验证已禁用", nil)
}

// Status 获取两步验证状态
func (h *TwoFactorHandler) Status(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid := userID.(uint)

	var tf model.UserTwoFactor
	err := h.db.Where("user_id = ?", uid).First(&tf).Error

	response.Success(c, gin.H{
		"configured": err == nil,
		"enabled":    err == nil && tf.IsEnabled,
	})
}

// LoginVerify 登录时验证两步验证码（必须携带账密阶段下发的 pending_token）
func (h *TwoFactorHandler) LoginVerify(c *gin.Context) {
	var req struct {
		PendingToken string `json:"pending_token" binding:"required"`
		Code         string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if h.cache == nil {
		response.InternalError(c, "cache not configured for 2FA login")
		return
	}

	key := "2fa:pending:" + strings.TrimSpace(req.PendingToken)
	rawUID, err := h.cache.Take(c.Request.Context(), key)
	if err != nil || strings.TrimSpace(rawUID) == "" {
		response.Unauthorized(c, "2FA session expired or invalid; please login again")
		return
	}
	uid64, err := strconv.ParseUint(rawUID, 10, 64)
	if err != nil || uid64 == 0 {
		response.Unauthorized(c, "2FA session invalid")
		return
	}
	userID := uint(uid64)

	var tf model.UserTwoFactor
	if err := h.db.Where("user_id = ? AND is_enabled = ?", userID, true).First(&tf).Error; err != nil {
		response.NotFound(c, "两步验证未启用")
		return
	}

	verified := false
	backupUsed := false

	// 先尝试 TOTP code
	if validateTOTP(tf.Secret, req.Code) {
		verified = true
	} else if validateBackupCode(&tf, req.Code) {
		// 尝试备份码
		verified = true
		backupUsed = true
	}

	if !verified {
		// 验证失败后需重新登录拿新的 pending_token（Take 已消费）
		response.BadRequest(c, "验证码错误，请重新登录后再试")
		return
	}

	// 更新最后使用时间
	now := time.Now()
	tf.LastUsedAt = &now
	h.db.Save(&tf)

	if h.authService == nil {
		response.InternalError(c, "auth service not configured for 2FA login")
		return
	}
	result, err := h.authService.GenerateTokenForUser(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"verified":         true,
		"backup_code_used": backupUsed,
		"token":            result.Token,
		"expires_at":       result.ExpiresAt,
		"user":             result.User,
	})
}

// ==================== TOTP 算法实现 ====================

// generateSecret 生成 TOTP secret
func generateSecret() string {
	// 生成 20 字节随机数据
	secret := make([]byte, 20)
	rand.Read(secret)
	// 使用 base32 编码
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// generateBackupCodes 生成备份码
func generateBackupCodes() []string {
	codes := make([]string, 8)
	for i := 0; i < 8; i++ {
		b := make([]byte, 4)
		rand.Read(b)
		codes[i] = fmt.Sprintf("%08x", b)
	}
	return codes
}

// validateTOTP 验证 TOTP code
func validateTOTP(secret, code string) bool {
	// 解码 secret
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	// 获取当前时间戳（30秒间隔）
	now := time.Now().Unix()
	timeStep := now / 30

	// 验证当前时间窗口和前后各1个窗口（允许时钟偏差）
	for i := int64(-1); i <= 1; i++ {
		if generateTOTP(secretBytes, timeStep+i) == code {
			return true
		}
	}

	return false
}

// generateTOTP 生成 TOTP code
func generateTOTP(secret []byte, timeCounter int64) string {
	// 将时间计数器转为 8 字节大端序
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeCounter))

	// HMAC-SHA1
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// 动态截断
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	// 取后6位
	return fmt.Sprintf("%06d", code%1000000)
}

// validateBackupCode 验证备份码
func validateBackupCode(tf *model.UserTwoFactor, code string) bool {
	var codes []string
	if err := json.Unmarshal([]byte(tf.BackupCodes), &codes); err != nil {
		return false
	}

	code = strings.ToLower(strings.TrimSpace(code))
	for i, c := range codes {
		if strings.ToLower(c) == code {
			// 移除已使用的备份码
			codes = append(codes[:i], codes[i+1:]...)
			newJSON, _ := json.Marshal(codes)
			tf.BackupCodes = string(newJSON)
			return true
		}
	}

	return false
}

// CheckTwoFactorRequired 检查用户是否需要两步验证
func CheckTwoFactorRequired(db *gorm.DB, userID uint) bool {
	var tf model.UserTwoFactor
	if err := db.Where("user_id = ? AND is_enabled = ?", userID, true).First(&tf).Error; err != nil {
		return false
	}
	return true
}
