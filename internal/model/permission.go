package model

import "encoding/json"

// Permission 权限定义
type Permission struct {
	Resource string   `json:"resource"` // 资源类型：clusters, deployments, pods, services, etc.
	Actions  []string `json:"actions"`  // 操作列表：view, create, edit, delete
}

// PermissionList 权限列表
type PermissionList []Permission

// HasPermission 检查是否有指定权限
func (pl PermissionList) HasPermission(resource, action string) bool {
	for _, p := range pl {
		if p.Resource == "*" || p.Resource == resource {
			for _, a := range p.Actions {
				if a == "*" || a == action {
					return true
				}
			}
		}
	}
	return false
}

// ToJSON 转换为JSON字符串
func (pl PermissionList) ToJSON() string {
	bytes, err := json.Marshal(pl)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

// ParsePermissions 从JSON字符串解析权限
func ParsePermissions(jsonStr string) (PermissionList, error) {
	var permissions PermissionList
	if jsonStr == "" || jsonStr == "{}" {
		return permissions, nil
	}
	err := json.Unmarshal([]byte(jsonStr), &permissions)
	return permissions, err
}

// 预定义的资源类型
var ResourceTypes = []string{
	"clusters",
	"deployments",
	"pods",
	"services",
	"configmaps",
	"secrets",
	"pvcs",
	"pvs",
	"namespaces",
	"nodes",
	"events",
	"alerts",
	"users",
	"roles",
	"audit_logs",
	"appstore",
}

// 预定义的操作类型
var ActionTypes = []string{
	"view",    // 查看
	"create",  // 创建
	"edit",    // 编辑
	"delete",  // 删除
	"exec",    // 执行（如终端）
	"admin",   // 管理
}

// 预定义角色模板
var RoleTemplates = map[string]PermissionList{
	"admin": {
		{Resource: "*", Actions: []string{"*"}},
	},
	"operator": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "pods", Actions: []string{"view", "create", "delete", "exec"}},
		{Resource: "services", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "configmaps", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "secrets", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "pvcs", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "pvs", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
		{Resource: "alerts", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "appstore", Actions: []string{"view", "create", "edit"}},
	},
	"user": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view", "create", "edit"}},
		{Resource: "pods", Actions: []string{"view", "exec"}},
		{Resource: "services", Actions: []string{"view"}},
		{Resource: "configmaps", Actions: []string{"view"}},
		{Resource: "pvcs", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
		{Resource: "appstore", Actions: []string{"view"}},
	},
	"viewer": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view"}},
		{Resource: "pods", Actions: []string{"view"}},
		{Resource: "services", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
	},
}
