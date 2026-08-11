import { get, post } from './request'

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
  duration_ms?: number
}

export interface AgentChatResponse {
  content: string
  actions?: any[]
  pending_actions?: PendingAction[]
  tool_trace?: ToolTraceItem[]
}

export interface MessageExtras {
  tool_trace?: ToolTraceItem[]
  pending_action_ids?: number[]
}

export interface AgentStreamEvent {
  type: 'status' | 'tool_start' | 'tool_result' | 'content_delta' | 'done' | 'error'
  status?: string
  name?: string
  args?: string
  result?: string
  is_error?: boolean
  delta?: string
  content?: string
  message?: string
  pending_actions?: PendingAction[]
  tool_trace?: ToolTraceItem[]
  message_id?: number
  pending?: PendingAction
  duration_ms?: number
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

export const agentChat = (data: AgentChatRequest) => {
  return post<ApiResult<AgentChatResponse>>('/aiops/agent', data)
}

export const stageK8SOperation = (data: ExecuteRequest) => {
  return post<ApiResult<StageResponse>>('/aiops/agent/execute', data)
}

export const confirmK8SOperation = (actionId: number) => {
  return post<ApiResult<ConfirmResponse>>(`/aiops/agent/confirm/${actionId}`)
}

export const listPendingActions = (conversationId: number) => {
  return get<ApiResult<{ pending_actions: PendingAction[] }>>(
    `/aiops/agent/pending?conversation_id=${conversationId}`
  )
}

export const cancelPendingActions = (conversationId: number, actionIds?: number[]) => {
  return post<ApiResult<null>>('/aiops/agent/pending/cancel', {
    conversation_id: conversationId,
    action_ids: actionIds || [],
  })
}

function getAuthToken(): string {
  const raw = localStorage.getItem('auth-storage')
  if (!raw) return ''
  try {
    const authData = JSON.parse(raw)
    return authData?.state?.token || ''
  } catch {
    return ''
  }
}

/** Consume Agent SSE stream; calls onEvent for each parsed event. */
export async function agentChatStream(
  data: AgentChatRequest,
  onEvent: (ev: AgentStreamEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch('/api/v1/aiops/agent/stream', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${getAuthToken()}`,
    },
    body: JSON.stringify(data),
    signal,
  })

  if (!response.ok) {
    const text = await response.text()
    let msg = text || `HTTP ${response.status}`
    try {
      const j = JSON.parse(text)
      msg = j.message || msg
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }

  const reader = response.body?.getReader()
  if (!reader) throw new Error('无法读取流式响应')

  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const parts = buffer.split('\n\n')
    buffer = parts.pop() || ''
    for (const part of parts) {
      const line = part
        .split('\n')
        .map((l) => l.trim())
        .find((l) => l.startsWith('data:'))
      if (!line) continue
      const raw = line.replace(/^data:\s*/, '')
      if (!raw || raw === '[DONE]') continue
      try {
        const ev = JSON.parse(raw) as AgentStreamEvent
        onEvent(ev)
      } catch {
        /* skip malformed */
      }
    }
  }
}
