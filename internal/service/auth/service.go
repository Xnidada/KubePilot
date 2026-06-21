package auth

import (
	"errors"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	"github.com/kubepilot/kubepilot/internal/repository"
	"gorm.io/gorm"
)

type Service struct {
	userRepo   *repository.UserRepository
	jwtManager *utils.JWTManager
}

func NewService(db *gorm.DB, jwtManager *utils.JWTManager) *Service {
	return &Service{
		userRepo:   repository.NewUserRepository(db),
		jwtManager: jwtManager,
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserInfo  `json:"user"`
}

type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	RealName string `json:"real_name"`
	RoleID   uint   `json:"role_id"`
	RoleName string `json:"role_name"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	RealName string `json:"real_name"`
}

func (s *Service) Login(req *LoginRequest) (*LoginResponse, error) {
	user, err := s.userRepo.GetByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid username or password")
		}
		return nil, err
	}

	if user.Status != 1 {
		return nil, errors.New("account is disabled")
	}

	if !crypto.CheckPassword(req.Password, user.Password) {
		return nil, errors.New("invalid username or password")
	}

	token, err := s.jwtManager.GenerateToken(user.ID, user.Username, user.RoleID)
	if err != nil {
		return nil, err
	}

	// Update last login
	s.userRepo.UpdateLastLogin(user.ID)

	return &LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			RealName: user.RealName,
			RoleID:   user.RoleID,
			RoleName: user.Role.Name,
		},
	}, nil
}

func (s *Service) Register(req *RegisterRequest) (*UserInfo, error) {
	// Check if username exists
	_, err := s.userRepo.GetByUsername(req.Username)
	if err == nil {
		return nil, errors.New("username already exists")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Check if email exists
	_, err = s.userRepo.GetByEmail(req.Email)
	if err == nil {
		return nil, errors.New("email already exists")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	hashedPassword, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
		RealName: req.RealName,
		Status:   1,
		RoleID:   2, // Default role: user
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		RealName: user.RealName,
		RoleID:   user.RoleID,
	}, nil
}

func (s *Service) GetUserByID(id uint) (*UserInfo, error) {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		RealName: user.RealName,
		RoleID:   user.RoleID,
		RoleName: user.Role.Name,
	}, nil
}

func (s *Service) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}

	if !crypto.CheckPassword(oldPassword, user.Password) {
		return errors.New("incorrect old password")
	}

	hashedPassword, err := crypto.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.Password = hashedPassword
	return s.userRepo.Update(user)
}

// GenerateTokenForUser 为指定用户生成 JWT token（用于 2FA 验证后）
func (s *Service) GenerateTokenForUser(userID uint) (*LoginResponse, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}

	token, err := s.jwtManager.GenerateToken(user.ID, user.Username, user.RoleID)
	if err != nil {
		return nil, err
	}

	// Update last login
	s.userRepo.UpdateLastLogin(user.ID)

	return &LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			RealName: user.RealName,
			RoleID:   user.RoleID,
			RoleName: user.Role.Name,
		},
	}, nil
}
