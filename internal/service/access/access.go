package access

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kubepilot/kubepilot/internal/model"
	"gorm.io/gorm"
)

type PermissionLevel string

const (
	PermissionRead  PermissionLevel = "read"
	PermissionWrite PermissionLevel = "write"
	PermissionAdmin PermissionLevel = "admin"
)

type Grant struct {
	ClusterID       uint            `json:"cluster_id"`
	Namespace       string          `json:"namespace"`
	PermissionLevel PermissionLevel `json:"permission_level"`
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Effective returns the union of direct user grants and grants inherited from
// enabled memberships, enabled groups, and enabled group-cluster assignments.
func (s *Service) Effective(ctx context.Context, userID uint) (*EffectiveAccess, error) {
	var direct []model.UserCluster
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&direct).Error; err != nil {
		return nil, fmt.Errorf("list direct grants: %w", err)
	}

	var inherited []model.GroupCluster
	if err := s.db.WithContext(ctx).
		Table("group_clusters AS gc").
		Select("gc.*").
		Joins("JOIN user_groups AS ug ON ug.id = gc.group_id AND ug.status = 1 AND ug.deleted_at IS NULL").
		Joins("JOIN user_group_members AS ugm ON ugm.group_id = ug.id AND ugm.status = 1").
		Where("ugm.user_id = ? AND gc.status = 1", userID).
		Find(&inherited).Error; err != nil {
		return nil, fmt.Errorf("list inherited grants: %w", err)
	}

	grants := make([]Grant, 0, len(direct)+len(inherited))
	for _, item := range direct {
		grants = append(grants, Grant{
			ClusterID:       item.ClusterID,
			Namespace:       item.Namespace,
			PermissionLevel: PermissionLevel(item.PermissionLevel),
		})
	}
	for _, item := range inherited {
		grants = append(grants, Grant{
			ClusterID:       item.ClusterID,
			Namespace:       item.Namespace,
			PermissionLevel: PermissionLevel(item.PermissionLevel),
		})
	}
	return NewEffectiveAccess(grants), nil
}

func (s *Service) CanCluster(ctx context.Context, userID, clusterID uint, required PermissionLevel) (bool, error) {
	effective, err := s.Effective(ctx, userID)
	if err != nil {
		return false, err
	}
	return effective.CanCluster(clusterID, required), nil
}

func (s *Service) CanNamespace(ctx context.Context, userID, clusterID uint, namespace string, required PermissionLevel) (bool, error) {
	effective, err := s.Effective(ctx, userID)
	if err != nil {
		return false, err
	}
	return effective.CanNamespace(clusterID, namespace, required), nil
}

func (s *Service) ClusterIDs(ctx context.Context, userID uint, required PermissionLevel) ([]uint, error) {
	effective, err := s.Effective(ctx, userID)
	if err != nil {
		return nil, err
	}
	return effective.ClusterIDs(required), nil
}

func (s *Service) Namespaces(ctx context.Context, userID, clusterID uint, required PermissionLevel) ([]string, error) {
	effective, err := s.Effective(ctx, userID)
	if err != nil {
		return nil, err
	}
	return effective.Namespaces(clusterID, required), nil
}

type grantKey struct {
	clusterID uint
	namespace string
}

// EffectiveAccess is an immutable, merged permission snapshot.
type EffectiveAccess struct {
	grants map[grantKey]PermissionLevel
}

func NewEffectiveAccess(grants []Grant) *EffectiveAccess {
	effective := &EffectiveAccess{grants: make(map[grantKey]PermissionLevel, len(grants))}
	for _, grant := range grants {
		namespace := strings.TrimSpace(grant.Namespace)
		if namespace == "" {
			namespace = "*"
		}
		if !grant.PermissionLevel.Valid() {
			continue
		}
		key := grantKey{clusterID: grant.ClusterID, namespace: namespace}
		if current, ok := effective.grants[key]; !ok || grant.PermissionLevel.rank() > current.rank() {
			effective.grants[key] = grant.PermissionLevel
		}
	}
	return effective
}

func (e *EffectiveAccess) Grants() []Grant {
	result := make([]Grant, 0, len(e.grants))
	for key, level := range e.grants {
		result = append(result, Grant{
			ClusterID:       key.clusterID,
			Namespace:       key.namespace,
			PermissionLevel: level,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ClusterID != result[j].ClusterID {
			return result[i].ClusterID < result[j].ClusterID
		}
		return result[i].Namespace < result[j].Namespace
	})
	return result
}

// CanCluster requires a wildcard grant because cluster-scoped resources are
// not safely covered by access to one namespace.
func (e *EffectiveAccess) CanCluster(clusterID uint, required PermissionLevel) bool {
	return e.allows(grantKey{clusterID: clusterID, namespace: "*"}, required)
}

func (e *EffectiveAccess) CanNamespace(clusterID uint, namespace string, required PermissionLevel) bool {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return false
	}
	return e.allows(grantKey{clusterID: clusterID, namespace: "*"}, required) ||
		e.allows(grantKey{clusterID: clusterID, namespace: namespace}, required)
}

func (e *EffectiveAccess) ClusterIDs(required PermissionLevel) []uint {
	if !required.Valid() {
		return []uint{}
	}
	seen := make(map[uint]struct{})
	for key, level := range e.grants {
		if level.rank() >= required.rank() {
			seen[key.clusterID] = struct{}{}
		}
	}
	result := make([]uint, 0, len(seen))
	for clusterID := range seen {
		result = append(result, clusterID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// Namespaces returns ["*"] when a wildcard grant satisfies the requested
// level; otherwise it returns the sorted names of matching namespaces.
func (e *EffectiveAccess) Namespaces(clusterID uint, required PermissionLevel) []string {
	if !required.Valid() {
		return []string{}
	}
	if e.CanCluster(clusterID, required) {
		return []string{"*"}
	}
	result := make([]string, 0)
	for key, level := range e.grants {
		if key.clusterID == clusterID && key.namespace != "*" && level.rank() >= required.rank() {
			result = append(result, key.namespace)
		}
	}
	sort.Strings(result)
	return result
}

func (e *EffectiveAccess) allows(key grantKey, required PermissionLevel) bool {
	if !required.Valid() {
		return false
	}
	actual, ok := e.grants[key]
	return ok && actual.rank() >= required.rank()
}

func (l PermissionLevel) Valid() bool {
	switch l {
	case PermissionRead, PermissionWrite, PermissionAdmin:
		return true
	default:
		return false
	}
}

func (l PermissionLevel) rank() int {
	switch l {
	case PermissionRead:
		return 1
	case PermissionWrite:
		return 2
	case PermissionAdmin:
		return 3
	default:
		return 0
	}
}
