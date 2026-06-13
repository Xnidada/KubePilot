import { get } from './request'

export interface PodMetric {
  name: string
  namespace: string
  status?: string
  node?: string
  total_cpu_millis?: number
  total_memory_mi?: number
  cpu_request_m?: number
  cpu_limit_m?: number
  memory_request_mi?: number
  memory_limit_mi?: number
  containers: ContainerMetric[]
}

export interface ContainerMetric {
  name: string
  image?: string
  cpu_millis?: number
  memory_mi?: number
  cpu_request_m?: number
  cpu_limit_m?: number
  memory_request_mi?: number
  memory_limit_mi?: number
}

export interface DeploymentMetric {
  name: string
  namespace: string
  replicas: number
  ready: number
  available: number
  pod_count: number
  cpu_request_m: number
  cpu_limit_m: number
  memory_request_mi: number
  memory_limit_mi: number
  cpu_usage_m: number
  memory_usage_mi: number
  cpu_usage_percent: number
  memory_usage_percent: number
}

export interface NodeMetric {
  name: string
  ip: string
  ready: boolean
  cpu_capacity_m: number
  memory_capacity_mi: number
  pod_capacity: number
  pod_count: number
  cpu_allocated_m: number
  memory_allocated_mi: number
  cpu_allocated_percent: number
  memory_allocated_percent: number
  cpu_usage_m: number
  memory_usage_mi: number
  cpu_usage_percent: number
  memory_usage_percent: number
}

export interface ClusterOverview {
  node_count: number
  deployment_count: number
  pod_count: number
  pod_running: number
  pod_pending: number
  pod_succeeded: number
  pod_failed: number
  cpu_capacity_m: number
  memory_capacity_mi: number
  cpu_allocated_m: number
  memory_allocated_mi: number
  cpu_allocated_percent: number
  memory_allocated_percent: number
  cpu_usage_m: number
  memory_usage_mi: number
  cpu_usage_percent: number
  memory_usage_percent: number
}

// 获取Pod资源指标
export const getPodMetrics = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: PodMetric[] }>(`/clusters/${clusterId}/workloads/metrics/pods`, { params })
}

// 获取Deployment资源指标
export const getDeploymentMetrics = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: DeploymentMetric[] }>(`/clusters/${clusterId}/workloads/metrics/deployments`, { params })
}

// 获取节点资源指标
export const getNodeMetrics = (clusterId: number) => {
  return get<{ code: number; data: NodeMetric[] }>(`/clusters/${clusterId}/workloads/metrics/nodes`)
}

// 获取集群资源概览
export const getClusterOverview = (clusterId: number) => {
  return get<{ code: number; data: ClusterOverview }>(`/clusters/${clusterId}/workloads/metrics/overview`)
}
