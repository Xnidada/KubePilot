import { Alert } from 'antd'
import { useAuthStore } from '../stores/auth'

type Props = {
  /** Defaults to aiops execute. Use aiops_config for settings page. */
  resource?: string
  action?: string
  style?: React.CSSProperties
}

/** Banner for users who can view AI pages but cannot run mutating AI actions. */
export function AIReadOnlyBanner({
  resource = 'aiops',
  action = 'execute',
  style,
}: Props) {
  const { hasPermission } = useAuthStore()
  const canView = resource === 'aiops_config'
    ? hasPermission('aiops_config', 'view') || hasPermission('aiops', 'view')
    : hasPermission('aiops', 'view')
  const canAct = hasPermission(resource, action)

  if (!canView || canAct) return null

  return (
    <Alert
      type="info"
      showIcon
      style={{ marginBottom: 16, ...style }}
      message="只读模式"
      description="可浏览 AI 智能相关页面与历史内容，但不能执行对话、诊断、工具调用或修改配置。"
    />
  )
}
