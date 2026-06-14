import { Tag } from 'antd'
import {
  CheckCircleOutlined,
  SyncOutlined,
  CloseCircleOutlined,
  ClockCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons'

interface StatusTagProps {
  status: string
}

const StatusTag: React.FC<StatusTagProps> = ({ status }) => {
  const getStatusConfig = (status: string) => {
    const config: Record<string, { color: string; icon: React.ReactNode; text: string }> = {
      // 通用状态
      Active: { color: 'success', icon: <CheckCircleOutlined />, text: 'Active' },
      Running: { color: 'success', icon: <CheckCircleOutlined />, text: 'Running' },
      Ready: { color: 'success', icon: <CheckCircleOutlined />, text: 'Ready' },
      Bound: { color: 'success', icon: <CheckCircleOutlined />, text: 'Bound' },
      Connected: { color: 'success', icon: <CheckCircleOutlined />, text: 'Connected' },
      Deployed: { color: 'success', icon: <CheckCircleOutlined />, text: 'Deployed' },

      // 中间状态
      Terminating: { color: 'warning', icon: <LoadingOutlined spin />, text: 'Terminating' },
      Updating: { color: 'processing', icon: <SyncOutlined spin />, text: 'Updating' },
      Pending: { color: 'warning', icon: <ClockCircleOutlined />, text: 'Pending' },
      Creating: { color: 'processing', icon: <LoadingOutlined spin />, text: 'Creating' },
      Succeeded: { color: 'default', icon: <CheckCircleOutlined />, text: 'Succeeded' },
      Unknown: { color: 'default', icon: <ExclamationCircleOutlined />, text: 'Unknown' },

      // 错误状态
      Failed: { color: 'error', icon: <CloseCircleOutlined />, text: 'Failed' },
      Error: { color: 'error', icon: <CloseCircleOutlined />, text: 'Error' },
      CrashLoopBackOff: { color: 'error', icon: <CloseCircleOutlined />, text: 'CrashLoopBackOff' },
      NotReady: { color: 'error', icon: <CloseCircleOutlined />, text: 'NotReady' },
      Lost: { color: 'error', icon: <CloseCircleOutlined />, text: 'Lost' },
      Disconnected: { color: 'error', icon: <CloseCircleOutlined />, text: 'Disconnected' },
    }

    return config[status] || { color: 'default', icon: null, text: status }
  }

  const config = getStatusConfig(status)

  return (
    <Tag color={config.color} icon={config.icon}>
      {config.text}
    </Tag>
  )
}

export default StatusTag
