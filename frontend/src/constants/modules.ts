/** Shared module deep-links for Modules page / Dashboard / alerts. */
export const MODULE_LINKS: Record<string, { path: string; label: string }[]> = {
  backup: [
    { path: '/system/backup?tab=schedules', label: '备份计划' },
    { path: '/system/backup?tab=backups', label: '备份记录' },
    { path: '/system/backup?tab=restores', label: '恢复记录' },
  ],
  eventforward: [
    { path: '/cluster/event-forward?tab=rules', label: '转发规则' },
    { path: '/cluster/event-forward?tab=logs', label: '转发日志' },
  ],
  inspection: [
    { path: '/cluster/inspection?tab=rules', label: '巡检规则' },
    { path: '/cluster/inspection?tab=reports', label: '巡检报告' },
  ],
  webhook: [
    { path: '/system/webhooks?tab=webhooks', label: 'Webhook 配置' },
    { path: '/system/webhooks?tab=logs', label: '调用日志' },
  ],
  aiops: [
    { path: '/aiops/agent', label: 'AI Agent' },
    { path: '/aiops/diagnosis', label: '智能诊断' },
    { path: '/aiops/settings', label: 'AI 设置' },
  ],
  scheduler: [
    { path: '/scheduler/tasks', label: '调度任务' },
    { path: '/scheduler/queues', label: '调度队列' },
  ],
  appstore: [{ path: '/appstore', label: '应用商店' }],
}

/** Primary ops page for a module (first deep link, or modules hub). */
export function moduleHomePath(name: string): string {
  const links = MODULE_LINKS[name]
  if (links && links.length > 0) return links[0].path
  return '/system/modules'
}
