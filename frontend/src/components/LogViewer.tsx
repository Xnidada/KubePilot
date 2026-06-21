import { useState, useEffect, useRef } from 'react'
import {
  Modal, Button, Space, Input, Spin, Switch, Select, message
} from 'antd'
import {
  DownloadOutlined, SearchOutlined, SyncOutlined,
  ArrowUpOutlined, ArrowDownOutlined
} from '@ant-design/icons'



interface LogViewerProps {
  visible: boolean
  onClose: () => void
  clusterId: number
  namespace: string
  podName: string
  containerName?: string
}

const LogViewer: React.FC<LogViewerProps> = ({
  visible,
  onClose,
  clusterId,
  namespace,
  podName,
  containerName,
}) => {
  const [logs, setLogs] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [searchText, setSearchText] = useState('')
  const [tailLines, setTailLines] = useState(100)
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [highlightedLogs, setHighlightedLogs] = useState<string>('')
  const logContainerRef = useRef<HTMLPreElement>(null)
  const refreshTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (visible) {
      fetchLogs()
    }
    return () => {
      stopAutoRefresh()
    }
  }, [visible, tailLines])

  useEffect(() => {
    if (autoRefresh) {
      startAutoRefresh()
    } else {
      stopAutoRefresh()
    }
    return () => stopAutoRefresh()
  }, [autoRefresh])

  useEffect(() => {
    highlightLogs()
  }, [logs, searchText])

  const fetchLogs = async () => {
    setLoading(true)
    try {
      const token = getAuthToken()
      const response = await fetch(
        `/api/v1/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/logs?tail=${tailLines}&container=${containerName || ''}`,
        { headers: { 'Authorization': `Bearer ${token}` } }
      )

      // 检查响应类型
      const contentType = response.headers.get('content-type')
      if (contentType && contentType.includes('application/json')) {
        // JSON 响应
        const res = await response.json()
        if (res.code === 0) {
          setLogs(res.data || 'No logs available')
        } else {
          message.error(res.message || '获取日志失败')
        }
      } else {
        // 纯文本响应（日志内容）
        const text = await response.text()
        if (response.ok) {
          setLogs(text || 'No logs available')
        } else {
          message.error('获取日志失败')
        }
      }
    } catch (e) {
      console.error(e)
      message.error('获取日志失败')
    } finally {
      setLoading(false)
    }
  }

  const startAutoRefresh = () => {
    refreshTimerRef.current = setInterval(() => {
      fetchLogs()
    }, 5000)
  }

  const stopAutoRefresh = () => {
    if (refreshTimerRef.current) {
      clearInterval(refreshTimerRef.current)
      refreshTimerRef.current = null
    }
  }

  const highlightLogs = () => {
    if (!searchText) {
      setHighlightedLogs(logs)
      return
    }

    const regex = new RegExp(`(${searchText.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi')
    const highlighted = logs.replace(regex, '<mark style="background: #ffd700; padding: 0 2px; border-radius: 2px;">$1</mark>')
    setHighlightedLogs(highlighted)
  }

  const handleDownload = () => {
    const blob = new Blob([logs], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${podName}-${new Date().toISOString()}.log`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const scrollToTop = () => {
    logContainerRef.current?.scrollTo(0, 0)
  }

  const scrollToBottom = () => {
    logContainerRef.current?.scrollTo(0, logContainerRef.current.scrollHeight)
  }

  return (
    <Modal
      title={`日志 - ${podName}${containerName ? ` (${containerName})` : ''}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={1000}
      style={{ top: 20 }}
    >
      <div style={{ marginBottom: 16 }}>
        <Space wrap>
          <Select
            value={tailLines}
            onChange={setTailLines}
            style={{ width: 120 }}
            options={[
              { label: '100 行', value: 100 },
              { label: '500 行', value: 500 },
              { label: '1000 行', value: 1000 },
              { label: '5000 行', value: 5000 },
            ]}
          />
          <Input
            placeholder="搜索..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
          />
          <Switch
            checked={autoRefresh}
            onChange={setAutoRefresh}
            checkedChildren="自动刷新"
            unCheckedChildren="手动"
          />
          <Button icon={<SyncOutlined />} onClick={fetchLogs}>
            刷新
          </Button>
          <Button icon={<DownloadOutlined />} onClick={handleDownload}>
            下载
          </Button>
          <Button icon={<ArrowUpOutlined />} onClick={scrollToTop}>
            顶部
          </Button>
          <Button icon={<ArrowDownOutlined />} onClick={scrollToBottom}>
            底部
          </Button>
        </Space>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 50 }}>
          <Spin size="large" />
        </div>
      ) : (
        <pre
          ref={logContainerRef}
          style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 8,
            height: 500,
            overflow: 'auto',
            fontSize: 13,
            fontFamily: 'Consolas, Monaco, monospace',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
          }}
          dangerouslySetInnerHTML={{ __html: highlightedLogs }}
        />
      )}
    </Modal>
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

export default LogViewer
