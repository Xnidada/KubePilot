package router

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/service/auth"
	"gorm.io/gorm"
)

// OAuthHandler OAuth 处理器
type OAuthHandler struct {
	db         *gorm.DB
	authSvc    *auth.Service
	cache      cache.Cache
	httpClient *http.Client
}

func NewOAuthHandler(db *gorm.DB, authSvc *auth.Service, cacheInstance cache.Cache) *OAuthHandler {
	return &OAuthHandler{
		db:      db,
		authSvc: authSvc,
		cache:   cacheInstance,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListProviders 获取 OAuth 提供商列表
func (h *OAuthHandler) ListProviders(c *gin.Context) {
	var configs []model.OAuthConfig
	if err := h.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 隐藏敏感信息
	result := make([]gin.H, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, gin.H{
			"id":       cfg.ID,
			"provider": cfg.Provider,
			"name":     cfg.Name,
			"enabled":  cfg.Enabled,
		})
	}

	response.Success(c, result)
}

// Login 发起 OAuth 登录
func (h *OAuthHandler) Login(c *gin.Context) {
	provider := c.Param("provider")

	var config model.OAuthConfig
	if err := h.db.Where("provider = ? AND enabled = ?", provider, true).First(&config).Error; err != nil {
		response.NotFound(c, "OAuth provider not found")
		return
	}

	// 生成 state 参数防止 CSRF
	state := generateRandomState()

	// 将 state 存储到缓存，5分钟过期
	ctx := context.Background()
	stateKey := fmt.Sprintf("oauth:state:%s", state)
	h.cache.Set(ctx, stateKey, provider, 5*time.Minute)

	// 构建授权 URL
	var authURL string
	switch provider {
	case "github":
		authURL = fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			config.AuthURL, config.ClientID, config.RedirectURL, config.Scopes, state)
	case "gitlab":
		authURL = fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&response_type=code&state=%s",
			config.AuthURL, config.ClientID, config.RedirectURL, config.Scopes, state)
	case "google":
		authURL = fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&response_type=code&state=%s&access_type=offline",
			config.AuthURL, config.ClientID, config.RedirectURL, config.Scopes, state)
	default:
		authURL = fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			config.AuthURL, config.ClientID, config.RedirectURL, config.Scopes, state)
	}

	response.Success(c, gin.H{
		"auth_url": authURL,
		"state":    state,
	})
}

// Callback OAuth 回调
func (h *OAuthHandler) Callback(c *gin.Context) {
	provider := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		response.BadRequest(c, "missing code parameter")
		return
	}

	// 验证 state 参数
	ctx := context.Background()
	stateKey := fmt.Sprintf("oauth:state:%s", state)
	savedProvider, err := h.cache.Get(ctx, stateKey)
	if err != nil || savedProvider != provider {
		response.BadRequest(c, "invalid or expired state parameter")
		return
	}
	// 删除已使用的 state
	h.cache.Delete(ctx, stateKey)

	var config model.OAuthConfig
	if err := h.db.Where("provider = ? AND enabled = ?", provider, true).First(&config).Error; err != nil {
		response.NotFound(c, "OAuth provider not found")
		return
	}

	// 获取 access token
	accessToken, err := h.exchangeCode(&config, code)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf("failed to exchange code: %v", err))
		return
	}

	// 获取用户信息
	userInfo, err := h.getUserInfo(&config, accessToken)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf("failed to get user info: %v", err))
		return
	}

	// 查找或创建用户
	user, err := h.findOrCreateUser(&config, userInfo)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf("failed to create user: %v", err))
		return
	}

	// 生成 JWT token
	token, err := h.authSvc.GenerateTokenForUser(user.ID)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	response.Success(c, token)
}

// exchangeCode 交换授权码获取 access token
func (h *OAuthHandler) exchangeCode(config *model.OAuthConfig, code string) (string, error) {
	data := map[string]string{
		"client_id":     config.ClientID,
		"client_secret": config.ClientSecret,
		"code":          code,
		"redirect_uri":  config.RedirectURL,
		"grant_type":    "authorization_code",
	}

	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", config.TokenURL, strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("invalid token response: %s", string(body))
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s", tokenResp.Error)
	}

	return tokenResp.AccessToken, nil
}

// OAuthUserInfo OAuth 用户信息
type OAuthUserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
	Name     string `json:"name"`
}

// getUserInfo 获取 OAuth 用户信息
func (h *OAuthHandler) getUserInfo(config *model.OAuthConfig, accessToken string) (*OAuthUserInfo, error) {
	req, _ := http.NewRequest("GET", config.UserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var userInfo OAuthUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("invalid user info response: %s", string(body))
	}

	return &userInfo, nil
}

// findOrCreateUser 查找或创建用户
func (h *OAuthHandler) findOrCreateUser(config *model.OAuthConfig, userInfo *OAuthUserInfo) (*model.User, error) {
	// 查找已关联的 OAuth 用户
	var oauthUser model.OAuthUser
	err := h.db.Where("provider = ? AND external_id = ?", config.Provider, userInfo.ID).First(&oauthUser).Error
	if err == nil {
		// 已存在，直接返回用户
		var user model.User
		if err := h.db.First(&user, oauthUser.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}

	// 查找是否有相同邮箱的用户
	var existingUser model.User
	err = h.db.Where("email = ?", userInfo.Email).First(&existingUser).Error
	if err == nil {
		// 关联到现有用户
		oauthUser = model.OAuthUser{
			UserID:     existingUser.ID,
			Provider:   config.Provider,
			ExternalID: userInfo.ID,
			Username:   userInfo.Username,
			Email:      userInfo.Email,
			Avatar:     userInfo.Avatar,
		}
		h.db.Create(&oauthUser)
		return &existingUser, nil
	}

	// 创建新用户
	username := userInfo.Username
	if username == "" {
		username = userInfo.Email
	}
	if username == "" {
		username = fmt.Sprintf("%s_%s", config.Provider, userInfo.ID)
	}

	// 生成随机密码
	randomPassword := generateRandomState()
	hashedPassword, _ := crypto.HashPassword(randomPassword)

	user := &model.User{
		Username: username,
		Email:    userInfo.Email,
		Password: hashedPassword,
		RealName: userInfo.Name,
		Avatar:   userInfo.Avatar,
		Status:   1,
		RoleID:   config.DefaultRole,
	}

	if err := h.db.Create(user).Error; err != nil {
		return nil, err
	}

	// 创建 OAuth 用户关联
	oauthUser = model.OAuthUser{
		UserID:     user.ID,
		Provider:   config.Provider,
		ExternalID: userInfo.ID,
		Username:   userInfo.Username,
		Email:      userInfo.Email,
		Avatar:     userInfo.Avatar,
	}
	h.db.Create(&oauthUser)

	return user, nil
}

// generateRandomState 生成随机 state
func generateRandomState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
