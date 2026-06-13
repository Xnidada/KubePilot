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
