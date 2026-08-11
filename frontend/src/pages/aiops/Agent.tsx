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
  Modal,
  Popconfirm,
  Checkbox,
  Collapse,
  Tag,
  Alert,
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
  ClearOutlined,
  CheckSquareOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { confirmK8SOperation, PendingAction, ToolTraceItem } from '../../api/agent'
import { useConversations } from '../../hooks/useConversations'
import ChatSidebar from '../../components/ChatSidebar'
import MarkdownRenderer from '../../components/MarkdownRenderer'

const { Title, Text } = Typography
const { TextArea } = Input

/** 去掉历史消息里拼进去的旧版轨迹/待确认文本 */
function stripLegacyAgentExtras(content: string): string {
  if (!content) return content
  return content
    .replace(/\n*---\s*\n+\*\*待确认写操作\*\*[\s\S]*$/m, '')
    .replace(/\n*<details>[\s\S]*?<\/details>\s*$/m, '')
    .trim()
}

function summarizeToolTrace(trace: ToolTraceItem[]): { name: string; count: number; hasError: boolean }[] {
  const map = new Map<string, { count: number; hasError: boolean }>()
  for (const t of trace) {
    const cur = map.get(t.name) || { count: 0, hasError: false }
    cur.count += 1
    cur.hasError = cur.hasError || !!t.is_error
    map.set(t.name, cur)
  }
  return [...map.entries()].map(([name, v]) => ({ name, ...v }))
}

type MessageExtras = {
  toolTrace?: ToolTraceItem[]
  pendingActions?: PendingAction[]
}
const AIAgent: React.FC = () => {
  const {
    conversations,
    activeConversation,
    activeId,
    deletingId,
    batchDeleting,
    messageBatchDeleting,
    createConversation,
    selectConversation,
    addMessage,
    deleteMessagePair,
    deleteMessages,
    clearConversation,
    deleteConversation,
    deleteConversations,
    renameConversation,
  } = useConversations()

  const [inputValue, setInputValue] = useState('')
  const [loading, setLoading] = useState(false)
  const [thinkingSeconds, setThinkingSeconds] = useState(0)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [hoveredMsgId, setHoveredMsgId] = useState<number | null>(null)
  const [msgSelectMode, setMsgSelectMode] = useState(false)
  const [selectedMsgIds, setSelectedMsgIds] = useState<number[]>([])
  /** 当前会话待确认写操作（后端 stage_mutation 产物） */
  const [pendingActions, setPendingActions] = useState<PendingAction[]>([])
  /** 按消息 id 挂载工具轨迹 / 待确认面板（会话内有效） */
  const [messageExtras, setMessageExtras] = useState<Record<number, MessageExtras>>({})
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [activeConversation?.messages])

  // 切换会话时退出消息多选，并清空待确认写操作
  useEffect(() => {
    setMsgSelectMode(false)
    setSelectedMsgIds([])
    setPendingActions([])
    setMessageExtras({})
  }, [activeId])

  useEffect(() => {
    if (!loading) {
      setThinkingSeconds(0)
      return
    }
    setThinkingSeconds(0)
    const timer = window.setInterval(() => {
      setThinkingSeconds(prev => prev + 1)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [loading])

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
    if (!selectedCluster) {
      message.warning('请先选择集群')
      return
    }

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

      const raw = await response.text()
      let res: any = null
      try {
        res = raw ? JSON.parse(raw) : null
      } catch {
        throw new Error(raw || `服务返回非 JSON（HTTP ${response.status}）`)
      }

      if (!response.ok || !res || res.code !== 0) {
        const errMsg = res?.message || `HTTP ${response.status}` || '未知错误'
        setPendingActions([])
        await addMessage(currentId, 'assistant', '❌ 请求失败: ' + errMsg)
        message.error(errMsg)
        return
      }

      const reply = res.data?.content || '抱歉，我无法理解您的请求。'
      const pending: PendingAction[] = res.data?.pending_actions || []
      const trace: ToolTraceItem[] = res.data?.tool_trace || []
      setPendingActions(pending)

      const saved = await addMessage(currentId, 'assistant', reply)
      if (saved?.id && (pending.length > 0 || trace.length > 0)) {
        setMessageExtras((prev) => ({
          ...prev,
          [saved.id]: {
            toolTrace: trace.length > 0 ? trace : undefined,
            pendingActions: pending.length > 0 ? pending : undefined,
          },
        }))
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        console.log('Request aborted')
      } else {
        console.error('Chat error:', error)
        const detail = error?.message || '网络异常或服务超时'
        message.error(detail)
        await addMessage(currentId!, 'assistant', `❌ AI 请求失败：${detail}\n\n可检查 **AI 设置** 中的 LLM 配置，或缩短问题后重试。`)
      }
    } finally {
      setLoading(false)
      abortControllerRef.current = null
    }
  }

  // 确认执行：直接消费后端 pending_actions（已 dry-run 暂存）
  const handleConfirm = async () => {
    if (!activeId) return

    if (pendingActions.length === 0) {
      message.warning('没有待确认的写操作（请重新发起变更请求）')
      return
    }

    const staged = [...pendingActions]
    Modal.confirm({
      title: `确认执行 ${staged.length} 项写操作？`,
      width: 560,
      icon: <WarningOutlined style={{ color: '#faad14' }} />,
      content: (
        <div style={{ marginTop: 12 }}>
          <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
            确认后将真正变更集群，此操作不可自动回滚。
          </Text>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 320, overflow: 'auto' }}>
            {staged.map((s) => (
              <div
                key={s.action_id || s.id}
                style={{
                  padding: '10px 12px',
                  background: '#fffbe6',
                  border: '1px solid #ffe58f',
                  borderRadius: 8,
                }}
              >
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 4 }}>
                  <Tag color="orange">{s.action}</Tag>
                  <Text strong>
                    {s.namespace}/{s.name}
                  </Text>
                </div>
                <Text type="secondary" style={{ fontSize: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>
                  {s.dry_run || s.description}
                </Text>
              </div>
            ))}
          </div>
        </div>
      ),
      okText: '确认执行',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setLoading(true)
        await addMessage(activeId, 'user', '确认执行')
        const results: string[] = []
        for (const item of staged) {
          const actionId = item.action_id || item.id
          const label = `${item.action} ${item.namespace}/${item.name}`
          try {
            const res = await confirmK8SOperation(actionId)
            if (res.code === 0 && res.data?.success) {
              results.push(`✅ ${res.data.message}`)
            } else {
              results.push(`❌ ${label}: 执行失败`)
            }
          } catch (error: any) {
            results.push(`❌ ${label}: ${error?.response?.data?.message || error.message || '执行失败'}`)
          }
        }
        setPendingActions([])
        setMessageExtras((prev) => {
          const next = { ...prev }
          for (const id of Object.keys(next)) {
            const key = Number(id)
            if (next[key]?.pendingActions) {
              next[key] = { ...next[key], pendingActions: undefined }
            }
          }
          return next
        })
        await addMessage(activeId, 'assistant', results.join('\n\n'))
        setLoading(false)
      },
    })
  }

  const handleCancel = async () => {
    let currentId = activeId
    if (!currentId) {
      currentId = await createConversation()
      if (!currentId) return
    }
    setPendingActions([])
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
    const isLastAssistant =
      !isUser &&
      activeConversation?.messages &&
      [...activeConversation.messages].reverse().find((m) => m.role === 'assistant')?.id === msg.id
    const extras = !isUser ? messageExtras[msg.id] : undefined
    const showPending =
      !isUser &&
      isLastAssistant &&
      pendingActions.length > 0 &&
      (extras?.pendingActions?.length || pendingActions.length) > 0
    const pendingForPanel =
      isLastAssistant && pendingActions.length > 0
        ? pendingActions
        : extras?.pendingActions || []
    const toolTrace = extras?.toolTrace || []
    const needsConfirm = showPending && pendingForPanel.length > 0
    const showDelete = !msgSelectMode && hoveredMsgId === msg.id
    const checked = selectedMsgIds.includes(msg.id)
    const displayContent = isUser ? msg.content : stripLegacyAgentExtras(msg.content || '')

    return (
      <div
        key={msg.id || index}
        style={{
          display: 'flex',
          justifyContent: isUser ? 'flex-end' : 'flex-start',
          alignItems: 'flex-start',
          marginBottom: 24,
          padding: '0 16px',
          position: 'relative',
          gap: 8,
          background: msgSelectMode && checked ? 'rgba(255,77,79,0.06)' : 'transparent',
          borderRadius: 8,
          cursor: msgSelectMode ? 'pointer' : undefined,
        }}
        onMouseEnter={() => setHoveredMsgId(msg.id)}
        onMouseLeave={() => setHoveredMsgId(null)}
        onClick={() => {
          if (!msgSelectMode || messageBatchDeleting) return
          setSelectedMsgIds(prev =>
            prev.includes(msg.id) ? prev.filter(id => msg.id !== id) : [...prev, msg.id]
          )
        }}
      >
        {msgSelectMode && (
          <Checkbox
            checked={checked}
            disabled={messageBatchDeleting}
            onClick={(e) => e.stopPropagation()}
            onChange={() => {
              setSelectedMsgIds(prev =>
                prev.includes(msg.id) ? prev.filter(id => id !== msg.id) : [...prev, msg.id]
              )
            }}
            style={{ marginTop: 14, flexShrink: 0 }}
          />
        )}
        {!isUser && (
          <Avatar
            icon={<ThunderboltOutlined />}
            style={{ backgroundColor: '#722ed1', marginRight: msgSelectMode ? 0 : 12, flexShrink: 0 }}
          />
        )}
        <div
          style={{
            maxWidth: msgSelectMode ? '70%' : '75%',
            position: 'relative',
            minWidth: 0,
            width: isUser ? 'fit-content' : '100%',
            display: 'flex',
            flexDirection: isUser ? 'row-reverse' : 'row',
            alignItems: 'flex-start',
            gap: 4,
          }}
        >
          <div
            style={{
              padding: isUser ? '10px 14px' : '12px 16px',
              borderRadius: 12,
              backgroundColor: isUser ? '#722ed1' : '#f0f2f5',
              color: isUser ? '#fff' : '#333',
              boxShadow: '0 1px 2px rgba(0,0,0,0.1)',
              outline: msgSelectMode && checked ? '2px solid #ff4d4f' : undefined,
              maxWidth: '100%',
              boxSizing: 'border-box',
              minWidth: 0,
              flex: isUser ? '0 1 auto' : '1 1 auto',
            }}
          >
            {isEmpty ? (
              <Spin size="small" />
            ) : isUser ? (
              <div
                style={{
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  overflowWrap: 'anywhere',
                  lineHeight: 1.5,
                }}
              >
                {displayContent}
              </div>
            ) : (
              <div className="markdown-body">
                <MarkdownRenderer content={displayContent} />
              </div>
            )}

            {!isUser && !isEmpty && toolTrace.length > 0 && (
              <div style={{ marginTop: 12 }} onClick={(e) => e.stopPropagation()}>
                <Collapse
                  size="small"
                  ghost
                  items={[
                    {
                      key: 'tools',
                      label: (
                        <Space size={6} wrap>
                          <ToolOutlined style={{ color: '#722ed1' }} />
                          <Text style={{ fontSize: 13 }}>工具调用</Text>
                          <Tag style={{ margin: 0 }}>{toolTrace.length}</Tag>
                          {summarizeToolTrace(toolTrace).map((t) => (
                            <Tag
                              key={t.name}
                              color={t.hasError ? 'error' : 'purple'}
                              style={{ margin: 0 }}
                            >
                              {t.name}
                              {t.count > 1 ? ` ×${t.count}` : ''}
                            </Tag>
                          ))}
                        </Space>
                      ),
                      children: (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                          {toolTrace.map((t, i) => (
                            <div
                              key={`${t.name}-${i}`}
                              style={{
                                background: '#fff',
                                borderRadius: 8,
                                border: '1px solid #e8e8e8',
                                padding: '8px 10px',
                              }}
                            >
                              <Space size={8} style={{ marginBottom: 4 }}>
                                <Tag color={t.is_error ? 'error' : 'processing'}>{t.name}</Tag>
                                {t.is_error && <Text type="danger" style={{ fontSize: 12 }}>失败</Text>}
                              </Space>
                              {t.args && (
                                <div style={{ marginBottom: 6 }}>
                                  <Text type="secondary" style={{ fontSize: 11 }}>参数</Text>
                                  <pre
                                    style={{
                                      margin: '2px 0 0',
                                      padding: 8,
                                      background: '#fafafa',
                                      borderRadius: 6,
                                      fontSize: 11,
                                      maxHeight: 96,
                                      overflow: 'auto',
                                      whiteSpace: 'pre-wrap',
                                      wordBreak: 'break-all',
                                    }}
                                  >
                                    {t.args}
                                  </pre>
                                </div>
                              )}
                              {t.result && (
                                <div>
                                  <Text type="secondary" style={{ fontSize: 11 }}>结果摘要</Text>
                                  <pre
                                    style={{
                                      margin: '2px 0 0',
                                      padding: 8,
                                      background: '#fafafa',
                                      borderRadius: 6,
                                      fontSize: 11,
                                      maxHeight: 120,
                                      overflow: 'auto',
                                      whiteSpace: 'pre-wrap',
                                      wordBreak: 'break-all',
                                    }}
                                  >
                                    {t.result}
                                  </pre>
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      ),
                    },
                  ]}
                />
              </div>
            )}

            {needsConfirm && !msgSelectMode && (
              <div style={{ marginTop: 12 }} onClick={(e) => e.stopPropagation()}>
                <Alert
                  type="warning"
                  showIcon
                  icon={<WarningOutlined />}
                  message={
                    <Text strong>
                      待确认写操作（{pendingForPanel.length}）
                    </Text>
                  }
                  description={
                    <div style={{ marginTop: 8 }}>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 12 }}>
                        {pendingForPanel.map((p) => (
                          <div
                            key={p.action_id || p.id}
                            style={{
                              background: '#fff',
                              borderRadius: 8,
                              border: '1px solid #ffe58f',
                              padding: '8px 10px',
                            }}
                          >
                            <Space wrap size={6} style={{ marginBottom: 4 }}>
                              <Tag color="orange">{p.action}</Tag>
                              <Text code>
                                {p.namespace}/{p.name}
                              </Text>
                            </Space>
                            <div
                              style={{
                                fontSize: 12,
                                color: '#666',
                                fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
                                lineHeight: 1.5,
                              }}
                            >
                              {p.dry_run || p.description}
                            </div>
                          </div>
                        ))}
                      </div>
                      <Space>
                        <Button
                          type="primary"
                          size="small"
                          danger
                          icon={<CheckCircleOutlined />}
                          onClick={handleConfirm}
                        >
                          确认执行
                        </Button>
                        <Button
                          size="small"
                          icon={<CloseCircleOutlined />}
                          onClick={handleCancel}
                        >
                          取消
                        </Button>
                      </Space>
                    </div>
                  }
                />
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
          {!msgSelectMode && (
            <div
              style={{
                flexShrink: 0,
                opacity: showDelete ? 1 : 0,
                pointerEvents: showDelete ? 'auto' : 'none',
                transition: 'opacity 0.15s',
                marginTop: 4,
              }}
            >
              <Popconfirm
                title="删除这一轮对话？"
                description="将同时删除对应的提问与回答。"
                okText="删除"
                okButtonProps={{ danger: true }}
                cancelText="取消"
                onConfirm={() => activeId && deleteMessagePair(activeId, msg.id)}
              >
                <Tooltip title="删除本轮对话">
                  <Button
                    type="text"
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                  />
                </Tooltip>
              </Popconfirm>
            </div>
          )}
        </div>
        {isUser && (
          <Avatar icon={<UserOutlined />} style={{ backgroundColor: '#87d068', marginLeft: msgSelectMode ? 0 : 12, flexShrink: 0 }} />
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

  const hasMessages = !!activeConversation && activeConversation.messages.length > 0
  const allMsgIds = activeConversation?.messages.map(m => m.id) || []
  const allMsgsSelected = allMsgIds.length > 0 && selectedMsgIds.length === allMsgIds.length
  const partialMsgsSelected = selectedMsgIds.length > 0 && !allMsgsSelected

  const exitMsgSelectMode = () => {
    setMsgSelectMode(false)
    setSelectedMsgIds([])
  }

  const handleBatchDeleteMessages = async () => {
    if (!activeId || selectedMsgIds.length === 0) return
    const ok = await deleteMessages(activeId, selectedMsgIds)
    if (ok) exitMsgSelectMode()
  }

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 180px)', background: '#fff', borderRadius: 8, overflow: 'hidden' }}>
      <ChatSidebar
        conversations={sidebarConversations}
        activeId={activeId ? String(activeId) : null}
        onSelect={(id) => selectConversation(Number(id))}
        onCreate={() => { void createConversation() }}
        onDelete={async (id) => { await deleteConversation(Number(id)) }}
        onBatchDelete={async (ids) => { await deleteConversations(ids.map(Number)) }}
        onRename={async (id, title) => { await renameConversation(Number(id), title) }}
        onClear={async (id) => { await clearConversation(Number(id)) }}
        deletingId={deletingId != null ? String(deletingId) : null}
        batchDeleting={batchDeleting}
        collapsed={sidebarCollapsed}
        onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
      />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '12px 24px', borderBottom: '1px solid #e5e5e5', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#fff' }}>
          <Space>
            <ThunderboltOutlined style={{ color: '#722ed1', fontSize: 20 }} />
            <Title level={5} style={{ margin: 0 }}>
              {activeConversation?.title || 'AI Agent'}
            </Title>
            <Tooltip title="AI Agent 可以理解自然语言并执行 K8S 操作">
              <QuestionCircleOutlined style={{ color: '#999' }} />
            </Tooltip>
          </Space>
          <Space wrap>
            <Select
              value={selectedCluster || undefined}
              onChange={setSelectedCluster}
              style={{ width: 200 }}
              placeholder="选择集群"
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
              disabled={msgSelectMode}
            />
            {activeId && hasMessages && (
              msgSelectMode ? (
                <>
                  <Checkbox
                    checked={allMsgsSelected}
                    indeterminate={partialMsgsSelected}
                    disabled={messageBatchDeleting}
                    onChange={() => setSelectedMsgIds(allMsgsSelected ? [] : allMsgIds)}
                  >
                    全选
                  </Checkbox>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    已选 {selectedMsgIds.length}/{allMsgIds.length}
                  </Text>
                  <Popconfirm
                    title={`删除选中的 ${selectedMsgIds.length} 条消息？`}
                    description="仅删除勾选的消息，不会自动附带整轮。"
                    okText="删除"
                    okButtonProps={{ danger: true, loading: messageBatchDeleting }}
                    cancelText="取消"
                    disabled={selectedMsgIds.length === 0 || messageBatchDeleting}
                    onConfirm={() => void handleBatchDeleteMessages()}
                  >
                    <Button
                      danger
                      icon={<DeleteOutlined />}
                      disabled={selectedMsgIds.length === 0}
                      loading={messageBatchDeleting}
                    >
                      删除所选
                    </Button>
                  </Popconfirm>
                  <Button onClick={exitMsgSelectMode} disabled={messageBatchDeleting}>
                    取消
                  </Button>
                </>
              ) : (
                <>
                  <Tooltip title="多选删除消息">
                    <Button
                      icon={<CheckSquareOutlined />}
                      disabled={loading}
                      onClick={() => {
                        setMsgSelectMode(true)
                        setSelectedMsgIds([])
                      }}
                    />
                  </Tooltip>
                  <Popconfirm
                    title="清空当前会话消息？"
                    description="仅清空消息，会话会保留。"
                    okText="清空"
                    cancelText="取消"
                    disabled={!hasMessages || loading}
                    onConfirm={() => clearConversation(activeId)}
                  >
                    <Tooltip title="清空消息">
                      <Button icon={<ClearOutlined />} disabled={!hasMessages || loading} />
                    </Tooltip>
                  </Popconfirm>
                  <Popconfirm
                    title="删除当前会话？"
                    description="会话及其全部消息将永久删除。"
                    okText="删除"
                    okButtonProps={{ danger: true }}
                    cancelText="取消"
                    disabled={loading}
                    onConfirm={() => deleteConversation(activeId)}
                  >
                    <Tooltip title="删除会话">
                      <Button danger icon={<DeleteOutlined />} disabled={loading} loading={deletingId === activeId} />
                    </Tooltip>
                  </Popconfirm>
                </>
              )
            )}
            {activeId && !hasMessages && (
              <Popconfirm
                title="删除当前会话？"
                description="会话将永久删除。"
                okText="删除"
                okButtonProps={{ danger: true }}
                cancelText="取消"
                disabled={loading}
                onConfirm={() => deleteConversation(activeId)}
              >
                <Tooltip title="删除会话">
                  <Button danger icon={<DeleteOutlined />} disabled={loading} loading={deletingId === activeId} />
                </Tooltip>
              </Popconfirm>
            )}
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
              <Text type="secondary" style={{ marginLeft: 8 }}>
                AI Agent 思考中...{thinkingSeconds > 0 ? `（已等待 ${thinkingSeconds}s）` : ''}
                {thinkingSeconds >= 20 ? '，复杂问题可能需要更久' : ''}
              </Text>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        <div style={{ padding: '16px 24px', borderTop: '1px solid #e5e5e5', background: '#fff' }}>
          {msgSelectMode ? (
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}>
              <Text type="secondary">
                多选模式：点击消息即可勾选，已选 {selectedMsgIds.length} 条
              </Text>
              <Space>
                <Popconfirm
                  title={`删除选中的 ${selectedMsgIds.length} 条消息？`}
                  okText="删除"
                  okButtonProps={{ danger: true, loading: messageBatchDeleting }}
                  cancelText="取消"
                  disabled={selectedMsgIds.length === 0 || messageBatchDeleting}
                  onConfirm={() => void handleBatchDeleteMessages()}
                >
                  <Button danger icon={<DeleteOutlined />} disabled={selectedMsgIds.length === 0} loading={messageBatchDeleting}>
                    删除所选
                  </Button>
                </Popconfirm>
                <Button onClick={exitMsgSelectMode} disabled={messageBatchDeleting}>退出多选</Button>
              </Space>
            </div>
          ) : (
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
          )}
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
