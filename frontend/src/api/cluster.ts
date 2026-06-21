import { get, post, put, del } from './request'

export interface Cluster {
  id: number
  name: string
  display_name: string
  description: string
  api_server: string
  status: string
  version: string
  node_count: number
  cpu_capacity: string
  memory_capacity: string
  last_health_check: string | null
  tags: string
  created_at: string
}

export interface ClusterNode {
  name: string
  ip: string
  cpu_capacity: string
  mem_capacity: string
  os: string
  kernel: string
  container_rt: string
  kubelet_ver: string
  ready: boolean
}

export interface ClusterInfo {
  version: string
  node_count: number
  cpu_capacity: string
  mem_capacity: string
  nodes: ClusterNode[]
}

export interface CreateClusterRequest {
  name: string
  display_name?: string
  description?: string
  api_server: string
  kubeconfig: string
  tags?: string
}

interface ListResponse {
  code: number
  data: Cluster[]
  total: number
  page: number
  size: number
}

interface DetailResponse {
  code: number
  data: Cluster
}

interface InfoResponse {
  code: number
  data: ClusterInfo
}

export const getClusterList = (page = 1, size = 10) => {
  return get<ListResponse>('/clusters', { params: { page, size } })
}

export const getClusterDetail = (id: number) => {
  return get<DetailResponse>(`/clusters/${id}`)
}

export const createCluster = (data: CreateClusterRequest) => {
  return post('/clusters', data)
}

export interface UpdateClusterRequest {
  display_name?: string
  description?: string
  api_server?: string
  kubeconfig?: string
  tags?: string
}

export const updateCluster = (id: number, data: UpdateClusterRequest) => {
  return put(`/clusters/${id}`, data)
}

export const deleteCluster = (id: number) => {
  return del(`/clusters/${id}`)
}

export const healthCheckCluster = (id: number) => {
  return post(`/clusters/${id}/health`)
}

export const getClusterInfo = (id: number) => {
  return get<InfoResponse>(`/clusters/${id}/info`)
}

export const getClusterNamespaces = (id: number) => {
  return get<string[]>(`/clusters/${id}/namespaces`)
}

export const getClusterNodes = (id: number) => {
  return get<ClusterNode[]>(`/clusters/${id}/nodes`)
}
