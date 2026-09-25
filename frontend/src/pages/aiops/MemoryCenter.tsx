import { useEffect, useState } from 'react'
import { Button, Card, Col, Form, Input, Modal, Popconfirm, Row, Select, Statistic, Table, Tag, message } from 'antd'
import { DeleteOutlined, PushpinOutlined, PlusOutlined } from '@ant-design/icons'
import { AgentMemory, MemoryMetrics, batchForgetAgentMemories, createAgentMemory, forgetAgentMemory, getMemoryMetrics, listAgentMemories, pinAgentMemory } from '../../api/aiops'
import { useAuthStore } from '../../stores/auth'

const MemoryCenter = () => {
  const { hasPermission, user } = useAuthStore(); const canCreate = hasPermission('aiops', 'create'); const canEdit = hasPermission('aiops', 'edit'); const canDelete = hasPermission('aiops', 'delete')
  const [rows, setRows] = useState<AgentMemory[]>([]); const [metrics, setMetrics] = useState<MemoryMetrics | null>(null); const [open, setOpen] = useState(false); const [form] = Form.useForm()
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([])
  const [deleting, setDeleting] = useState(false)
  const canForget = (row: AgentMemory) => canDelete && row.user_id === user?.id && !row.is_pinned
  const load = async () => { try { const [m, s] = await Promise.all([listAgentMemories(), getMemoryMetrics()]); if (m.code === 0) { const items = m.data || []; setRows(items); const selectable = new Set(items.filter(canForget).map((r) => r.id)); setSelectedKeys((keys) => keys.filter((key) => selectable.has(Number(key)))) }; if (s.code === 0) setMetrics(s.data) } catch { message.error('加载记忆中心失败') } }
  useEffect(() => { load() }, [])
  const create = async () => { const v = await form.validateFields(); await createAgentMemory(v); message.success('记忆已保存'); form.resetFields(); setOpen(false); load() }
  const forget = async (ids: number[]) => {
    if (ids.length === 0) return
    setDeleting(true)
    try {
      if (ids.length === 1) await forgetAgentMemory(ids[0])
      else await batchForgetAgentMemories(ids)
      message.success(`已遗忘 ${ids.length} 条记忆`)
      setSelectedKeys([])
      await load()
    } catch { message.error('遗忘失败') }
    finally { setDeleting(false) }
  }
  return <div>
    <Card title="记忆中心" extra={canCreate && <Button icon={<PlusOutlined />} type="primary" onClick={() => setOpen(true)}>新增记忆</Button>}>
      <Row gutter={16}>{[['会话轮次', metrics?.runs], ['平均 Prompt Token', Math.round(metrics?.avg_prompt_tokens || 0)], ['上下文命中率', `${Math.round((metrics?.context_hit_rate || 0) * 100)}%`], ['重复工具调用率', `${Math.round((metrics?.repeated_tool_call_rate || 0) * 100)}%`], ['用户纠正率', `${Math.round((metrics?.user_correction_rate || 0) * 100)}%`], ['错误陈述率', `${Math.round((metrics?.error_assertion_rate || 0) * 100)}%`], ['平均延迟', `${Math.round(metrics?.avg_latency_ms || 0)} ms`]].map(([t,v]) => <Col span={3} key={String(t)}><Statistic title={t} value={v as string | number} /></Col>)}</Row>
    </Card>
    <Card style={{ marginTop: 16 }} title="长期记忆" extra={canDelete && <Popconfirm title={`遗忘选中的 ${selectedKeys.length} 条记忆？`} onConfirm={() => forget(selectedKeys.map(Number))} okText="遗忘" okButtonProps={{ danger: true }}><Button danger icon={<DeleteOutlined />} disabled={selectedKeys.length === 0 || deleting}>遗忘所选{selectedKeys.length > 0 ? ` (${selectedKeys.length})` : ''}</Button></Popconfirm>}><Table rowKey="id" dataSource={rows} pagination={{ pageSize: 10 }} rowSelection={canDelete ? { selectedRowKeys: selectedKeys, preserveSelectedRowKeys: true, onChange: (keys) => { if (keys.length > 100) message.warning('每次最多选择 100 条'); setSelectedKeys(keys.slice(0, 100)) }, getCheckboxProps: (r) => ({ disabled: !canForget(r) }) } : undefined} columns={[
      { title: '类型', dataIndex: 'type', render: (v) => <Tag color={v === 'preference' ? 'blue' : 'green'}>{v === 'preference' ? '用户偏好' : '已验证运维知识'}</Tag> },
      { title: '内容', dataIndex: 'content' }, { title: '来源', dataIndex: 'source' }, { title: '置信度', dataIndex: 'confidence', render: (v) => `${Math.round(v * 100)}%` }, { title: '有效期', dataIndex: 'expires_at', render: (v) => v || '长期' },
      { title: '操作', render: (_, r) => <>{canEdit && r.user_id === user?.id && <Button type="link" icon={<PushpinOutlined />} onClick={async () => { await pinAgentMemory(r.id); load() }}>{r.is_pinned ? '取消固定' : '固定'}</Button>}{canForget(r) && <Popconfirm title="遗忘这条记忆？" onConfirm={() => forget([r.id])}><Button type="link" danger icon={<DeleteOutlined />} disabled={deleting}>遗忘</Button></Popconfirm>}</> }
    ]} /></Card>
    <Modal title="新增长期记忆" open={open} onOk={create} onCancel={() => setOpen(false)}><Form form={form} layout="vertical" initialValues={{ type: 'preference', confidence: 0.8 }}><Form.Item name="type" label="类型"><Select options={[{ value: 'preference', label: '用户偏好' }, { value: 'verified_knowledge', label: '已验证运维知识' }]} /></Form.Item><Form.Item name="content" label="内容" rules={[{ required: true }]}><Input.TextArea rows={4} /></Form.Item><Form.Item name="confidence" label="置信度"><Input type="number" /></Form.Item></Form></Modal>
  </div>
}
export default MemoryCenter
