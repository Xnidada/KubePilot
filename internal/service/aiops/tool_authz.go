package aiops

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
)

// ensureToolAccess checks platform role + cluster/namespace grants for a tool call.
// write=true requires write/admin grant; empty namespace means cluster-scoped ("*").
func (s *Service) ensureToolAccess(ctx context.Context, userID, clusterID uint, namespace string, write bool) error {
	if userID == 0 || clusterID == 0 {
		return fmt.Errorf("missing user or cluster for authorization")
	}
	var user model.User
	if err := s.db.WithContext(ctx).Select("id", "role_id").First(&user, userID).Error; err != nil {
		return fmt.Errorf("user not found")
	}
	var role model.Role
	if err := s.db.WithContext(ctx).First(&role, user.RoleID).Error; err != nil {
		return fmt.Errorf("role not found")
	}
	if role.IsSystem || role.Name == "admin" {
		return nil
	}
	perms, err := model.ParsePermissions(role.Permissions)
	if err != nil {
		return fmt.Errorf("invalid permissions")
	}
	action := "view"
	if write {
		action = "execute"
	}
	if !perms.HasPermission("aiops", action) && !perms.HasPermission("aiops", "execute") {
		// allow execute to cover both read tools and write staging
		if !perms.HasPermission("aiops", "view") && !write {
			return fmt.Errorf("insufficient aiops permissions")
		}
		if write && !perms.HasPermission("aiops", "execute") {
			return fmt.Errorf("insufficient aiops permissions for write")
		}
	}
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "*"
	}
	level := "read"
	if write {
		level = "write"
	}
	ok, err := authz.NewGORMGrantResolver(s.db).Authorize(ctx, userID, clusterID, ns, level)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("cluster access denied for namespace %q", ns)
	}
	return nil
}

func toolNamespaceFromArgs(argsJSON string) string {
	var args struct {
		Namespace string `json:"namespace"`
	}
	_ = parseToolArgs(argsJSON, &args)
	return strings.TrimSpace(args.Namespace)
}

func toolIsWrite(name string) bool {
	switch name {
	case "propose_mutation", "stage_mutation", "stage_mutations", "delete_by_prefix":
		return true
	default:
		return false
	}
}
