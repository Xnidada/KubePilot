import { post } from './request'

export interface ExecuteRequest {
  cluster_id: number
  action: string
  namespace?: string
  name: string
  image?: string
  replicas?: number
  ports?: number[]
  service_type?: string
  port?: number
  target_port?: number
  node_port?: number
  selector?: Record<string, string>
}

export interface ExecuteResult {
  success: boolean
  message: string
  details?: string[]
}

// 执行 K8S 操作
export const executeK8SOperation = (data: ExecuteRequest) => {
  return post<{ code: number; data: ExecuteResult }>('/aiops/agent/execute', data)
}

// 解析用户意图并执行
export const parseAndExecute = (clusterId: number, message: string) => {
  return post<{ code: number; data: ExecuteResult }>('/aiops/agent/execute', {
    cluster_id: clusterId,
    action: 'parse',
    name: message,
  })
}
