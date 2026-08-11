import { post } from './request'

export interface AgentChatRequest {
  cluster_id: number
  conversation_id?: number
  message: string
  context?: any
}

export interface PendingAction {
  id: number
  action_id: number
  action: string
  name: string
  namespace: string
  description: string
  dry_run: string
  need_confirm: boolean
}

export interface ToolTraceItem {
  name: string
  args: string
  result: string
  is_error?: boolean
}

export interface AgentChatResponse {
  content: string
  actions?: any[]
  pending_actions?: PendingAction[]
  tool_trace?: ToolTraceItem[]
}

export interface ExecuteRequest {
  cluster_id: number
  action: string
  name: string
  namespace: string
  image?: string
  replicas?: number
  ports?: number[]
  service_type?: string
  port?: number
  target_port?: number
  node_port?: number
  selector?: Record<string, string>
  yaml?: string
  conversation_id?: number
}

export interface StageResponse {
  action_id: number
  dry_run: string
  status: string
}

export interface ConfirmResponse {
  success: boolean
  message: string
  result?: any
}

export interface ApiResult<T> {
  code: number
  data: T
  message: string
}

// AI Agent 对话
export const agentChat = (data: AgentChatRequest) => {
  return post<ApiResult<AgentChatResponse>>('/aiops/agent', data)
}

/** 暂存写操作并 dry-run（旧路径；新 Agent 由后端 stage_mutation 直接返回 action_id） */
export const stageK8SOperation = (data: ExecuteRequest) => {
  return post<ApiResult<StageResponse>>('/aiops/agent/execute', data)
}

/** 确认执行已暂存操作 */
export const confirmK8SOperation = (actionId: number) => {
  return post<ApiResult<ConfirmResponse>>(`/aiops/agent/confirm/${actionId}`)
}
