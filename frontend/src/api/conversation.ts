import { get, post, put, del } from './request'

export interface Conversation {
  id: number
  title: string
  cluster_id: number | null
  message_count: number
  created_at: string
  updated_at: string
}

export interface Message {
  id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  created_at: string
}

// Conversations
export const getConversations = () => {
  return get<{ code: number; data: Conversation[] }>('/aiops/conversations')
}

export const createConversation = (data?: {
  title?: string
  cluster_id?: number
}) => {
  return post<{ code: number; data: Conversation }>('/aiops/conversations', data || {})
}

export const getConversation = (id: number) => {
  return get<{ code: number; data: { id: number; title: string; messages: Message[] } }>(`/aiops/conversations/${id}`)
}

export const updateConversation = (id: number, data: { title?: string; cluster_id?: number }) => {
  return put(`/aiops/conversations/${id}`, data)
}

export const deleteConversation = (id: number) => {
  return del(`/aiops/conversations/${id}`)
}

export const clearConversation = (id: number) => {
  return post(`/aiops/conversations/${id}/clear`)
}

// Messages
export const getMessages = (conversationId: number, page = 1, size = 100) => {
  return get<{ code: number; data: Message[]; total: number }>(
    `/aiops/conversations/${conversationId}/messages`,
    { params: { page, size } }
  )
}

export const addMessage = (conversationId: number, data: { role: string; content: string }) => {
  return post<{ code: number; data: Message }>(
    `/aiops/conversations/${conversationId}/messages`,
    data
  )
}

export const deleteMessage = (conversationId: number, messageId: number) => {
  return del(`/aiops/conversations/${conversationId}/messages/${messageId}`)
}
