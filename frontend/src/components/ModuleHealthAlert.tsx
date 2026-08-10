import { Alert, Button, Space } from 'antd'
import { useNavigate } from 'react-router-dom'
import { ModuleStatus } from '../api/modules'
import { useModuleStatus } from '../hooks/useModuleStatus'
import { moduleHomePath } from '../constants/modules'

type Props = {
  module: string
  title?: string
  /** Navigate here from the alert action */
  fixPath?: string
  fixLabel?: string
}

/** Shows an error/warning banner when a module is unhealthy or has a health_warning in details. */
export function ModuleHealthAlert({ module, title, fixPath, fixLabel }: Props) {
  const navigate = useNavigate()
  const status = useModuleStatus(module)
  const target = fixPath || moduleHomePath(module)

  if (!status || !status.enabled) return null

  const warning = typeof status.details?.health_warning === 'string'
    ? String(status.details.health_warning)
    : ''

  if (status.healthy && !warning) return null

  const msg = title || `${status.name} 模块健康检查`
  const desc = status.healthy ? warning : (status.health_error || warning || '模块异常')

  return (
    <Alert
      type={status.healthy ? 'warning' : 'error'}
      showIcon
      style={{ marginBottom: 16 }}
      message={msg}
      description={
        <Space direction="vertical" size={4}>
          <span>{desc}</span>
          <Button type="link" size="small" style={{ padding: 0 }} onClick={() => navigate(target)}>
            {fixLabel || '前往处理'}
          </Button>
        </Space>
      }
    />
  )
}

export type { ModuleStatus }
