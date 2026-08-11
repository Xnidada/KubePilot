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
  conversation_id?: number
}

export interface StagedActionResult {
  success: boolean
  staged?: boolean
  action_id: number
  status: string
  dry_run: string
  message: string
  confirm_path?: string
  details?: string[]
}

export interface ConfirmActionResult {
  success: boolean
  message: string
  details?: string[]
  action_id: number
  status: string
}

/** Stage a write action and return dry-run preview. Does NOT mutate the cluster. */
export const stageK8SOperation = (data: ExecuteRequest) => {
  return post<{ code: number; data: StagedActionResult }>('/aiops/agent/execute', data)
}

/** Confirm a previously staged pending action and execute it. */
export const confirmK8SOperation = (actionId: number) => {
  return post<{ code: number; data: ConfirmActionResult }>(`/aiops/agent/confirm/${actionId}`)
}

/** @deprecated use stageK8SOperation — execute now only stages */
export const executeK8SOperation = stageK8SOperation
