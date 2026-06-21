import { useEffect, useRef, useState } from 'react'
import { Modal, Spin } from 'antd'
import { Terminal as XTerminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'

interface TerminalProps {
  visible: boolean
  onClose: () => void
  clusterId: number
  namespace: string
  podName: string
  containerName?: string
}

const Terminal: React.FC<TerminalProps> = ({
  visible,
  onClose,
  clusterId,
  namespace,
  podName,
  containerName,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [connecting, setConnecting] = useState(false)
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Suppress unused warning - connected is used in WebSocket callbacks
  void connected

  useEffect(() => {
    if (visible) {
      connectTerminal()
    }
    return () => {
      cleanup()
    }
  }, [visible, containerName])

  const connectTerminal = () => {
    cleanup()
    setConnecting(true)
    setError(null)

    if (!terminalRef.current) return

    // 创建终端
    const term = new XTerminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Consolas, Monaco, monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)
    fitAddon.fit()

    termRef.current = term

    // 获取 token
    const token = getAuthToken()

    // 构建 WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws/terminal/${clusterId}/${namespace}/${podName}?container=${containerName || ''}&token=${token}`

    // 连接 WebSocket
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      setConnecting(false)
      setConnected(true)
      term.writeln('\x1b[32m✓ 已连接到终端\x1b[0m')
      term.writeln('')
    }

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        term.write(event.data)
      } else if (event.data instanceof Blob) {
        event.data.text().then(text => term.write(text))
      }
    }

    ws.onerror = () => {
      setConnecting(false)
      setError('连接失败')
      term.writeln('\x1b[31m✗ 连接失败\x1b[0m')
    }

    ws.onclose = () => {
      setConnected(false)
      term.writeln('\r\n\x1b[33m连接已关闭\x1b[0m')
    }

    // 终端输入发送到 WebSocket
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data)
      }
    })

    // 窗口大小调整
    const handleResize = () => fitAddon.fit()
    window.addEventListener('resize', handleResize)
  }

  const cleanup = () => {
    if (termRef.current) {
      termRef.current.dispose()
      termRef.current = null
    }
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    setConnected(false)
    setConnecting(false)
  }

  const handleClose = () => {
    cleanup()
    onClose()
  }

  return (
    <Modal
      title={`终端 - ${podName}${containerName ? ` (${containerName})` : ''}`}
      open={visible}
      onCancel={handleClose}
      footer={null}
      width={900}
      styles={{ body: { padding: 0 } }}
    >
      <div
        ref={terminalRef}
        style={{
          height: 500,
          background: '#1e1e1e',
        }}
      />
      {connecting && (
        <div style={{ padding: 16, textAlign: 'center' }}>
          <Spin tip="连接中..." />
        </div>
      )}
      {error && (
        <div style={{ padding: 16, color: '#ff4d4f' }}>
          {error}
        </div>
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

export default Terminal
