import { get, post, put, del } from './request'

export interface User {
  id: number
  username: string
  email: string
  real_name: string
  phone: string
  status: number
  role_id: number
  role_name: string
  last_login: string
  created_at: string
}

export interface Role {
  id: number
  name: string
  description: string
  permissions: string
  is_system: boolean
}

export interface AuditLog {
  id: number
  user_id: number
  username: string
  action: string
  resource_type: string
  resource_name: string
  cluster_id: number
  namespace: string
  request_body: string
  response_code: number
  latency: number
  ip: string
  success: boolean
  created_at: string
}

// Users
export const getUsers = (page = 1, size = 10) => {
  return get<{ code: number; data: User[]; total: number; page: number; size: number }>('/system/users', { params: { page, size } })
}

export const getUser = (id: number) => {
  return get<{ code: number; data: User }>(`/system/users/${id}`)
}

export const createUser = (data: {
  username: string
  email: string
  password: string
  real_name?: string
  phone?: string
  role_id: number
}) => {
  return post('/system/users', data)
}

export const updateUser = (id: number, data: {
  email?: string
  real_name?: string
  phone?: string
  role_id?: number
  status?: number
}) => {
  return put(`/system/users/${id}`, data)
}

export const deleteUser = (id: number) => {
  return del(`/system/users/${id}`)
}

export const resetPassword = (id: number) => {
  return post(`/system/users/${id}/reset-password`)
}

// Roles
export const getRoles = () => {
  return get<{ code: number; data: Role[] }>('/system/roles')
}

export const getRole = (id: number) => {
  return get<{ code: number; data: Role }>(`/system/roles/${id}`)
}

export const createRole = (data: {
  name: string
  description?: string
  permissions: Permission[]
}) => {
  return post('/system/roles', data)
}

export const updateRole = (id: number, data: {
  name?: string
  description?: string
  permissions?: Permission[]
}) => {
  return put(`/system/roles/${id}`, data)
}

export const deleteRole = (id: number) => {
  return del(`/system/roles/${id}`)
}

export interface Permission {
  resource: string
  actions: string[]
}

// Resource and Action types
export const getResourceTypes = () => {
  return get<{ code: number; data: string[] }>('/system/resources')
}

export const getActionTypes = () => {
  return get<{ code: number; data: string[] }>('/system/actions')
}

export const getRoleTemplates = () => {
  return get<{ code: number; data: Record<string, Permission[]> }>('/system/role-templates')
}

// Audit Logs
export const getAuditLogs = (params?: {
  page?: number
  size?: number
  username?: string
  action?: string
  resource?: string
}) => {
  return get<{ code: number; data: AuditLog[]; total: number }>('/system/audit-logs', { params })
}

// ==================== 两步验证 ====================

export const get2FAStatus = () => {
  return get<{ code: number; data: { configured: boolean; enabled: boolean } }>('/2fa/status')
}

export const setup2FA = () => {
  return post<{ code: number; data: { secret: string; qr_code_url: string; backup_codes: string[] } }>('/2fa/setup')
}

export const verifyAndEnable2FA = (code: string) => {
  return post('/2fa/verify-enable', { code })
}

export const disable2FA = (code: string) => {
  return post('/2fa/disable', { code })
}

export const verify2FALogin = (userId: number, code: string) => {
  return post<{ code: number; data: { verified: boolean; user_id: number } }>('/auth/2fa/verify', { user_id: userId, code })
}

// ==================== 集群巡检 ====================

export interface InspectionRule {
  id: number
  cluster_id: number
  name: string
  description: string
  resource: string
  check_type: string
  condition: string
  threshold: string
  script: string
  schedule: string
  enabled: boolean
  created_at: string
}

export interface InspectionReport {
  id: number
  rule_id: number
  cluster_id: number
  status: string
  total_checks: number
  passed: number
  failed: number
  warnings: number
  error: string
  started_at: string
  completed_at: string
  created_at: string
}

export interface InspectionResult {
  id: number
  report_id: number
  resource_type: string
  resource_name: string
  namespace: string
  status: string
  message: string
  details: string
  created_at: string
}

export const listInspectionRules = (clusterId?: number) => {
  const params = clusterId ? `?cluster_id=${clusterId}` : ''
  return get<{ code: number; data: InspectionRule[] }>(`/inspection/rules${params}`)
}

export const createInspectionRule = (data: Partial<InspectionRule>) => {
  return post<{ code: number; data: InspectionRule }>('/inspection/rules', data)
}

export const updateInspectionRule = (id: number, data: Partial<InspectionRule>) => {
  return put(`/inspection/rules/${id}`, data)
}

export const deleteInspectionRule = (id: number) => {
  return del(`/inspection/rules/${id}`)
}

export const runInspection = (ruleId: number) => {
  return post<{ code: number; data: { report_id: number; status: string } }>(`/inspection/rules/${ruleId}/run`)
}

export const listInspectionReports = (clusterId?: number, ruleId?: number) => {
  let params = '?'
  if (clusterId) params += `cluster_id=${clusterId}&`
  if (ruleId) params += `rule_id=${ruleId}&`
  return get<{ code: number; data: InspectionReport[] }>(`/inspection/reports${params}`)
}

export const getInspectionReport = (id: number) => {
  return get<{ code: number; data: InspectionReport }>(`/inspection/reports/${id}`)
}

export const getInspectionResults = (reportId: number) => {
  return get<{ code: number; data: InspectionResult[] }>(`/inspection/reports/${reportId}/results`)
}

// ==================== Event 转发 ====================

export interface EventForwardRule {
  id: number
  cluster_id: number
  name: string
  description: string
  webhook_url: string
  namespaces: string
  resources: string
  event_types: string
  reasons: string
  headers: string
  template: string
  enabled: boolean
  created_at: string
}

export interface EventForwardLog {
  id: number
  rule_id: number
  cluster_id: number
  namespace: string
  resource: string
  event_type: string
  reason: string
  message: string
  status: string
  status_code: number
  error: string
  created_at: string
}

export const listEventForwardRules = (clusterId?: number) => {
  const params = clusterId ? `?cluster_id=${clusterId}` : ''
  return get<{ code: number; data: EventForwardRule[] }>(`/event-forward/rules${params}`)
}

export const createEventForwardRule = (data: Partial<EventForwardRule>) => {
  return post<{ code: number; data: EventForwardRule }>('/event-forward/rules', data)
}

export const updateEventForwardRule = (id: number, data: Partial<EventForwardRule>) => {
  return put(`/event-forward/rules/${id}`, data)
}

export const deleteEventForwardRule = (id: number) => {
  return del(`/event-forward/rules/${id}`)
}

export const testEventForwardRule = (id: number) => {
  return post(`/event-forward/rules/${id}/test`)
}

export const listEventForwardLogs = (ruleId?: number, status?: string) => {
  let params = '?'
  if (ruleId) params += `rule_id=${ruleId}&`
  if (status) params += `status=${status}&`
  return get<{ code: number; data: EventForwardLog[] }>(`/event-forward/logs${params}`)
}

// ==================== SSO/OAuth ====================

export interface OAuthProvider {
  id: number
  provider: string
  name: string
  enabled: boolean
}

export const listOAuthProviders = () => {
  return get<{ code: number; data: OAuthProvider[] }>('/oauth/providers')
}

export const initiateOAuthLogin = (provider: string) => {
  return get<{ code: number; data: { auth_url: string; state: string } }>(`/oauth/${provider}/login`)
}
