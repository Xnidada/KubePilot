import { get, post, put, del } from './request'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface ChatResponse {
  content: string
  usage: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface DiagnosisRequest {
  cluster_id: number
  resource_type: string
  resource_name: string
  namespace?: string
  problem: string
}

export interface DiagnosisResponse {
  analysis: string
  steps: string[]
  solutions: string[]
  prevention: string[]
  commands: string[]
}

export interface LLMConfig {
  id: number
  provider: string
  api_key: string
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  timeout: number
  is_active: boolean
  created_at: string
}

export interface AgentAction {
  id: number
  action_type: string
  resource_type: string
  resource_name: string
  namespace: string
  description: string
  need_confirm: boolean
}

export interface AgentChatResponse {
  content: string
  actions?: AgentAction[]
}

// Chat API
export const chat = (data: {
  message: string
  cluster_id?: number
  context?: string
}) => {
  return post<{ code: number; data: ChatResponse }>('/aiops/chat', data)
}

// Stream Chat API
export const chatStream = async (data: {
  message: string
  cluster_id?: number
  context?: string
}): Promise<Response> => {
  const token = getAuthToken()
  const response = await fetch('/api/v1/aiops/chat/stream', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify(data),
  })
  return response
}

// Clear chat history
export const clearChatHistory = () => {
  return del('/aiops/chat/history')
}

// Diagnose resource
export const diagnoseResource = (data: DiagnosisRequest) => {
  return post<{ code: number; data: DiagnosisResponse }>('/aiops/diagnose', data)
}

// ==================== LLM Config ====================

// List all LLM configs
export const listLLMConfigs = () => {
  return get<{ code: number; data: LLMConfig[] }>('/aiops/configs')
}

// Get default LLM config
export const getLLMConfig = () => {
  return get<{ code: number; data: any }>('/aiops/configs/default')
}

// Get LLM config by ID
export const getLLMConfigByID = (id: number) => {
  return get<{ code: number; data: LLMConfig }>(`/aiops/configs/${id}`)
}

// Save LLM config
export const saveLLMConfig = (data: {
  provider: string
  api_key: string
  base_url?: string
  model: string
  temperature?: number
  max_tokens?: number
  timeout?: number
}) => {
  return post<{ code: number; data: any }>('/aiops/configs', data)
}

// Update LLM config
export const updateLLMConfig = (id: number, data: {
  api_key?: string
  base_url?: string
  model?: string
  temperature?: number
  max_tokens?: number
  timeout?: number
}) => {
  return put<{ code: number; data: any }>(`/aiops/configs/${id}`, data)
}

// Delete LLM config
export const deleteLLMConfig = (id: number) => {
  return del(`/aiops/configs/${id}`)
}

// Set default LLM config
export const setDefaultLLMConfig = (id: number) => {
  return post(`/aiops/configs/${id}/set-default`)
}

// Test LLM connection
export const testLLMConfig = (data: {
  provider: string
  api_key: string
  base_url?: string
  model: string
}) => {
  return post<{ code: number; data: any }>('/aiops/configs/test', data)
}

// ==================== AI Agent ====================

// Agent chat
export const agentChat = (data: {
  message: string
  cluster_id: number
}) => {
  return post<{ code: number; data: AgentChatResponse }>('/aiops/agent', data)
}

// Confirm agent action
export const confirmAgentAction = (actionId: number) => {
  return post(`/aiops/agent/confirm/${actionId}`)
}

// Helper function to get auth token
function getAuthToken(): string {
  const token = localStorage.getItem('auth-storage')
  if (token) {
    try {
      const authData = JSON.parse(token)
      return authData?.state?.token || ''
    } catch {
      return ''
    }
  }
  return ''
}

// ==================== AI 驱动功能 ====================

export interface ExplainResponse {
  explanation: string
  examples?: string
  references?: string
}

export interface ResourceGuideResponse {
  overview: string
  status: string
  health_score: number
  suggestions: string[]
  operations: string[]
  warnings: string[]
}

export interface TranslateYAMLResponse {
  translated: string
  notes?: string
}

export interface AnalyzeLogsResponse {
  summary: string
  patterns: string[]
  errors: string[]
  root_cause: string
  solutions: string[]
  commands: string[]
  severity: string
}

// 划词解释
export const explainText = (data: {
  text: string
  cluster_id?: number
  context?: string
}) => {
  return post<{ code: number; data: ExplainResponse }>('/aiops/explain', data)
}

// 流式划词解释
export const explainTextStream = async (data: {
  text: string
  cluster_id?: number
  context?: string
}): Promise<Response> => {
  const token = getAuthToken()
  const response = await fetch('/api/v1/aiops/explain/stream', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify(data),
  })
  return response
}

// 资源指南
export const getResourceGuide = (data: {
  cluster_id: number
  resource_type: string
  resource_name?: string
  namespace?: string
}) => {
  return post<{ code: number; data: ResourceGuideResponse }>('/aiops/resource-guide', data)
}

// YAML 翻译
export const translateYAML = (data: {
  yaml: string
  direction?: string
}) => {
  return post<{ code: number; data: TranslateYAMLResponse }>('/aiops/translate-yaml', data)
}

// 日志问诊
export const analyzeLogs = (data: {
  cluster_id: number
  resource_type?: string
  resource_name: string
  namespace: string
  container?: string
  lines?: number
  logs?: string
}) => {
  return post<{ code: number; data: AnalyzeLogsResponse }>('/aiops/analyze-logs', data)
}
