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
  UserOutlined,
  ThunderboltOutlined,
  StopOutlined,
  DeleteOutlined,
  QuestionCircleOutlined,
  ToolOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { executeK8SOperation, ExecuteRequest } from '../../api/agent'
import { useConversations } from '../../hooks/useConversations'
import ChatSidebar from '../../components/ChatSidebar'
import MarkdownRenderer from '../../components/MarkdownRenderer'

const { Title, Text } = Typography
const { TextArea } = Input

// 检测是否包含确认提示或 action 块
const hasConfirmationPrompt = (text: string): boolean => {
  if (text.includes('```action')) return true
  const keywords = [
    '请确认是否执行',
    '是否执行此操作',
    '确认执行',
    '请确认',
    '确认吗',
  ]
  return keywords.some(kw => text.includes(kw))
}

const AIAgent: React.FC = () => {
  const {
    conversations,
    activeConversation,
    activeId,
    createConversation,
    selectConversation,
    addMessage,
    deleteMessagePair,
    deleteConversation,
    renameConversation,
  } = useConversations()

  const [inputValue, setInputValue] = useState('')
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
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
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
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

  const handleSend = async (content?: string) => {
    const sendContent = content || inputValue.trim()
    if (!sendContent || loading) return

    let currentId = activeId
    if (!currentId) {
      currentId = await createConversation()
      if (!currentId) return
    }

    if (!content) {
      setInputValue('')
    }

    // 保存用户消息到后端
    await addMessage(currentId, 'user', sendContent)

    setLoading(true)

    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      const token = getAuthToken()
      const response = await fetch('/api/v1/aiops/agent', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          message: sendContent,
          cluster_id: selectedCluster,
          conversation_id: currentId,
        }),
        signal: abortController.signal,
      })

      const res = await response.json()
      if (res.code === 0) {
        await addMessage(currentId, 'assistant', res.data.content)
      } else {
        await addMessage(currentId, 'assistant', '❌ 请求失败: ' + (res.message || '未知错误'))
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        console.log('Request aborted')
      } else {
        console.error('Chat error:', error)
        message.error('AI 服务不可用')
        await addMessage(currentId!, 'assistant', '❌ AI 服务不可用，请在 **AI 设置** 中配置 LLM。')
      }
    } finally {
      setLoading(false)
      abortControllerRef.current = null
    }
  }

  // 确认执行操作 - 调用后端 API 真正执行
  const handleConfirm = async () => {
    if (!activeId || !activeConversation) return

    const lastAssistantMsg = [...activeConversation.messages]
      .reverse()
      .find(m => m.role === 'assistant')

    if (!lastAssistantMsg) return

    const content = lastAssistantMsg.content

    // 解析所有 ```action JSON 块
    const actionRegex = /```action\s*\n([\s\S]*?)\n```/g
    const actions: any[] = []
    let match

    while ((match = actionRegex.exec(content)) !== null) {
      try {
        const actionData = JSON.parse(match[1])
        actions.push(actionData)
      } catch (e) {
        console.error('Failed to parse action:', e)
      }
    }

    if (actions.length === 0) {
      await handleSend('确认执行以上操作')
      return
    }

    // 执行所有操作
    setLoading(true)
    await addMessage(activeId, 'user', '确认执行')

    const results: string[] = []

    for (const action of actions) {
      try {
        const request: ExecuteRequest = {
          cluster_id: selectedCluster,
          action: action.action,
          name: action.name || action.resource_name,
          namespace: action.namespace || 'default',
          image: action.image || 'nginx:latest',
          replicas: action.replicas || 1,
          ports: action.ports || (action.container_port ? [action.container_port] : []),
          service_type: action.service_type || action.type || 'ClusterIP',
          port: action.port || 80,
          target_port: action.target_port || action.container_port || 80,
          node_port: action.node_port || action.nodePort,
          selector: action.selector || (action.name ? { app: action.name } : {}),
        }

        const res = await executeK8SOperation(request)
        if (res.code === 0 && res.data) {
          const details = res.data.details ? '\n' + res.data.details.map(d => `  - ${d}`).join('\n') : ''
          results.push(`✅ ${res.data.message}${details}`)
        } else {
          results.push(`❌ ${action.action} ${action.name}: 执行失败`)
        }
      } catch (error: any) {
        results.push(`❌ ${action.action} ${action.name}: ${error.message || '执行失败'}`)
      }
    }

    await addMessage(activeId, 'assistant', results.join('\n\n'))
    setLoading(false)
  }

  const handleCancel = async () => {
    let currentId = activeId
    if (!currentId) {
      currentId = await createConversation()
      if (!currentId) return
    }
    await addMessage(currentId, 'assistant', '❌ 操作已取消')
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
    const needsConfirm = !isUser && hasConfirmationPrompt(msg.content)

    return (
      <div
        key={msg.id || index}
        style={{
          display: 'flex',
          justifyContent: isUser ? 'flex-end' : 'flex-start',
          marginBottom: 24,
          padding: '0 16px',
          position: 'relative',
        }}
      >
        {!isUser && (
          <Avatar
            icon={<ThunderboltOutlined />}
            style={{ backgroundColor: '#722ed1', marginRight: 12, flexShrink: 0 }}
          />
        )}
        <div style={{ maxWidth: '75%', position: 'relative' }}>
          <div
            style={{
              padding: '12px 16px',
              borderRadius: 12,
              backgroundColor: isUser ? '#722ed1' : '#f0f2f5',
              color: isUser ? '#fff' : '#333',
              boxShadow: '0 1px 2px rgba(0,0,0,0.1)',
            }}
          >
            {isEmpty ? (
              <Spin size="small" />
            ) : isUser ? (
              <div style={{ whiteSpace: 'pre-wrap' }}>{msg.content}</div>
            ) : (
              <div className="markdown-body">
                <MarkdownRenderer content={msg.content} />
              </div>
            )}

            {/* 确认/取消按钮 */}
            {needsConfirm && (
              <div style={{ marginTop: 12, paddingTop: 12, borderTop: '1px solid #e5e5e5' }}>
                <Space>
                  <Button
                    type="primary"
                    size="small"
                    icon={<CheckCircleOutlined />}
                    onClick={handleConfirm}
                  >
                    确认执行
                  </Button>
                  <Button
                    danger
                    size="small"
                    icon={<CloseCircleOutlined />}
                    onClick={handleCancel}
                  >
                    取消
                  </Button>
                </Space>
              </div>
            )}

            <div
              style={{
                textAlign: 'right',
                fontSize: 11,
                opacity: 0.7,
                marginTop: 8,
                color: isUser ? '#fff' : '#999',
              }}
            >
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
        collapsed={sidebarCollapsed}
        onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
      />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '12px 24px', borderBottom: '1px solid #e5e5e5', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#fff' }}>
          <Space>
            <ThunderboltOutlined style={{ color: '#722ed1', fontSize: 20 }} />
            <Title level={5} style={{ margin: 0 }}>AI Agent</Title>
            <Tooltip title="AI Agent 可以理解自然语言并执行 K8S 操作">
              <QuestionCircleOutlined style={{ color: '#999' }} />
            </Tooltip>
          </Space>
          <Space>
            <Select
              value={selectedCluster}
              onChange={setSelectedCluster}
              style={{ width: 200 }}
              placeholder="选择集群"
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
            />
          </Space>
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 0', background: '#fff' }}>
          {(!activeConversation || activeConversation.messages.length === 0) ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#999' }}>
              <ToolOutlined style={{ fontSize: 64, marginBottom: 24, color: '#d9d9d9' }} />
              <Title level={4} style={{ color: '#666' }}>AI Agent</Title>
              <Text type="secondary" style={{ textAlign: 'center', maxWidth: 400 }}>
                使用自然语言描述你想做的操作，AI 会自动执行 K8S 命令
              </Text>
              <div style={{ marginTop: 24, textAlign: 'left' }}>
                <Text type="secondary">示例：</Text>
                <ul style={{ color: '#999', marginTop: 8 }}>
                  <li>帮我创建一个 nginx deployment，3个副本</li>
                  <li>查看 default 命名空间的 service</li>
                  <li>删除 test 命名空间的所有 pod</li>
                </ul>
              </div>
            </div>
          ) : (
            activeConversation.messages.map((msg, index) => renderMessage(msg, index))
          )}
          {loading && (
            <div style={{ display: 'flex', alignItems: 'center', padding: '0 16px', marginBottom: 24 }}>
              <Avatar icon={<ThunderboltOutlined />} style={{ backgroundColor: '#722ed1', marginRight: 12 }} />
              <Spin size="small" />
              <Text type="secondary" style={{ marginLeft: 8 }}>AI Agent 思考中...</Text>
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
              placeholder="描述你想做的操作... (Enter 发送，Shift+Enter 换行)"
              autoSize={{ minRows: 1, maxRows: 4 }}
              disabled={loading}
              style={{ flex: 1 }}
            />
            {loading ? (
              <Button danger icon={<StopOutlined />} onClick={handleStop} style={{ height: 'auto' }}>
                停止
              </Button>
            ) : (
              <Button type="primary" icon={<SendOutlined />} onClick={() => handleSend()} disabled={!inputValue.trim()} style={{ height: 'auto', backgroundColor: '#722ed1', borderColor: '#722ed1' }}>
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

export default AIAgent
