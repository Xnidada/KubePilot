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
  Segmented,
} from 'antd'
import {
  SendOutlined,
  UserOutlined,
  ThunderboltOutlined,
  StopOutlined,
  DeleteOutlined,
  RobotOutlined,
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

type ChatMode = 'chat' | 'agent'

// 检测是否包含确认提示或 action 块
const hasConfirmationPrompt = (text: string): boolean => {
  // 检测 action JSON 块
  if (text.includes('```action')) return true
  // 检测确认关键词
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
    updateLastMessage,
    deleteMessagePair,
    deleteConversation,
    renameConversation,
  } = useConversations()

  const [inputValue, setInputValue] = useState('')
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [chatMode, setChatMode] = useState<ChatMode>('agent')
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
      const apiUrl = chatMode === 'agent' ? '/api/v1/aiops/agent' : '/api/v1/aiops/chat/stream'

      const response = await fetch(apiUrl, {
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

      if (chatMode === 'agent') {
        const res = await response.json()
        if (res.code === 0) {
          await addMessage(currentId, 'assistant', res.data.content)
        } else {
          await addMessage(currentId, 'assistant', '❌ 请求失败: ' + (res.message || '未知错误'))
        }
      } else {
        // 流式响应
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

        // 保存完整消息到后端
        if (fullContent) {
          await addMessage(currentId!, 'assistant', fullContent)
        }
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

    // 从对话历史中提取要执行的操作
    const lastAssistantMsg = [...activeConversation.messages]
      .reverse()
      .find(m => m.role === 'assistant')

    if (!lastAssistantMsg) return

    const content = lastAssistantMsg.content

    // 解析所有 ```action JSON 块（支持多个）
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
      // 如果没有 action 块，发送确认消息让 AI 继续
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
          ...action,
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

    // 显示所有执行结果
    await addMessage(activeId, 'assistant', results.join('\n\n'))
    setLoading(false)
  }

  // 取消操作
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
            icon={chatMode === 'agent' ? <ThunderboltOutlined /> : <RobotOutlined />}
            style={{ backgroundColor: chatMode === 'agent' ? '#722ed1' : '#1890ff', marginRight: 12, flexShrink: 0 }}
          />
        )}
        <div style={{ maxWidth: '75%', position: 'relative' }}>
          <div
            style={{
              padding: '12px 16px',
              borderRadius: 12,
              backgroundColor: isUser ? (chatMode === 'agent' ? '#722ed1' : '#1890ff') : '#f0f2f5',
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

            {/* 确认/取消按钮 - 仅在 Agent 模式下显示 */}
            {needsConfirm && chatMode === 'agent' && (
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
              style={{
                position: 'absolute',
                top: -8,
                right: isUser ? 'auto' : -8,
                left: isUser ? -8 : 'auto',
                opacity: 0.5,
                fontSize: 12,
              }}
            />
          </Tooltip>
        </div>
        {isUser && (
          <Avatar
            icon={<UserOutlined />}
            style={{ backgroundColor: '#87d068', marginLeft: 12, flexShrink: 0 }}
          />
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
        {/* 顶部栏 */}
        <div
          style={{
            padding: '12px 24px',
            borderBottom: '1px solid #e5e5e5',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            background: '#fff',
          }}
        >
          <Space>
            <Segmented
              value={chatMode}
              onChange={(value) => setChatMode(value as ChatMode)}
              options={[
                { value: 'chat', icon: <QuestionCircleOutlined />, label: '对话模式' },
                { value: 'agent', icon: <ToolOutlined />, label: 'Agent 模式' },
              ]}
            />
            <Tooltip title={chatMode === 'chat' ? '纯问答对话' : '可执行 K8S 操作'}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                {chatMode === 'chat' ? '💬 纯对话' : '🤖 可执行操作'}
              </Text>
            </Tooltip>
          </Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
        </div>

        {/* 消息区域 */}
        <div style={{ flex: 1, overflow: 'auto', padding: '24px 0', background: '#fff' }}>
          {(!activeConversation || activeConversation.messages.length === 0) ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#999' }}>
              {chatMode === 'agent' ? (
                <>
                  <ThunderboltOutlined style={{ fontSize: 64, marginBottom: 24, color: '#d9d9d9' }} />
                  <Title level={4} style={{ color: '#666' }}>AI Agent</Title>
                  <Text type="secondary">通过自然语言管理 Kubernetes 集群</Text>
                  <div style={{ marginTop: 24, textAlign: 'left', maxWidth: 500 }}>
                    <Text type="secondary">
                      <strong>查询示例</strong>（直接执行）：<br/>
                      • "查看所有 Pod"<br/>
                      • "显示节点状态"<br/><br/>
                      <strong>操作示例</strong>（需确认）：<br/>
                      • "创建一个 nginx Deployment"<br/>
                      • "删除 test Pod"
                    </Text>
                  </div>
                </>
              ) : (
                <>
                  <RobotOutlined style={{ fontSize: 64, marginBottom: 24, color: '#d9d9d9' }} />
                  <Title level={4} style={{ color: '#666' }}>AI 对话</Title>
                  <Text type="secondary">与 AI 助手自由对话</Text>
                </>
              )}
            </div>
          ) : (
            activeConversation.messages.map((msg, index) => renderMessage(msg, index))
          )}
          {loading && (
            <div style={{ display: 'flex', alignItems: 'center', padding: '0 16px', marginBottom: 24 }}>
              <Avatar icon={chatMode === 'agent' ? <ThunderboltOutlined /> : <RobotOutlined />} style={{ backgroundColor: chatMode === 'agent' ? '#722ed1' : '#1890ff', marginRight: 12 }} />
              <Spin size="small" />
              <Text type="secondary" style={{ marginLeft: 8 }}>AI 思考中...</Text>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        {/* 输入区域 */}
        <div style={{ padding: '16px 24px', borderTop: '1px solid #e5e5e5', background: '#fff' }}>
          <div style={{ display: 'flex', gap: 8 }}>
            <TextArea
              value={inputValue}
              onChange={e => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={chatMode === 'agent' ? '输入指令... (例如: 查看所有 Pod)' : '输入问题...'}
              autoSize={{ minRows: 1, maxRows: 4 }}
              disabled={loading}
              style={{ flex: 1 }}
            />
            {loading ? (
              <Button danger icon={<StopOutlined />} onClick={handleStop} style={{ height: 'auto' }}>
                停止
              </Button>
            ) : (
              <Button
                type="primary"
                icon={<SendOutlined />}
                onClick={() => handleSend()}
                disabled={!inputValue.trim()}
                style={{ height: 'auto', background: chatMode === 'agent' ? '#722ed1' : '#1890ff', borderColor: chatMode === 'agent' ? '#722ed1' : '#1890ff' }}
              >
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
