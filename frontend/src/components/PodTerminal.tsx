import { useEffect, useRef, useState } from 'react'
import { Modal, Select } from 'antd'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'

interface PodTerminalProps {
  visible: boolean
  onClose: () => void
  clusterId: number
  namespace: string
  podName: string
}

const PodTerminal: React.FC<PodTerminalProps> = ({
  visible,
  onClose,
  clusterId,
  namespace,
  podName,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [containers, setContainers] = useState<{ name: string; image: string }[]>([])
  const [selectedContainer, setSelectedContainer] = useState<string>('')

  useEffect(() => {
    if (visible) {
      fetchContainers()
    }
    return () => {
      cleanup()
    }
  }, [visible])

  useEffect(() => {
    if (visible && selectedContainer && terminalRef.current) {
      initTerminal()
    }
  }, [visible, selectedContainer])

  const fetchContainers = async () => {
    try {
      const token = getAuthToken()
      const response = await fetch(
        `/api/v1/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/containers`,
        {
          headers: { 'Authorization': `Bearer ${token}` },
        }
      )
      const data = await response.json()
      if (data.code === 0 && data.data) {
        setContainers(data.data)
        if (data.data.length > 0) {
          setSelectedContainer(data.data[0].name)
        }
      }
    } catch (error) {
      console.error('Failed to fetch containers:', error)
    }
  }

  const getAuthToken = () => {
    const token = localStorage.getItem('auth-storage')
    if (token) {
      try {
        const authData = JSON.parse(token)
        return authData?.state?.token
      } catch {
        return ''
      }
    }
    return ''
  }

  const initTerminal = () => {
    cleanup()

    if (!terminalRef.current) return

    // 创建终端
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Consolas, Monaco, monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)
    fitAddon.fit()

    termRef.current = term

    // 连接WebSocket
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws/terminal/${clusterId}/${namespace}/${podName}?container=${selectedContainer}`

    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      term.writeln('Connected to pod: ' + podName)
      term.writeln('')
    }

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        term.write(event.data)
      }
    }

    ws.onerror = (error) => {
      console.error('WebSocket error:', error)
      term.writeln('\r\nConnection error')
    }

    ws.onclose = () => {
      term.writeln('\r\nConnection closed')
    }

    // 终端输入发送到WebSocket
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data)
      }
    })

    // 窗口大小调整
    const handleResize = () => {
      fitAddon.fit()
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
    }
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
  }

  const handleClose = () => {
    cleanup()
    onClose()
  }

  return (
    <Modal
      title={`终端 - ${podName}`}
      open={visible}
      onCancel={handleClose}
      footer={null}
      width={900}
      styles={{ body: { padding: 0 } }}
    >
      <div style={{ padding: '8px 16px', background: '#f5f5f5', borderBottom: '1px solid #e8e8e8' }}>
        <span style={{ marginRight: 16 }}>容器:</span>
        <Select
          value={selectedContainer}
          onChange={setSelectedContainer}
          style={{ width: 300 }}
          options={containers.map(c => ({
            label: `${c.name} (${c.image})`,
            value: c.name,
          }))}
        />
      </div>
      <div
        ref={terminalRef}
        style={{
          height: 500,
          padding: 8,
          background: '#1e1e1e',
        }}
      />
    </Modal>
  )
}

export default PodTerminal
