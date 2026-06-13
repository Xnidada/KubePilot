import { get, post, put, del } from './request'

export interface AlertRule {
  id: number
  name: string
  cluster_id: number
  cluster?: { name: string }
  namespace: string
  resource: string
  metric: string
  condition: string
  threshold: number
  duration: string
  enabled: boolean
  last_alert: string
  created_at: string
}

export interface AlertHistory {
  id: number
  rule_id: number
  rule?: { name: string }
  cluster_id: number
  cluster?: { name: string }
  namespace: string
  resource: string
  message: string
  value: number
  status: string
  triggered_at: string
  resolved_at: string
}

export interface NotificationChannel {
  id: number
  name: string
  type: string
  config: string
  enabled: boolean
  created_at: string
}

// Alert Rules
export const getAlertRules = (page = 1, size = 10, clusterId?: number) => {
  const params: any = { page, size }
  if (clusterId) params.cluster_id = clusterId
  return get<{ code: number; data: AlertRule[]; total: number }>('/alerts/rules', { params })
}

export const createAlertRule = (data: {
  name: string
  cluster_id: number
  namespace?: string
  resource?: string
  metric: string
  condition: string
  threshold: number
  duration?: string
  channels?: number[]
}) => {
  return post('/alerts/rules', data)
}

export const updateAlertRule = (id: number, data: {
  name?: string
  metric?: string
  condition?: string
  threshold?: number
  duration?: string
  enabled?: boolean
}) => {
  return put(`/alerts/rules/${id}`, data)
}

export const deleteAlertRule = (id: number) => {
  return del(`/alerts/rules/${id}`)
}

// Alert History
export const getAlertHistory = (page = 1, size = 20, status?: string) => {
  const params: any = { page, size }
  if (status) params.status = status
  return get<{ code: number; data: AlertHistory[]; total: number }>('/alerts/history', { params })
}

// Notification Channels
export const getNotificationChannels = () => {
  return get<{ code: number; data: NotificationChannel[] }>('/alerts/channels')
}

export const createNotificationChannel = (data: {
  name: string
  type: string
  config: string
}) => {
  return post('/alerts/channels', data)
}

export const updateNotificationChannel = (id: number, data: {
  name?: string
  config?: string
  enabled?: boolean
}) => {
  return put(`/alerts/channels/${id}`, data)
}

export const deleteNotificationChannel = (id: number) => {
  return del(`/alerts/channels/${id}`)
}
