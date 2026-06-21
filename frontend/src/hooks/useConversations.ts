import { useState, useEffect, useCallback } from 'react'
import * as conversationApi from '../api/conversation'

export interface Message {
  id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  created_at: string
}

export interface Conversation {
  id: number
  title: string
  cluster_id: number | null
  message_count: number
  messages: Message[]
  created_at: string
  updated_at: string
}

export function useConversations() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeId, setActiveId] = useState<number | null>(null)
  const [activeConversation, setActiveConversation] = useState<Conversation | null>(null)

  // 加载对话列表
  const fetchConversations = useCallback(async () => {
    try {
      const res = await conversationApi.getConversations()
      if (res.code === 0) {
        setConversations((res.data || []).map((c: any) => ({
          ...c,
          messages: [],
        })))
      }
    } catch (error) {
      console.error('Failed to fetch conversations:', error)
    }
  }, [])

  // 加载对话详情（包含消息）
  const fetchConversationDetail = useCallback(async (id: number) => {
    try {
      const res = await conversationApi.getConversation(id)
      if (res.code === 0) {
        setActiveConversation(res.data as Conversation)
      }
    } catch (error) {
      console.error('Failed to fetch conversation:', error)
    }
  }, [])

  // 初始化加载对话列表
  useEffect(() => {
    fetchConversations()
  }, [fetchConversations])

  // 当 activeId 变化时加载对话详情
  useEffect(() => {
    if (activeId) {
      fetchConversationDetail(activeId)
    } else {
      setActiveConversation(null)
    }
  }, [activeId, fetchConversationDetail])

  // 创建新对话
  const createConversation = useCallback(async (title?: string) => {
    try {
      const res = await conversationApi.createConversation({ title: title || '新对话' })
      if (res.code === 0) {
        await fetchConversations()
        setActiveId(res.data.id)
        return res.data.id
      }
    } catch (error) {
      console.error('Failed to create conversation:', error)
    }
    return null
  }, [fetchConversations])

  // 选择对话
  const selectConversation = useCallback((id: number) => {
    setActiveId(id)
  }, [])

  // 添加消息
  const addMessage = useCallback(async (conversationId: number, role: 'user' | 'assistant', content: string) => {
    try {
      const res = await conversationApi.addMessage(conversationId, { role, content })
      if (res.code === 0) {
        await fetchConversationDetail(conversationId)
        await fetchConversations()
        return res.data
      }
    } catch (error) {
      console.error('Failed to add message:', error)
    }
    return null
  }, [fetchConversationDetail, fetchConversations])

  // 更新最后一条消息（用于流式输出，直接更新本地状态）
  const updateLastMessage = useCallback((conversationId: number, content: string) => {
    setActiveConversation(prev => {
      if (!prev || prev.id !== conversationId) return prev
      const messages = [...prev.messages]
      if (messages.length > 0) {
        messages[messages.length - 1] = {
          ...messages[messages.length - 1],
          content,
        }
      }
      return { ...prev, messages }
    })
  }, [])

  // 删除消息对
  const deleteMessagePair = useCallback(async (conversationId: number, messageId: number) => {
    try {
      await conversationApi.deleteMessage(conversationId, messageId)
      await fetchConversationDetail(conversationId)
    } catch (error) {
      console.error('Failed to delete message:', error)
    }
  }, [fetchConversationDetail])

  // 清空对话
  const clearConversation = useCallback(async (id: number) => {
    try {
      await conversationApi.clearConversation(id)
      await fetchConversationDetail(id)
    } catch (error) {
      console.error('Failed to clear conversation:', error)
    }
  }, [fetchConversationDetail])

  // 删除对话
  const deleteConversation = useCallback(async (id: number) => {
    try {
      await conversationApi.deleteConversation(id)
      if (activeId === id) {
        setActiveId(null)
      }
      await fetchConversations()
    } catch (error) {
      console.error('Failed to delete conversation:', error)
    }
  }, [activeId, fetchConversations])

  // 重命名对话
  const renameConversation = useCallback(async (id: number, title: string) => {
    try {
      await conversationApi.updateConversation(id, { title })
      await fetchConversations()
    } catch (error) {
      console.error('Failed to rename conversation:', error)
    }
  }, [fetchConversations])

  return {
    conversations,
    activeConversation,
    activeId,
    createConversation,
    selectConversation,
    addMessage,
    updateLastMessage,
    deleteMessagePair,
    clearConversation,
    deleteConversation,
    renameConversation,
    fetchConversations,
    fetchConversationDetail,
  }
}
