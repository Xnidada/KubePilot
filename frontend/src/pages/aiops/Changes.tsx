import { useEffect, useState } from 'react'
import { Alert, Button, Card, message, Modal, Space, Table, Tag, Typography } from 'antd'
import { AgentChange, cancelAgentChange, confirmK8SOperation, decideAgentChange, exportAgentChange, listAgentChanges } from '../../api/agent'
import { getAgentApprovalSettings } from '../../api/aiops'

const { Text, Title } = Typography

const labels: Record<string, string> = {
  pending: '待提交', approval_pending: '待他人审批', approved: '已批准待执行',
  rejected: '已拒绝', executing: '执行中', executed: '已执行',
  rollback_pending: '已提交回滚待核验', rolled_back: '已回滚', failed: '失败', cancelled: '已取消',
}

function preview(change: AgentChange) {
  return <div style={{ maxHeight: 420, overflow: 'auto' }}>
    <Text strong>{change.action} {change.namespace}/{change.resource_name}</Text>
    <pre style={{ whiteSpace: 'pre-wrap' }}>{change.dry_run || '无预览'}</pre>
    <Text type="secondary">诊断依据：{change.evidence || '待提交时记录'}</Text>
  </div>
}

const Changes: React.FC = () => {
  const [mine, setMine] = useState<AgentChange[]>([])
  const [approvals, setApprovals] = useState<AgentChange[]>([])
  const [busy, setBusy] = useState(false)
  const [approvalEnabled, setApprovalEnabled] = useState<boolean | null>(null)

  const load = async () => {
    void getAgentApprovalSettings().then(res => setApprovalEnabled(res.data.enabled)).catch(() => setApprovalEnabled(null))
    try {
      const res = await listAgentChanges()
      setMine(res.data?.mine || [])
      setApprovals(res.data?.awaiting_my_approval || [])
    } catch { message.error('无法加载变更记录') }
  }
  useEffect(() => { void load() }, [])

  const download = async (id: number) => {
    try {
      const data = await exportAgentChange(id)
      const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }))
      const link = document.createElement('a')
      link.href = url
      link.download = `kubepilot-change-${id}.json`
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch { message.error('导出失败') }
  }

  const decide = (change: AgentChange, decision: 'approve' | 'reject') => {
    Modal.confirm({
      title: decision === 'approve' ? '批准此生产变更？' : '拒绝此生产变更？',
      width: 700,
      content: preview(change),
      okText: decision === 'approve' ? '确认批准' : '确认拒绝',
      okButtonProps: { danger: decision === 'reject' },
      onOk: async () => {
        setBusy(true)
        try { await decideAgentChange(change.id, decision); message.success('审批已记录'); await load() }
        finally { setBusy(false) }
      },
    })
  }

  const execute = (change: AgentChange) => {
    Modal.confirm({
      title: change.status === 'pending' ? '提交或执行变更？' : '执行已审批变更？', width: 700, content: preview(change), okText: change.status === 'pending' ? '确认提交' : '执行并观察',
      onOk: async () => {
        setBusy(true)
        try {
          const res = await confirmK8SOperation(change.id)
          message.info(res.data?.message || '执行完成')
        } catch { message.error('执行失败；请查看变更记录和回滚状态') }
        finally { setBusy(false); await load() }
      },
    })
  }

  const cancel = (change: AgentChange) => {
    Modal.confirm({
      title: '取消此变更？', content: preview(change), okText: '确认取消', okButtonProps: { danger: true },
      onOk: async () => {
        setBusy(true)
        try { await cancelAgentChange(change.id); message.success('变更已取消'); await load() }
        finally { setBusy(false) }
      },
    })
  }

  const common = [
    { title: 'ID', dataIndex: 'id' },
    { title: '集群', dataIndex: 'cluster_id' },
    { title: '变更', render: (_: unknown, row: AgentChange) => `${row.action} ${row.namespace}/${row.resource_name}` },
    { title: '状态', render: (_: unknown, row: AgentChange) => <Tag>{labels[row.status] || row.status}</Tag> },
    { title: '预览', render: (_: unknown, row: AgentChange) => <Button type="link" onClick={() => Modal.info({ title: '变更预览与证据', width: 700, content: preview(row) })}>查看</Button> },
  ]

  return <div style={{ padding: 24 }}>
    <Title level={3}>变更中心</Title>
    <Alert type="info" showIcon message={approvalEnabled === null ? '审批开关状态暂不可用，请以服务端校验为准。' :
      `生产集群双人审批已${approvalEnabled ? '开启' : '关闭'}；仅 Deployment 创建、扩缩容和更新支持自动观察与回滚。其他写操作需独立变更预案。`} style={{ marginBottom: 16 }} />
    <Card title={`待我审批（${approvals.length}）`} style={{ marginBottom: 16 }}>
      <Table rowKey="id" dataSource={approvals} pagination={{ pageSize: 10 }} columns={[...common,
        { title: '操作', render: (_: unknown, row: AgentChange) => <Space>
          <Button type="link" disabled={busy} onClick={() => decide(row, 'approve')}>批准</Button>
          <Button type="link" danger disabled={busy} onClick={() => decide(row, 'reject')}>拒绝</Button>
          <Button type="link" onClick={() => download(row.id)}>导出审计</Button>
        </Space> },
      ]} />
    </Card>
    <Card title="我的变更">
      <Table rowKey="id" dataSource={mine} pagination={{ pageSize: 10 }} columns={[...common,
        { title: '操作', render: (_: unknown, row: AgentChange) => <Space>
          {(row.status === 'pending' || row.status === 'approved') && <Button type="link" disabled={busy} onClick={() => execute(row)}>{row.status === 'pending' ? '提交/执行' : '执行并观察'}</Button>}
          {['pending', 'approval_pending', 'approved'].includes(row.status) && <Button type="link" danger disabled={busy} onClick={() => cancel(row)}>取消</Button>}
          <Button type="link" onClick={() => download(row.id)}>导出审计</Button>
        </Space> },
      ]} />
    </Card>
  </div>
}

export default Changes
