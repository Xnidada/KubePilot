import { get, post, put, del } from './request'

// ==================== ConfigMap ====================
export interface ConfigMap {
  name: string
  namespace: string
  keys: string[]
  data_count: number
  age: string
}

export const getConfigMaps = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: ConfigMap[] }>(`/clusters/${clusterId}/workloads/configmaps`, { params })
}

export const getConfigMap = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any }>(`/clusters/${clusterId}/workloads/configmaps/${namespace}/${name}`)
}

export const createConfigMap = (clusterId: number, data: {
  namespace: string
  name: string
  data: Record<string, string>
  labels?: Record<string, string>
}) => {
  return post(`/clusters/${clusterId}/workloads/configmaps`, data)
}

export const updateConfigMap = (clusterId: number, namespace: string, name: string, data: {
  data: Record<string, string>
  labels?: Record<string, string>
}) => {
  return put(`/clusters/${clusterId}/workloads/configmaps/${namespace}/${name}`, data)
}

export const deleteConfigMap = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/configmaps/${namespace}/${name}`)
}

// ==================== Secret ====================
export interface Secret {
  name: string
  namespace: string
  type: string
  keys: string[]
  data_count: number
  age: string
}

export const getSecrets = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Secret[] }>(`/clusters/${clusterId}/workloads/secrets`, { params })
}

export const getSecret = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any }>(`/clusters/${clusterId}/workloads/secrets/${namespace}/${name}`)
}

export const createSecret = (clusterId: number, data: {
  namespace: string
  name: string
  type?: string
  data: Record<string, string>
  labels?: Record<string, string>
}) => {
  return post(`/clusters/${clusterId}/workloads/secrets`, data)
}

export const updateSecret = (clusterId: number, namespace: string, name: string, data: {
  data: Record<string, string>
  labels?: Record<string, string>
}) => {
  return put(`/clusters/${clusterId}/workloads/secrets/${namespace}/${name}`, data)
}

export const deleteSecret = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/secrets/${namespace}/${name}`)
}

// ==================== Ingress ====================
export interface Ingress {
  name: string
  namespace: string
  class_name: string
  hosts: string[]
  paths: string[]
  address: string
  tls: boolean
  age: string
}

export const getIngresses = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Ingress[] }>(`/clusters/${clusterId}/workloads/ingresses`, { params })
}

export const getIngress = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any }>(`/clusters/${clusterId}/workloads/ingresses/${namespace}/${name}`)
}

export const createIngress = (clusterId: number, data: {
  namespace: string
  name: string
  class_name?: string
  host: string
  paths: { path: string; path_type?: string; service: string; port: number }[]
  tls_secret?: string
  annotations?: Record<string, string>
  labels?: Record<string, string>
}) => {
  return post(`/clusters/${clusterId}/workloads/ingresses`, data)
}

export const updateIngress = (clusterId: number, namespace: string, name: string, data: any) => {
  return put(`/clusters/${clusterId}/workloads/ingresses/${namespace}/${name}`, data)
}

export const deleteIngress = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/ingresses/${namespace}/${name}`)
}

// ==================== Namespace ====================
export interface NamespaceDetail {
  name: string
  status: string
  labels: Record<string, string>
  age: string
  resources: {
    pods: number
    services: number
    deployments: number
    configmaps: number
    secrets: number
  }
}

export const getNamespaceDetail = (clusterId: number, name: string) => {
  return get<{ code: number; data: NamespaceDetail }>(`/clusters/${clusterId}/workloads/namespaces/${name}`)
}

export const createNamespace = (clusterId: number, data: {
  name: string
  labels?: Record<string, string>
}) => {
  return post(`/clusters/${clusterId}/workloads/namespaces`, data)
}

export const deleteNamespace = (clusterId: number, name: string) => {
  return del(`/clusters/${clusterId}/workloads/namespaces/${name}`)
}

export const getResourceQuota = (clusterId: number, namespace: string) => {
  return get<{ code: number; data: any[] }>(`/clusters/${clusterId}/workloads/namespaces/${namespace}/quotas`)
}

export const createResourceQuota = (clusterId: number, data: {
  namespace: string
  name: string
  cpu?: string
  memory?: string
  pods?: string
}) => {
  return post(`/clusters/${clusterId}/workloads/namespaces/${data.namespace}/quotas`, data)
}
