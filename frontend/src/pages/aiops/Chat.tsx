import { useState, useRef, useEffect } from 'react'
import {
  Input,
  Button,
  Space,
  Typography,
  Avatar,
  Spin,
  message,
  Select,
  Tooltip,
} from 'antd'
import {
  SendOutlined,
  RobotOutlined,
  UserOutlined,
  ClearOutlined,
  StopOutlined,
  DeleteOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { useConversations } from '../../hooks/useConversations'
import ChatSidebar from '../../components/ChatSidebar'
import MarkdownRenderer from '../../components/MarkdownRenderer'

const { Title, Text } = Typography
const { TextArea } = Input

const AIChat: React.FC = () => {
  const {
    conversations,
    activeConversation,
    activeId,
    createConversation,
    selectConversation,
    addMessage,
    updateLastMessage,
    deleteMessagePair,
    deleteConversation,
    renameConversation,
    clearConversation,
  } = useConversations()

  const [inputValue, setInputValue] = useState('')
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [activeConversation?.messages])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data && res.data.length > 0) {
        setSelectedCluster(res.data[0].id)
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const scrollToBottom = () => {
    setTimeout(() => {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }, 100)
  }

  const handleStop = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
      setLoading(false)
      message.info('已停止生成')
    }
  }

  const handleSend = async () => {
    if (!inputValue.trim() || loading) return

    let currentId = activeId
    if (!currentId) {
      currentId = await createConversation()
      if (!currentId) return
    }

    const userContent = inputValue.trim()
    setInputValue('')

    await addMessage(currentId, 'user', userContent)
    setLoading(true)

    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      const token = getAuthToken()
      const response = await fetch('/api/v1/aiops/chat/stream', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          message: userContent,
          cluster_id: selectedCluster,
          conversation_id: currentId,
        }),
        signal: abortController.signal,
      })

      if (!response.ok) throw new Error('Stream request failed')

      const reader = response.body?.getReader()
      if (!reader) throw new Error('No reader available')

      const decoder = new TextDecoder()
      let buffer = ''
      let fullContent = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const parts = buffer.split('\n\n')
        buffer = parts.pop() || ''

        for (const part of parts) {
          const lines = part.split('\n')
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const jsonStr = line.slice(6).trim()
              if (jsonStr === '[DONE]') continue
              try {
                const data = JSON.parse(jsonStr)
                if (data.content) {
                  fullContent += data.content
                  updateLastMessage(currentId!, fullContent)
                }
              } catch {}
            }
          }
        }
      }

      // 处理缓冲区剩余
      if (buffer.trim()) {
        const lines = buffer.split('\n')
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const jsonStr = line.slice(6).trim()
            if (jsonStr && jsonStr !== '[DONE]') {
              try {
                const data = JSON.parse(jsonStr)
                if (data.content) fullContent += data.content
              } catch {}
            }
          }
        }
      }

      if (fullContent) {
        await addMessage(currentId!, 'assistant', fullContent)
      }
    } catch (error: any) {
      if (error.name !== 'AbortError') {
        console.error('Chat error:', error)
        message.error('AI 对话失败')
        await addMessage(currentId!, 'assistant', '❌ AI 服务不可用')
      }
    } finally {
      setLoading(false)
      abortControllerRef.current = null
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const renderMessage = (msg: any, index: number) => {
    const isUser = msg.role === 'user'
    const isEmpty = !msg.content && !isUser

    return (
      <div
        key={msg.id || index}
        style={{ display: 'flex', justifyContent: isUser ? 'flex-end' : 'flex-start', marginBottom: 24, padding: '0 16px', position: 'relative' }}
      >
        {!isUser && (
          <Avatar icon={<RobotOutlined />} style={{ backgroundColor: '#1890ff', marginRight: 12, flexShrink: 0 }} />
        )}
        <div style={{ maxWidth: '75%', position: 'relative' }}>
          <div style={{ padding: '12px 16px', borderRadius: 12, backgroundColor: isUser ? '#1890ff' : '#f0f2f5', color: isUser ? '#fff' : '#333', boxShadow: '0 1px 2px rgba(0,0,0,0.1)' }}>
            {isEmpty ? (
              <Spin size="small" />
            ) : isUser ? (
              <div style={{ whiteSpace: 'pre-wrap' }}>{msg.content}</div>
            ) : (
              <div className="markdown-body">
                <MarkdownRenderer content={msg.content} />
              </div>
            )}
            <div style={{ textAlign: 'right', fontSize: 11, opacity: 0.7, marginTop: 8, color: isUser ? '#fff' : '#999' }}>
              {new Date(msg.created_at).toLocaleTimeString()}
            </div>
          </div>
          <Tooltip title="删除此对话">
            <Button
              type="text"
              size="small"
              icon={<DeleteOutlined />}
              onClick={() => activeId && deleteMessagePair(activeId, msg.id)}
              style={{ position: 'absolute', top: -8, right: isUser ? 'auto' : -8, left: isUser ? -8 : 'auto', opacity: 0.5, fontSize: 12 }}
            />
          </Tooltip>
        </div>
        {isUser && (
          <Avatar icon={<UserOutlined />} style={{ backgroundColor: '#87d068', marginLeft: 12, flexShrink: 0 }} />
        )}
      </div>
    )
  }

  const sidebarConversations = conversations.map(c => ({
    id: String(c.id),
    title: c.title,
    createdAt: new Date(c.created_at),
    updatedAt: new Date(c.updated_at),
    messageCount: c.message_count,
  }))

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 180px)', background: '#fff', borderRadius: 8, overflow: 'hidden' }}>
      <ChatSidebar
        conversations={sidebarConversations}
        activeId={activeId ? String(activeId) : null}
        onSelect={(id) => selectConversation(Number(id))}
        onCreate={() => createConversation()}
        onDelete={(id) => deleteConversation(Number(id))}
        onRename={(id, title) => renameConversation(Number(id), title)}
      />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '12px 24px', borderBottom: '1px solid #e5e5e5', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#fff' }}>
          <Text strong style={{ fontSize: 16 }}>
            {activeConversation?.title || 'AI 对话'}
          </Text>
          <Space>
            <Select
              value={selectedCluster}
              onChange={setSelectedCluster}
              style={{ width: 200 }}
              placeholder="选择集群上下文"
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
            />
            <Tooltip title="清空当前对话">
              <Button icon={<ClearOutlined />} onClick={() => activeId && clearConversation(activeId)}>
                清空
              </Button>
            </Tooltip>
          </Space>
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 0', background: '#fff' }}>
          {(!activeConversation || activeConversation.messages.length === 0) ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#999' }}>
              <RobotOutlined style={{ fontSize: 64, marginBottom: 24, color: '#d9d9d9' }} />
              <Title level={4} style={{ color: '#666' }}>开始新的对话</Title>
              <Text type="secondary">输入问题开始与 AI 助手交流</Text>
            </div>
          ) : (
            activeConversation.messages.map((msg, index) => renderMessage(msg, index))
          )}
          {loading && (
            <div style={{ display: 'flex', alignItems: 'center', padding: '0 16px', marginBottom: 24 }}>
              <Avatar icon={<RobotOutlined />} style={{ backgroundColor: '#1890ff', marginRight: 12 }} />
              <Spin size="small" />
              <Text type="secondary" style={{ marginLeft: 8 }}>AI 思考中...</Text>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        <div style={{ padding: '16px 24px', borderTop: '1px solid #e5e5e5', background: '#fff' }}>
          <div style={{ display: 'flex', gap: 8 }}>
            <TextArea
              value={inputValue}
              onChange={e => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入消息... (Enter 发送，Shift+Enter 换行)"
              autoSize={{ minRows: 1, maxRows: 4 }}
              disabled={loading}
              style={{ flex: 1 }}
            />
            {loading ? (
              <Button danger icon={<StopOutlined />} onClick={handleStop} style={{ height: 'auto' }}>
                停止
              </Button>
            ) : (
              <Button type="primary" icon={<SendOutlined />} onClick={handleSend} disabled={!inputValue.trim()} style={{ height: 'auto' }}>
                发送
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function getAuthToken(): string {
  const token = localStorage.getItem('auth-storage')
  if (token) {
    try {
      const authData = JSON.parse(token)
      return authData?.state?.token || ''
    } catch { return '' }
  }
  return ''
}

export default AIChat
