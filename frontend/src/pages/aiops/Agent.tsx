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
import {
  agentChatStream,
  cancelPendingActions,
  confirmK8SOperation,
  listPendingActions,
  PendingAction,
  ToolTraceItem,
} from '../../api/agent'
import { useConversations } from '../../hooks/useConversations'
import ChatSidebar from '../../components/ChatSidebar'
import MarkdownRenderer from '../../components/MarkdownRenderer'
import { AIReadOnlyBanner } from '../../components/AIReadOnlyBanner'
import { useAuthStore } from '../../stores/auth'

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

/** 回答「依据」：可点开的工具轨迹面板 */
function ToolEvidencePanel({
  trace,
  compact,
}: {
  trace: ToolTraceItem[]
  compact?: boolean
}) {
  const [open, setOpen] = useState<string[]>([])
  const [focusIdx, setFocusIdx] = useState<number | null>(null)

  if (!trace.length) return null

  const openPanel = (idx?: number) => {
    setOpen(['tools'])
    if (typeof idx === 'number') {
      setFocusIdx(idx)
      requestAnimationFrame(() => {
        document.getElementById(`tool-evidence-${idx}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
      })
    }
  }

  return (
    <div style={{ marginTop: compact ? 0 : 12, marginBottom: compact ? 8 : 0 }} onClick={(e) => e.stopPropagation()}>
      <Collapse
        size="small"
        ghost
        activeKey={open}
        onChange={(keys) => setOpen(Array.isArray(keys) ? keys.map(String) : [String(keys)])}
        items={[
          {
            key: 'tools',
            label: (
              <Space size={6} wrap>
                <ToolOutlined style={{ color: '#722ed1' }} />
                <Text
                  style={{ fontSize: 13, cursor: 'pointer', color: '#722ed1' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    openPanel()
                  }}
                >
                  基于 {trace.length} 次工具
                </Text>
                {summarizeToolTrace(trace).map((t) => {
                  const firstIdx = trace.findIndex((x) => x.name === t.name)
                  return (
                    <Tag
                      key={t.name}
                      color={t.hasError ? 'error' : 'purple'}
                      style={{ margin: 0, cursor: 'pointer' }}
                      onClick={(e) => {
                        e.stopPropagation()
                        openPanel(firstIdx >= 0 ? firstIdx : undefined)
                      }}
                    >
                      {t.name}
                      {t.count > 1 ? ` ×${t.count}` : ''}
                    </Tag>
                  )
                })}
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {trace.map((t, i) => (
                  <div
                    id={`tool-evidence-${i}`}
                    key={`${t.name}-${i}`}
                    style={{
                      background: '#fff',
                      borderRadius: 8,
                      border: focusIdx === i ? '1px solid #722ed1' : '1px solid #e8e8e8',
                      padding: '8px 10px',
                    }}
                  >
                    <Space size={8} style={{ marginBottom: 4 }} wrap>
                      <Tag color={t.is_error ? 'error' : 'processing'}>{t.name}</Tag>
                      {typeof t.duration_ms === 'number' && (
                        <Text type="secondary" style={{ fontSize: 11 }}>{t.duration_ms}ms</Text>
                      )}
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
  )
}

type MessageExtras = {
  toolTrace?: ToolTraceItem[]
  pendingActions?: PendingAction[]
}

function isControllerOwnedDelete(action: PendingAction): boolean {
  const text = `${action.action || ''} ${action.dry_run || ''} ${action.description || ''}`
  return (
    action.action === 'delete_pod' &&
    (/WARNING:.*Deployment\//i.test(text) ||
      /WARNING:.*ReplicaSet\//i.test(text) ||
      /会立刻重建|可能被立即重建|may be recreated/i.test(text))
  )
}

/** 待确认写操作面板：固定渲染在本轮助手回答气泡底部 */
function PendingConfirmPanel({
  actions,
  onConfirm,
  onCancel,
}: {
  actions: PendingAction[]
  onConfirm: () => void
  onCancel: () => void
}) {
  if (!actions.length) return null
  const hasRecreateRisk = actions.some(isControllerOwnedDelete)
  return (
    <div style={{ marginTop: 12 }} onClick={(e) => e.stopPropagation()}>
      <Alert
        type="warning"
        showIcon
        icon={<WarningOutlined />}
        message={<Text strong>待确认写操作（{actions.length}）</Text>}
        description={
          <div style={{ marginTop: 8 }}>
            {hasRecreateRisk && (
              <Alert
                type="error"
                showIcon
                style={{ marginBottom: 12 }}
                message="删除控制器托管的 Pod 会被立即重建"
                description="Deployment/ReplicaSet 管理的 Pod 删掉后会马上拉起新实例，工作负载看起来像“没删掉”。若要彻底移除，请改为删除 Deployment。"
              />
            )}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 12 }}>
              {actions.map((p) => (
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
                    {isControllerOwnedDelete(p) && <Tag color="red">可能被重建</Tag>}
                  </Space>
                  <div
                    style={{
                      fontSize: 12,
                      color: '#666',
                      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
                      lineHeight: 1.5,
                      whiteSpace: 'pre-wrap',
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
                onClick={onConfirm}
              >
                确认执行
              </Button>
              <Button size="small" icon={<CloseCircleOutlined />} onClick={onCancel}>
                取消
              </Button>
            </Space>
          </div>
        }
      />
    </div>
  )
}

const AIAgent: React.FC = () => {
  const { hasPermission } = useAuthStore()
  const canExecute = hasPermission('aiops', 'execute')
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
    fetchConversationDetail,
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
  /** 本轮产生 pending 的助手消息 id（固定挂在该气泡底部） */
  const [pendingHostMessageId, setPendingHostMessageId] = useState<number | null>(null)
  /** 按消息 id 挂载工具轨迹 / 待确认面板（会话内有效） */
  const [messageExtras, setMessageExtras] = useState<Record<number, MessageExtras>>({})
  /** 流式中的临时助手气泡（结束后由会话详情替换） */
  const [liveAssistant, setLiveAssistant] = useState<{
    content: string
    streaming: boolean
    status?: string
    toolTrace: ToolTraceItem[]
  } | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [
    activeConversation?.messages,
    liveAssistant?.content,
    liveAssistant?.toolTrace?.length,
    liveAssistant?.status,
    pendingActions.length,
  ])

  // 切换会话：清空本地态，并从后端恢复 pending / extras
  useEffect(() => {
    setMsgSelectMode(false)
    setSelectedMsgIds([])
    setPendingActions([])
    setPendingHostMessageId(null)
    setMessageExtras({})
    setLiveAssistant(null)
    if (!activeId) return

    let cancelled = false
    ;(async () => {
      try {
        const res = await listPendingActions(activeId)
        if (cancelled) return
        if (res.code === 0) {
          setPendingActions(res.data?.pending_actions || [])
        }
      } catch (e) {
        console.error('Failed to load pending actions', e)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [activeId])

  // 会话详情加载后，从消息 extras 恢复工具轨迹，并把 pending 挂到对应助手消息
  useEffect(() => {
    const msgs = activeConversation?.messages
    if (!msgs?.length) return
    const next: Record<number, MessageExtras> = {}
    let hostFromExtras: number | null = null
    for (const m of msgs) {
      if (m.role !== 'assistant' || !m.extras) continue
      try {
        const raw = JSON.parse(m.extras) as {
          tool_trace?: ToolTraceItem[]
          pending_action_ids?: number[]
        }
        const linked =
          pendingActions.length > 0 && raw.pending_action_ids?.length
            ? pendingActions.filter((p) =>
                raw.pending_action_ids!.includes(p.action_id || p.id)
              )
            : undefined
        if (linked?.length) {
          hostFromExtras = m.id
        }
        next[m.id] = {
          toolTrace: raw.tool_trace,
          pendingActions: linked?.length ? linked : undefined,
        }
      } catch {
        /* ignore */
      }
    }
    if (Object.keys(next).length > 0) {
      setMessageExtras((prev) => ({ ...prev, ...next }))
    }
    // 若本轮尚未记录 host（例如刷新/切回会话），用 extras 反推
    if (hostFromExtras && pendingActions.length > 0) {
      setPendingHostMessageId((prev) => prev ?? hostFromExtras)
    }
  }, [activeConversation?.messages, pendingActions])

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
    if (!canExecute) {
      message.warning('当前为只读权限，无法发送 AI 对话')
      return
    }
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

    await addMessage(currentId, 'user', sendContent)

    setLoading(true)
    setPendingActions([])
    setPendingHostMessageId(null)
    setLiveAssistant({ content: '', streaming: true, status: 'thinking', toolTrace: [] })

    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      await agentChatStream(
        {
          message: sendContent,
          cluster_id: selectedCluster,
          conversation_id: currentId,
        },
        (ev) => {
          if (ev.type === 'status') {
            setLiveAssistant((prev) =>
              prev ? { ...prev, status: ev.status || prev.status } : prev
            )
          } else if (ev.type === 'tool_start') {
            setLiveAssistant((prev) => {
              if (!prev) return prev
              const trace = [
                ...prev.toolTrace,
                { name: ev.name || 'tool', args: ev.args || '', result: '', is_error: false },
              ]
              return { ...prev, status: `running:${ev.name}`, toolTrace: trace }
            })
          } else if (ev.type === 'tool_result') {
            setLiveAssistant((prev) => {
              if (!prev) return prev
              const trace = [...prev.toolTrace]
              for (let i = trace.length - 1; i >= 0; i--) {
                if (trace[i].name === ev.name && !trace[i].result) {
                  trace[i] = {
                    ...trace[i],
                    result: ev.result || '',
                    is_error: !!ev.is_error,
                    duration_ms: ev.duration_ms,
                  }
                  break
                }
              }
              return { ...prev, toolTrace: trace }
            })
            // 不在 tool_result 时展示待确认：需等最终回答流式结束后（done）再挂载，
            // 否则会抢在 AI 输出完成前出现在上一条助手气泡上。
          } else if (ev.type === 'content_delta') {
            setLiveAssistant((prev) =>
              prev
                ? {
                    ...prev,
                    content: prev.content + (ev.delta || ''),
                    status: 'writing',
                  }
                : prev
            )
          } else if (ev.type === 'done') {
            const pending = ev.pending_actions || []
            const trace = ev.tool_trace || []
            setPendingActions(pending)
            setPendingHostMessageId(ev.message_id || null)
            setLiveAssistant({
              content: ev.content || '',
              streaming: false,
              status: 'done',
              toolTrace: trace,
            })
          } else if (ev.type === 'error') {
            throw new Error(ev.message || 'Agent 流式错误')
          }
        },
        abortController.signal
      )

      // 后端已落库助手消息；刷新详情并挂上 extras
      await fetchConversationDetail(currentId)
      setLiveAssistant(null)
    } catch (error: any) {
      if (error.name === 'AbortError') {
        console.log('Request aborted')
        setLiveAssistant((prev) =>
          prev
            ? {
                ...prev,
                streaming: false,
                status: 'stopped',
                content:
                  prev.content ||
                  (prev.toolTrace.length
                    ? '（已停止；可展开上方工具依据查看已拉取结果）'
                    : '（已停止）'),
              }
            : null
        )
      } else {
        console.error('Chat error:', error)
        const detail = error?.message || '网络异常或服务超时'
        message.error(detail)
        let kept = false
        setLiveAssistant((prev) => {
          if (prev && (prev.content || prev.toolTrace.length > 0)) {
            kept = true
            return {
              ...prev,
              streaming: false,
              status: 'error',
              content: prev.content
                ? `${prev.content}\n\n❌ ${detail}`
                : `❌ AI 请求失败：${detail}`,
            }
          }
          return null
        })
        if (!kept) {
          await addMessage(
            currentId!,
            'assistant',
            `❌ AI 请求失败：${detail}\n\n可检查 **AI 设置** 中的 LLM 配置，或缩短问题后重试。`
          )
        }
      }
    } finally {
      setLoading(false)
      abortControllerRef.current = null
      // 中止/异常时工具可能已落库 pending，结束后再同步一次
      if (currentId) {
        try {
          const res = await listPendingActions(currentId)
          setPendingActions(res.data?.pending_actions || [])
        } catch {
          /* ignore */
        }
      }
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
    const recreateRisk = staged.some(isControllerOwnedDelete)
    Modal.confirm({
      title: `确认执行 ${staged.length} 项写操作？`,
      width: 560,
      icon: <WarningOutlined style={{ color: '#faad14' }} />,
      content: (
        <div style={{ marginTop: 12 }}>
          <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
            确认后将真正变更集群，此操作不可自动回滚。
          </Text>
          {recreateRisk && (
            <Alert
              type="error"
              showIcon
              style={{ marginBottom: 12 }}
              message="注意：删除 Deployment 托管的 Pod 会被立刻重建"
              description="API 删除单个 Pod 会成功，但 ReplicaSet 会马上创建新 Pod，界面上仍能看到同类实例。彻底移除请删除 Deployment。"
            />
          )}
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
                  {isControllerOwnedDelete(s) && <Tag color="red">可能被重建</Tag>}
                </div>
                <Text
                  type="secondary"
                  style={{
                    fontSize: 12,
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
                    whiteSpace: 'pre-wrap',
                    display: 'block',
                  }}
                >
                  {s.dry_run || s.description}
                </Text>
              </div>
            ))}
          </div>
        </div>
      ),
      okText: recreateRisk ? '仍要删除该 Pod' : '确认执行',
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
              const details = res.data.details?.length
                ? `\n${res.data.details.map((d) => `- ${d}`).join('\n')}`
                : ''
              const recreated = res.data.details?.some((d) => d.includes('recreated_by_controller=true'))
              results.push(`${recreated ? '⚠️' : '✅'} ${res.data.message}${details}`)
            } else {
              results.push(`❌ ${label}: ${res.data?.message || '执行失败'}`)
            }
          } catch (error: any) {
            results.push(`❌ ${label}: ${error?.response?.data?.message || error.message || '执行失败'}`)
          }
        }
        setPendingActions([])
        setPendingHostMessageId(null)
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
    try {
      await cancelPendingActions(
        currentId,
        pendingActions.map((p) => p.action_id || p.id)
      )
    } catch (e) {
      console.error(e)
    }
    setPendingActions([])
    setPendingHostMessageId(null)
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
    const extras = !isUser ? messageExtras[msg.id] : undefined
    // live 气泡仍在时由 live 承载 pending，避免双份；流式未结束也不展示
    const streamBusy = !!liveAssistant?.streaming
    const liveOwnsPending = !!liveAssistant && !liveAssistant.streaming && pendingActions.length > 0
    const isPendingHost = !isUser && (
      msg.id === pendingHostMessageId ||
      !!(extras?.pendingActions && extras.pendingActions.length > 0)
    )
    const pendingForPanel =
      !streamBusy && !liveOwnsPending && isPendingHost
        ? (extras?.pendingActions?.length
            ? extras.pendingActions
            : msg.id === pendingHostMessageId
              ? pendingActions
              : [])
        : []
    const toolTrace = extras?.toolTrace || []
    const needsConfirm = pendingForPanel.length > 0
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
              <ToolEvidencePanel trace={toolTrace} />
            )}

            {needsConfirm && !msgSelectMode && (
              <PendingConfirmPanel
                actions={pendingForPanel}
                onConfirm={handleConfirm}
                onCancel={handleCancel}
              />
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
    username: c.username,
    realName: c.real_name,
    mine: c.mine,
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
    <div>
      <AIReadOnlyBanner />
      <div style={{ display: 'flex', height: 'calc(100vh - 180px)', background: '#fff', borderRadius: 8, overflow: 'hidden' }}>
      <ChatSidebar
        conversations={sidebarConversations}
        activeId={activeId ? String(activeId) : null}
        onSelect={(id) => selectConversation(Number(id))}
        onCreate={() => {
          if (!canExecute) {
            message.warning('当前为只读权限，无法创建对话')
            return
          }
          void createConversation()
        }}
        onDelete={async (id) => {
          if (!canExecute) {
            message.warning('当前为只读权限，无法删除对话')
            return
          }
          await deleteConversation(Number(id))
        }}
        onBatchDelete={async (ids) => {
          if (!canExecute) {
            message.warning('当前为只读权限，无法删除对话')
            return
          }
          await deleteConversations(ids.map(Number))
        }}
        onRename={async (id, title) => {
          if (!canExecute) {
            message.warning('当前为只读权限，无法重命名对话')
            return
          }
          await renameConversation(Number(id), title)
        }}
        onClear={async (id) => {
          if (!canExecute) {
            message.warning('当前为只读权限，无法清空对话')
            return
          }
          await clearConversation(Number(id))
        }}
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
          {(!activeConversation || (activeConversation.messages.length === 0 && !liveAssistant)) ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#999' }}>
              <ToolOutlined style={{ fontSize: 64, marginBottom: 24, color: '#d9d9d9' }} />
              <Title level={4} style={{ color: '#666' }}>AI Agent</Title>
              <Text type="secondary" style={{ textAlign: 'center', maxWidth: 400 }}>
                使用自然语言描述你想做的操作，AI 会通过工具查询/变更集群
              </Text>
              <div style={{ marginTop: 24, textAlign: 'left' }}>
                <Text type="secondary">示例：</Text>
                <ul style={{ color: '#999', marginTop: 8 }}>
                  <li>列出 default 命名空间的 Pod</li>
                  <li>查看某个 Pod 的事件和日志</li>
                  <li>删除 default 下 cj-test 开头的 Pod（需确认）</li>
                </ul>
              </div>
            </div>
          ) : (
            <>
              {activeConversation?.messages.map((msg, index) => renderMessage(msg, index))}
              {liveAssistant && (
                <div style={{ display: 'flex', justifyContent: 'flex-start', alignItems: 'flex-start', marginBottom: 24, padding: '0 16px', gap: 8 }}>
                  <Avatar icon={<ThunderboltOutlined />} style={{ backgroundColor: '#722ed1', marginRight: 12, flexShrink: 0 }} />
                  <div style={{ maxWidth: '75%', width: '100%' }}>
                    <div style={{ padding: '12px 16px', borderRadius: 12, backgroundColor: '#f0f2f5', color: '#333', boxShadow: '0 1px 2px rgba(0,0,0,0.1)' }}>
                      {liveAssistant.status &&
                        liveAssistant.status !== 'done' &&
                        liveAssistant.status !== 'writing' &&
                        liveAssistant.status !== 'stopped' &&
                        liveAssistant.status !== 'error' && (
                        <div style={{ marginBottom: 8 }}>
                          <Space size={6}>
                            <Spin size="small" />
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              {liveAssistant.status.startsWith('running:')
                                ? `正在调用 ${liveAssistant.status.replace('running:', '')}…`
                                : liveAssistant.status === 'thinking'
                                  ? `思考中…${thinkingSeconds > 0 ? `（${thinkingSeconds}s）` : ''}`
                                  : liveAssistant.status === 'summarizing'
                                    ? '整理回答…'
                                    : liveAssistant.status}
                            </Text>
                          </Space>
                        </div>
                      )}
                      {(liveAssistant.status === 'stopped' || liveAssistant.status === 'error') && (
                        <div style={{ marginBottom: 8 }}>
                          <Text type={liveAssistant.status === 'error' ? 'danger' : 'secondary'} style={{ fontSize: 13 }}>
                            {liveAssistant.status === 'stopped' ? '已停止生成' : '生成中断'}
                          </Text>
                        </div>
                      )}
                      {liveAssistant.toolTrace.length > 0 && (
                        <ToolEvidencePanel trace={liveAssistant.toolTrace} compact />
                      )}
                      {/* 流式中用纯文本，避免半截 Markdown 破坏渲染 */}
                      {liveAssistant.content ? (
                        liveAssistant.streaming ? (
                          <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', lineHeight: 1.6 }}>
                            {liveAssistant.content}
                            <span style={{ opacity: 0.4 }}>▍</span>
                          </div>
                        ) : (
                          <div className="markdown-body">
                            <MarkdownRenderer content={liveAssistant.content} />
                          </div>
                        )
                      ) : (
                        !liveAssistant.toolTrace.length && liveAssistant.streaming && <Spin size="small" />
                      )}
                      {/* 本轮回答结束后，待确认固定挂在本轮 live 气泡底部 */}
                      {!liveAssistant.streaming &&
                        pendingActions.length > 0 &&
                        (liveAssistant.status === 'done' ||
                          liveAssistant.status === 'stopped' ||
                          liveAssistant.status === 'error') && (
                          <PendingConfirmPanel
                            actions={pendingActions}
                            onConfirm={handleConfirm}
                            onCancel={handleCancel}
                          />
                        )}
                    </div>
                  </div>
                </div>
              )}
            </>
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
                placeholder={canExecute ? '描述你想做的操作... (Enter 发送，Shift+Enter 换行)' : '只读模式：可浏览历史对话，不可发送'}
                autoSize={{ minRows: 1, maxRows: 4 }}
                disabled={loading || !canExecute}
                style={{ flex: 1 }}
              />
              {loading ? (
                <Button danger icon={<StopOutlined />} onClick={handleStop} style={{ height: 'auto' }}>
                  停止
                </Button>
              ) : (
                <Button type="primary" icon={<SendOutlined />} onClick={() => handleSend()} disabled={!canExecute || !inputValue.trim()} style={{ height: 'auto', backgroundColor: '#722ed1', borderColor: '#722ed1' }}>
                  发送
                </Button>
              )}
            </div>
          )}
        </div>
      </div>
      </div>
    </div>
  )
}

export default AIAgent
