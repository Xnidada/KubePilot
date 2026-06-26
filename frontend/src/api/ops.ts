import { get, post } from './request'

// ==================== Pod 诊断 ====================

export interface PodDiagnosis {
  pod_name: string
  namespace: string
  status: string
  node: string
  ip: string
  restarts: number
  age: string
  containers: Array<{
    name: string
    image: string
    ready: boolean
    restart_count: number
    state: string
    exit_code: number
    reason: string
  }>
  events: Array<{
    type: string
    reason: string
    message: string
    count: number
    first_time: string
    last_time: string
  }>
  conditions: Array<{
    type: string
    status: string
    reason: string
    message: string
  }>
  resource_usage: {
    cpu_request: string
    cpu_limit: string
    mem_request: string
    mem_limit: string
    pod_scheduled: boolean
    ready: boolean
    initialized: boolean
    containers_ready: boolean
  }
  labels: Record<string, string>
  owner_ref: string
  qos_class: string
  volumes: string[]
  problems: string[]
  suggestions: string[]
}

export const diagnosePod = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: PodDiagnosis }>(`/ops/${clusterId}/diagnose/pod/${namespace}/${name}`)
}

// ==================== 资源趋势 ====================

export interface ResourceMetrics {
  timestamp: string
  cpu_usage: string
  memory_usage: string
  cpu_percent: number
  mem_percent: number
}

export const getResourceTrend = (clusterId: number, type: string, name: string, ns?: string) => {
  return get<{ code: number; data: { metrics: ResourceMetrics[] } }>(`/ops/${clusterId}/metrics/trend`, {
    params: { type, name, ns }
  })
}

// ==================== 事件时间线 ====================

export interface TimelineEvent {
  time: string
  type: string
  reason: string
  message: string
  namespace: string
  resource_kind: string
  resource_name: string
  count: number
}

export const getEventTimeline = (clusterId: number, ns?: string, hours?: number) => {
  return get<{ code: number; data: { total: number; warning_count: number; events: TimelineEvent[] } }>(`/ops/${clusterId}/events/timeline`, {
    params: { ns, hours }
  })
}

// ==================== 节点压力 ====================

export interface NodePressure {
  name: string
  status: string
  cpu_capacity: string
  cpu_allocated: string
  cpu_percent: number
  mem_capacity: string
  mem_allocated: string
  mem_percent: number
  pod_capacity: number
  pod_count: number
  pod_percent: number
  pressure_level: string
}

export const getNodePressure = (clusterId: number) => {
  return get<{ code: number; data: NodePressure[] }>(`/ops/${clusterId}/nodes/pressure`)
}

// ==================== 回滚 ====================

export const rollbackDeployment = (clusterId: number, namespace: string, name: string, revision?: number) => {
  return post(`/ops/${clusterId}/rollback/deployment/${namespace}/${name}`, { revision })
}

// ==================== 资源依赖图 ====================

export interface ResourceGraph {
  nodes: Array<{
    id: string
    kind: string
    name: string
    namespace: string
    status: string
  }>
  edges: Array<{
    source: string
    target: string
    type: string
  }>
}

export const getResourceGraph = (clusterId: number, ns?: string) => {
  return get<{ code: number; data: ResourceGraph }>(`/ops/${clusterId}/resource-graph`, {
    params: { ns }
  })
}

// ==================== RBAC ====================

export interface RBACInfo {
  roles: Array<{
    name: string
    namespace: string
    rules: Array<{
      resources: string[]
      verbs: string[]
      api_groups: string[]
    }>
  }>
  cluster_roles: Array<{
    name: string
    rules: Array<{
      resources: string[]
      verbs: string[]
      api_groups: string[]
    }>
  }>
  role_bindings: Array<{
    name: string
    namespace: string
    role: string
    subjects: Array<{
      kind: string
      name: string
      namespace: string
    }>
  }>
  cluster_role_bindings: Array<{
    name: string
    role: string
    subjects: Array<{
      kind: string
      name: string
      namespace: string
    }>
  }>
  user_permissions: Record<string, string[]>
}

export const getRBACVisualization = (clusterId: number) => {
  return get<{ code: number; data: RBACInfo }>(`/ops/${clusterId}/rbac`)
}

// ==================== 闲置资源 ====================

export interface IdleResource {
  kind: string
  name: string
  namespace: string
  age: string
  reason: string
}

export const findIdleResources = (clusterId: number) => {
  return get<{ code: number; data: { total: number; resources: IdleResource[] } }>(`/ops/${clusterId}/idle-resources`)
}

export const cleanIdleResources = (clusterId: number, resources: Array<{ kind: string; name: string; namespace: string }>) => {
  return post(`/ops/${clusterId}/idle-resources/clean`, { resources })
}
