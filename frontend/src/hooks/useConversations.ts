import { useState, useEffect, useCallback } from 'react'
import { message } from 'antd'
import * as conversationApi from '../api/conversation'

export interface Message {
  id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  extras?: string
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
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [batchDeleting, setBatchDeleting] = useState(false)
  const [messageBatchDeleting, setMessageBatchDeleting] = useState(false)

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

  useEffect(() => {
    fetchConversations()
  }, [fetchConversations])

  useEffect(() => {
    if (activeId) {
      fetchConversationDetail(activeId)
    } else {
      setActiveConversation(null)
    }
  }, [activeId, fetchConversationDetail])

  const createConversation = useCallback(async (title?: string) => {
    try {
      const res = await conversationApi.createConversation({ title: title || '新对话' })
      if (res.code === 0) {
        await fetchConversations()
        setActiveId(res.data.id)
        return res.data.id as number
      }
      message.error((res as any).message || '创建会话失败')
    } catch (error: any) {
      console.error('Failed to create conversation:', error)
      message.error(error?.message || '创建会话失败')
    }
    return null
  }, [fetchConversations])

  const selectConversation = useCallback((id: number) => {
    setActiveId(id)
  }, [])

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

  // 删除一轮对话：用户消息 + 紧邻的助手回复（或反过来）
  const deleteMessagePair = useCallback(async (conversationId: number, messageId: number) => {
    const msgs = activeConversation?.id === conversationId ? activeConversation.messages : []
    const idx = msgs.findIndex(m => m.id === messageId)
    const ids = new Set<number>([messageId])

    if (idx >= 0) {
      const cur = msgs[idx]
      if (cur.role === 'user') {
        const next = msgs[idx + 1]
        if (next?.role === 'assistant') ids.add(next.id)
      } else if (cur.role === 'assistant') {
        const prev = msgs[idx - 1]
        if (prev?.role === 'user') ids.add(prev.id)
      }
    }

    try {
      for (const id of ids) {
        await conversationApi.deleteMessage(conversationId, id)
      }
      await fetchConversationDetail(conversationId)
      await fetchConversations()
      message.success(ids.size > 1 ? '已删除该轮对话' : '已删除消息')
      return true
    } catch (error: any) {
      console.error('Failed to delete message:', error)
      message.error(error?.message || '删除消息失败')
      return false
    }
  }, [activeConversation, fetchConversationDetail, fetchConversations])

  const deleteMessages = useCallback(async (conversationId: number, messageIds: number[]) => {
    const uniqueIds = Array.from(new Set(messageIds.filter(Boolean)))
    if (uniqueIds.length === 0) return false

    setMessageBatchDeleting(true)
    let success = 0
    let failed = 0
    try {
      for (const id of uniqueIds) {
        try {
          await conversationApi.deleteMessage(conversationId, id)
          success += 1
        } catch (error) {
          console.error('Failed to delete message:', id, error)
          failed += 1
        }
      }

      await fetchConversationDetail(conversationId)
      await fetchConversations()

      if (failed === 0) {
        message.success(`已删除 ${success} 条消息`)
        return true
      }
      message.warning(`删除完成：成功 ${success}，失败 ${failed}`)
      return success > 0
    } finally {
      setMessageBatchDeleting(false)
    }
  }, [fetchConversationDetail, fetchConversations])

  const clearConversation = useCallback(async (id: number) => {
    try {
      await conversationApi.clearConversation(id)
      if (activeId === id) {
        await fetchConversationDetail(id)
      }
      await fetchConversations()
      message.success('已清空会话消息')
      return true
    } catch (error: any) {
      console.error('Failed to clear conversation:', error)
      message.error(error?.message || '清空失败')
      return false
    }
  }, [activeId, fetchConversationDetail, fetchConversations])

  const deleteConversation = useCallback(async (id: number) => {
    setDeletingId(id)
    try {
      await conversationApi.deleteConversation(id)

      const remaining = conversations.filter(c => c.id !== id)
      setConversations(remaining)

      if (activeId === id) {
        const next = remaining[0]
        setActiveId(next ? next.id : null)
        if (!next) setActiveConversation(null)
      }

      message.success('会话已删除')
      void fetchConversations()
      return true
    } catch (error: any) {
      console.error('Failed to delete conversation:', error)
      message.error(error?.message || '删除会话失败')
      return false
    } finally {
      setDeletingId(null)
    }
  }, [activeId, conversations, fetchConversations])

  const deleteConversations = useCallback(async (ids: number[]) => {
    const uniqueIds = Array.from(new Set(ids.filter(Boolean)))
    if (uniqueIds.length === 0) return false

    setBatchDeleting(true)
    let success = 0
    let failed = 0
    const deleted = new Set<number>()
    try {
      for (const id of uniqueIds) {
        try {
          await conversationApi.deleteConversation(id)
          deleted.add(id)
          success += 1
        } catch (error) {
          console.error('Failed to delete conversation:', id, error)
          failed += 1
        }
      }

      const remaining = conversations.filter(c => !deleted.has(c.id))
      setConversations(remaining)

      if (activeId && deleted.has(activeId)) {
        const next = remaining[0]
        setActiveId(next ? next.id : null)
        if (!next) setActiveConversation(null)
      }

      await fetchConversations()

      if (failed === 0) {
        message.success(`已删除 ${success} 个会话`)
        return true
      }
      message.warning(`删除完成：成功 ${success}，失败 ${failed}`)
      return success > 0
    } finally {
      setBatchDeleting(false)
    }
  }, [activeId, conversations, fetchConversations])

  const renameConversation = useCallback(async (id: number, title: string) => {
    try {
      await conversationApi.updateConversation(id, { title })
      setConversations(prev => prev.map(c => (c.id === id ? { ...c, title } : c)))
      if (activeConversation?.id === id) {
        setActiveConversation(prev => (prev ? { ...prev, title } : prev))
      }
      message.success('已重命名')
      return true
    } catch (error: any) {
      console.error('Failed to rename conversation:', error)
      message.error(error?.message || '重命名失败')
      return false
    }
  }, [activeConversation])

  return {
    conversations,
    activeConversation,
    activeId,
    deletingId,
    batchDeleting,
    messageBatchDeleting,
    createConversation,
    selectConversation,
    addMessage,
    updateLastMessage,
    deleteMessagePair,
    deleteMessages,
    clearConversation,
    deleteConversation,
    deleteConversations,
    renameConversation,
    fetchConversations,
    fetchConversationDetail,
  }
}
