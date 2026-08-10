package authz

import (
	"context"

	"github.com/kubepilot/kubepilot/internal/model"
	"gorm.io/gorm"
)

type GrantResolver interface {
	Authorize(ctx context.Context, userID, clusterID uint, namespace, requiredLevel string) (bool, error)
	AllowedNamespaces(ctx context.Context, userID, clusterID uint, requiredLevel string) ([]string, error)
}

type GORMGrantResolver struct {
	db *gorm.DB
}

func NewGORMGrantResolver(db *gorm.DB) *GORMGrantResolver {
	return &GORMGrantResolver{db: db}
}

func (r *GORMGrantResolver) Authorize(
	ctx context.Context,
	userID, clusterID uint,
	namespace, requiredLevel string,
) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.UserCluster{}).
		Where("user_id = ? AND cluster_id = ?", userID, clusterID).
		Where("permission_level IN ?", acceptedLevels(requiredLevel))
	if namespace == "*" {
		query = query.Where("namespace = ?", "*")
	} else {
		query = query.Where("namespace IN ?", []string{"*", namespace})
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GORMGrantResolver) AllowedNamespaces(
	ctx context.Context,
	userID, clusterID uint,
	requiredLevel string,
) ([]string, error) {
	var namespaces []string
	err := r.db.WithContext(ctx).Model(&model.UserCluster{}).
		Where("user_id = ? AND cluster_id = ?", userID, clusterID).
		Where("permission_level IN ?", acceptedLevels(requiredLevel)).
		Distinct("namespace").
		Pluck("namespace", &namespaces).Error
	return namespaces, err
}

func acceptedLevels(required string) []string {
	switch required {
	case "read":
		return []string{"read", "write", "admin"}
	case "write":
		return []string{"write", "admin"}
	case "admin":
		return []string{"admin"}
	default:
		return []string{}
	}
}
