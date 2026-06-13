import { get, post, put, del } from './request'

export interface Deployment {
  name: string
  namespace: string
  ready: string
  up_to_date: number
  available: number
  age: string
  images: string[]
}

export interface DeploymentDetail {
  name: string
  namespace: string
  labels: Record<string, string>
  annotations: Record<string, string>
  replicas: number
  max_surge?: number
  max_unavailable?: number
  containers: ContainerDetail[]
  volumes?: VolumeConfig[]
  node_selector?: Record<string, string>
  tolerations?: Toleration[]
  service_account_name?: string
  dns_policy?: string
  host_network?: boolean
  restart_policy?: string
  termination_grace_period?: number
}

export interface ContainerDetail {
  name: string
  image: string
  image_pull_policy?: string
  command?: string[]
  args?: string[]
  resources?: ResourceConfig
  ports?: ContainerPort[]
  env?: EnvVar[]
  volume_mounts?: VolumeMount[]
  liveness_probe?: ProbeConfig
  readiness_probe?: ProbeConfig
}

export interface Pod {
  name: string
  namespace: string
  status: string
  ready: string
  restarts: number
  age: string
  node: string
  ip: string
}

export interface Service {
  name: string
  namespace: string
  type: string
  cluster_ip: string
  ports: string
  age: string
}

export interface Node {
  name: string
  ip: string
  status: string
  roles: string
  cpu_capacity: string
  mem_capacity: string
  os: string
  kernel: string
  container_rt: string
  kubelet_ver: string
}

export interface Event {
  type: string
  reason: string
  message: string
  namespace: string
  object: string
  age: string
}

// 企业级Deployment创建请求
export interface EnterpriseDeploymentRequest {
  name: string
  namespace: string
  description?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  replicas?: number
  max_surge?: number
  max_unavailable?: number
  containers: ContainerConfig[]
  init_containers?: ContainerConfig[]
  volumes?: VolumeConfig[]
  scheduling?: SchedulingConfig
  network?: NetworkConfig
  advanced?: AdvancedConfig
}

export interface ContainerConfig {
  name: string
  image: string
  image_pull_policy?: string
  command?: string[]
  args?: string[]
  working_dir?: string
  resources?: ResourceConfig
  ports?: ContainerPort[]
  env?: EnvVar[]
  volume_mounts?: VolumeMount[]
  liveness_probe?: ProbeConfig
  readiness_probe?: ProbeConfig
  startup_probe?: ProbeConfig
  lifecycle?: LifecycleConfig
  security_context?: ContainerSecurityContext
}

export interface ResourceConfig {
  cpu_request?: string
  cpu_limit?: string
  memory_request?: string
  memory_limit?: string
  gpu_request?: string
  gpu_limit?: string
}

export interface ContainerPort {
  name?: string
  container_port: number
  protocol?: string
}

export interface EnvVar {
  name: string
  value?: string
  value_from?: EnvVarSource
}

export interface EnvVarSource {
  type: string
  configmap_name?: string
  configmap_key?: string
  secret_name?: string
  secret_key?: string
  field_path?: string
}

export interface VolumeMount {
  name: string
  mount_path: string
  sub_path?: string
  read_only?: boolean
}

export interface VolumeConfig {
  name: string
  type: string
  empty_dir?: { medium?: string; size_limit?: string }
  host_path?: { path: string; type?: string }
  nfs?: { server: string; path: string; read_only?: boolean }
  configmap?: string
  secret?: string
  pvc_name?: string
}

export interface ProbeConfig {
  probe_type: string
  http_get?: { path: string; port: number; scheme?: string; headers?: Record<string, string> }
  tcp_socket?: { port: number }
  exec?: { command: string[] }
  initial_delay_seconds?: number
  period_seconds?: number
  timeout_seconds?: number
  success_threshold?: number
  failure_threshold?: number
}

export interface LifecycleConfig {
  post_start?: LifecycleHandler
  pre_stop?: LifecycleHandler
}

export interface LifecycleHandler {
  type: string
  command?: string[]
  http_get?: { path: string; port: number; scheme?: string }
}

export interface ContainerSecurityContext {
  privileged?: boolean
  read_only_root_filesystem?: boolean
  run_as_user?: number
  run_as_group?: number
  run_as_non_root?: boolean
  allow_privilege_escalation?: boolean
}

export interface SchedulingConfig {
  node_selector?: Record<string, string>
  node_affinity?: NodeAffinityTerm[]
  pod_affinity?: PodAffinityTerm[]
  pod_anti_affinity?: PodAffinityTerm[]
  tolerations?: Toleration[]
}

export interface NodeAffinityTerm {
  weight?: number
  key: string
  operator: string
  values?: string[]
}

export interface PodAffinityTerm {
  weight?: number
  topology_key: string
  label_selector?: Record<string, string>
  namespaces?: string[]
}

export interface Toleration {
  key?: string
  operator?: string
  value?: string
  effect?: string
}

export interface NetworkConfig {
  create_service?: boolean
  service_config?: ServiceConfig
  create_ingress?: boolean
  ingress_config?: IngressConfig
}

export interface ServiceConfig {
  name?: string
  type?: string
  ports: ServicePort[]
  annotations?: Record<string, string>
}

export interface ServicePort {
  name?: string
  port: number
  target_port: number
  node_port?: number
  protocol?: string
}

export interface IngressConfig {
  name?: string
  class_name?: string
  annotations?: Record<string, string>
  host: string
  path?: string
  path_type?: string
  tls_secret?: string
}

export interface AdvancedConfig {
  service_account_name?: string
  pod_security_context?: PodSecurityContext
  termination_grace_period_seconds?: number
  dns_policy?: string
  host_network?: boolean
  host_pid?: boolean
  image_pull_secrets?: string[]
  restart_policy?: string
}

export interface PodSecurityContext {
  run_as_user?: number
  run_as_group?: number
  run_as_non_root?: boolean
  fs_group?: number
}

// Deployments
export const getDeployments = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Deployment[] }>(`/clusters/${clusterId}/workloads/deployments`, { params })
}

export const getDeploymentDetail = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: DeploymentDetail }>(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}`)
}

export const createDeployment = (clusterId: number, data: any) => {
  return post(`/clusters/${clusterId}/workloads/deployments`, data)
}

export const createEnterpriseDeployment = (clusterId: number, data: EnterpriseDeploymentRequest) => {
  return post(`/clusters/${clusterId}/workloads/deployments/enterprise`, data)
}

export const updateDeployment = (clusterId: number, namespace: string, name: string, data: any) => {
  return put(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}`, data)
}

export const scaleDeployment = (clusterId: number, namespace: string, name: string, replicas: number) => {
  return post(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}/scale`, { replicas })
}

export const deleteDeployment = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}`)
}

export const getDeploymentHistory = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any }>(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}/history`)
}

export const rollbackDeployment = (clusterId: number, namespace: string, name: string, revision: number) => {
  return post(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}/rollback`, { revision })
}

// Pods
export const getPods = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Pod[] }>(`/clusters/${clusterId}/workloads/pods`, { params })
}

export const createPod = (clusterId: number, data: any) => {
  return post(`/clusters/${clusterId}/workloads/pods`, data)
}

export const getPodLogs = (clusterId: number, namespace: string, name: string, tail?: number) => {
  const params = tail ? { tail: tail.toString() } : {}
  return get<string>(`/clusters/${clusterId}/workloads/pods/${namespace}/${name}/logs`, { params })
}

export const deletePod = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/pods/${namespace}/${name}`)
}

// Services
export const getServices = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Service[] }>(`/clusters/${clusterId}/workloads/services`, { params })
}

export const getServiceDetail = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any }>(`/clusters/${clusterId}/workloads/services/${namespace}/${name}`)
}

export const getDeploymentServices = (clusterId: number, namespace: string, name: string) => {
  return get<{ code: number; data: any[] }>(`/clusters/${clusterId}/workloads/deployments/${namespace}/${name}/services`)
}

export const createService = (clusterId: number, data: any) => {
  return post(`/clusters/${clusterId}/workloads/services`, data)
}

export const updateService = (clusterId: number, namespace: string, name: string, data: any) => {
  return put(`/clusters/${clusterId}/workloads/services/${namespace}/${name}`, data)
}

export const deleteService = (clusterId: number, namespace: string, name: string) => {
  return del(`/clusters/${clusterId}/workloads/services/${namespace}/${name}`)
}

// Nodes
export const getNodes = (clusterId: number) => {
  return get<{ code: number; data: Node[] }>(`/clusters/${clusterId}/workloads/nodes`)
}

// Namespaces
export const getNamespaces = (clusterId: number) => {
  return get<{ code: number; data: string[] }>(`/clusters/${clusterId}/workloads/namespaces`)
}

// Events
export const getEvents = (clusterId: number, namespace?: string) => {
  const params = namespace ? { ns: namespace } : {}
  return get<{ code: number; data: Event[] }>(`/clusters/${clusterId}/workloads/events`, { params })
}
