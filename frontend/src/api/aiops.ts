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
  input_price_per_m: number
  output_price_per_m: number
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
  pending_actions?: Array<{
    id: number
    action_id: number
    action: string
    name: string
    namespace: string
    description: string
    dry_run: string
    need_confirm: boolean
  }>
  tool_trace?: Array<{
    name: string
    args: string
    result: string
    is_error?: boolean
  }>
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
  input_price_per_m?: number
  output_price_per_m?: number
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
  input_price_per_m?: number
  output_price_per_m?: number
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

// Test LLM connection. When editing, pass id and leave api_key empty to reuse stored key.
export const testLLMConfig = (data: {
  id?: number
  provider: string
  api_key?: string
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

// ==================== Token Usage ====================

export interface TokenUsageStats {
  total_tokens: number
  total_prompt_tokens: number
  total_completion_tokens: number
  total_cost_estimate: number
  by_day: Array<{
    date: string
    total_tokens: number
    prompt_tokens: number
    completion_tokens: number
  }>
  by_model: Array<{
    model: string
    total_tokens: number
  }>
  by_user: Array<{
    user_id: number
    username: string
    total_tokens: number
  }>
  by_type: Array<{
    chat_type: string
    total_tokens: number
  }>
}

export interface TokenUsageLog {
  id: number
  user_id: number
  conversation_id: number
  llm_config_id: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  chat_type: string
  created_at: string
}

export const getTokenUsageStats = (days?: number) => {
  return get<{ code: number; data: TokenUsageStats }>(`/aiops/token-usage/stats?days=${days || 30}`)
}

export const getTokenUsageRecent = (limit?: number) => {
  return get<{ code: number; data: TokenUsageLog[] }>(`/aiops/token-usage/recent?limit=${limit || 50}`)
}
