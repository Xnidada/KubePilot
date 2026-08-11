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
	"statefulsets",
	"daemonsets",
	"replicasets",
	"jobs",
	"cronjobs",
	"hpas",
	"pods",
	"services",
	"ingresses",
	"networkpolicies",
	"configmaps",
	"secrets",
	"pvcs",
	"pvs",
	"storageclasses",
	"crds",
	"custom_resources",
	"namespaces",
	"nodes",
	"events",
	"metrics",
	"operations",
	"cost",
	"alerts",
	"users",
	"user_groups",
	"cluster_access",
	"roles",
	"audit_logs",
	"appstore",
	"scheduler",
	"inspection",
	"event_forward",
	"aiops",
	"aiops_config",
	"backups",
	"webhooks",
}

// 预定义的操作类型
var ActionTypes = []string{
	"view",      // 查看元数据
	"view_data", // 查看敏感数据明文（如 Secret）
	"create",    // 创建
	"edit",      // 编辑
	"delete",    // 删除
	"execute",   // 执行业务操作
	"exec",      // 打开终端
	"admin",     // 管理
}

// 预定义角色模板
var RoleTemplates = map[string]PermissionList{
	"admin": {
		{Resource: "*", Actions: []string{"*"}},
	},
	"operator": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "statefulsets", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "daemonsets", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "replicasets", Actions: []string{"view", "edit", "delete"}},
		{Resource: "jobs", Actions: []string{"view", "create", "delete"}},
		{Resource: "cronjobs", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "hpas", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "pods", Actions: []string{"view", "create", "delete", "exec"}},
		{Resource: "services", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "ingresses", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "networkpolicies", Actions: []string{"view", "create", "delete"}},
		{Resource: "configmaps", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "secrets", Actions: []string{"view", "view_data", "create", "edit", "delete"}},
		{Resource: "pvcs", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "pvs", Actions: []string{"view"}},
		{Resource: "storageclasses", Actions: []string{"view"}},
		{Resource: "crds", Actions: []string{"view"}},
		{Resource: "custom_resources", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
		{Resource: "metrics", Actions: []string{"view"}},
		{Resource: "cost", Actions: []string{"view"}},
		{Resource: "operations", Actions: []string{"execute"}},
		{Resource: "alerts", Actions: []string{"view", "create", "edit", "delete"}},
		{Resource: "appstore", Actions: []string{"view", "create", "edit"}},
		{Resource: "scheduler", Actions: []string{"view", "create", "edit", "delete", "execute"}},
		{Resource: "inspection", Actions: []string{"view", "create", "edit", "delete", "execute"}},
		{Resource: "event_forward", Actions: []string{"view", "create", "edit", "delete", "execute"}},
		{Resource: "aiops", Actions: []string{"view", "execute"}},
		{Resource: "backups", Actions: []string{"view", "create", "execute"}},
		{Resource: "webhooks", Actions: []string{"view", "create", "edit", "delete"}},
	},
	"user": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view", "create"}},
		{Resource: "statefulsets", Actions: []string{"view", "create"}},
		{Resource: "daemonsets", Actions: []string{"view"}},
		{Resource: "replicasets", Actions: []string{"view"}},
		{Resource: "jobs", Actions: []string{"view", "create"}},
		{Resource: "cronjobs", Actions: []string{"view", "create"}},
		{Resource: "hpas", Actions: []string{"view", "create"}},
		{Resource: "pods", Actions: []string{"view", "exec"}},
		{Resource: "services", Actions: []string{"view", "create"}},
		{Resource: "ingresses", Actions: []string{"view", "create"}},
		{Resource: "networkpolicies", Actions: []string{"view"}},
		{Resource: "configmaps", Actions: []string{"view"}},
		{Resource: "pvcs", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
		{Resource: "metrics", Actions: []string{"view"}},
		{Resource: "cost", Actions: []string{"view"}},
		{Resource: "appstore", Actions: []string{"view"}},
		{Resource: "scheduler", Actions: []string{"view", "create"}},
		{Resource: "inspection", Actions: []string{"view"}},
	},
	"viewer": {
		{Resource: "clusters", Actions: []string{"view"}},
		{Resource: "deployments", Actions: []string{"view"}},
		{Resource: "statefulsets", Actions: []string{"view"}},
		{Resource: "daemonsets", Actions: []string{"view"}},
		{Resource: "replicasets", Actions: []string{"view"}},
		{Resource: "jobs", Actions: []string{"view"}},
		{Resource: "cronjobs", Actions: []string{"view"}},
		{Resource: "hpas", Actions: []string{"view"}},
		{Resource: "pods", Actions: []string{"view"}},
		{Resource: "services", Actions: []string{"view"}},
		{Resource: "ingresses", Actions: []string{"view"}},
		{Resource: "networkpolicies", Actions: []string{"view"}},
		{Resource: "configmaps", Actions: []string{"view"}},
		// secrets:view 仅元数据；明文需要 view_data（viewer 不授予）
		{Resource: "secrets", Actions: []string{"view"}},
		{Resource: "pvcs", Actions: []string{"view"}},
		{Resource: "pvs", Actions: []string{"view"}},
		{Resource: "storageclasses", Actions: []string{"view"}},
		{Resource: "crds", Actions: []string{"view"}},
		{Resource: "custom_resources", Actions: []string{"view"}},
		{Resource: "namespaces", Actions: []string{"view"}},
		{Resource: "nodes", Actions: []string{"view"}},
		{Resource: "events", Actions: []string{"view"}},
		{Resource: "metrics", Actions: []string{"view"}},
		{Resource: "cost", Actions: []string{"view"}},
		{Resource: "alerts", Actions: []string{"view"}},
		{Resource: "appstore", Actions: []string{"view"}},
		{Resource: "scheduler", Actions: []string{"view"}},
		{Resource: "inspection", Actions: []string{"view"}},
		{Resource: "event_forward", Actions: []string{"view"}},
		{Resource: "aiops", Actions: []string{"view"}},
		{Resource: "backups", Actions: []string{"view"}},
		{Resource: "operations", Actions: []string{"view"}},
	},
}
