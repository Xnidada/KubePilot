import { useEffect, useState } from 'react'
import { Button, Card, Col, DatePicker, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Statistic, Table, Tag, Typography, message } from 'antd'
import { DeleteOutlined, PushpinOutlined, PlusOutlined } from '@ant-design/icons'
import { AgentMemory, AgentMemoryAudit, MemoryMetrics, MetricReviewSample, batchForgetAgentMemories, createAgentMemory, forgetAgentMemory, getMemoryMetrics, getMetricReviewSamples, listAgentMemories, listAgentMemoryAudits, pinAgentMemory, reviewMetricAssertion } from '../../api/aiops'
import { useAuthStore } from '../../stores/auth'

const MemoryCenter = () => {
  const { hasPermission, user } = useAuthStore(); const canCreate = hasPermission('aiops', 'create'); const canEdit = hasPermission('aiops', 'edit'); const canDelete = hasPermission('aiops', 'delete'); const canReview = hasPermission('aiops_config', 'edit')
  const [rows, setRows] = useState<AgentMemory[]>([]); const [metrics, setMetrics] = useState<MemoryMetrics | null>(null); const [open, setOpen] = useState(false); const [form] = Form.useForm()
  const [audits, setAudits] = useState<AgentMemoryAudit[]>([])
  const [samples, setSamples] = useState<MetricReviewSample[]>([])
  const memoryType = Form.useWatch('type', form)
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([])
  const [deleting, setDeleting] = useState(false)
  const canForget = (row: AgentMemory) => canDelete && row.user_id === user?.id && !row.is_pinned
  const load = async () => { try { const [m, s, a] = await Promise.all([listAgentMemories(), getMemoryMetrics(), listAgentMemoryAudits()]); if (m.code === 0) { const items = m.data || []; setRows(items); const selectable = new Set(items.filter(canForget).map((r) => r.id)); setSelectedKeys((keys) => keys.filter((key) => selectable.has(Number(key)))) }; if (s.code === 0) setMetrics(s.data); if (a.code === 0) setAudits(a.data || []); if (canReview) { const review = await getMetricReviewSamples(); if (review.code === 0) setSamples(review.data || []) } } catch { message.error('加载记忆中心失败') } }
  useEffect(() => { load() }, [])
  const create = async () => { try { const v = await form.validateFields(); const result = await createAgentMemory({ ...v, expires_at: v.expires_at?.toISOString() }); if (result.code !== 0) throw new Error('create failed'); message.success('记忆已保存'); form.resetFields(); setOpen(false); await load() } catch (err) { if (err && typeof err === 'object' && 'errorFields' in err) return; message.error('保存记忆失败，请检查内容及核验来源') } }
  const review = async (id: number, errorAssertion: boolean) => { try { await reviewMetricAssertion(id, errorAssertion); message.success('审核结果已记录'); await load() } catch { message.error('审核失败') } }
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
      <Row gutter={16}>{[['会话轮次', metrics?.runs ?? 0], ['平均已上报 Prompt Token', Math.round(metrics?.avg_prompt_tokens ?? 0)], ['记忆召回轮次占比', `${Math.round((metrics?.context_hit_rate ?? 0) * 100)}%`], ['同轮重复工具调用率', `${Math.round((metrics?.repeated_tool_call_rate ?? 0) * 100)}%`], ['用户纠正率（关键词估算）', `${Math.round((metrics?.user_correction_rate ?? 0) * 100)}%`], ['错误陈述率（人工审核）', metrics?.error_assertion_rate == null ? '未评估' : `${Math.round(metrics.error_assertion_rate * 100)}%`], ['平均端到端延迟', `${Math.round(metrics?.avg_latency_ms ?? 0)} ms`]].map(([t,v]) => <Col span={3} key={String(t)}><Statistic title={t} value={v as string | number} /></Col>)}</Row>
      <Typography.Text type="secondary">错误陈述已审核 {metrics?.error_assertion_reviewed ?? 0} 轮；记忆召回不代表回答正确。</Typography.Text>
    </Card>
    <Card style={{ marginTop: 16 }} title="长期记忆" extra={canDelete && <Popconfirm title={`遗忘选中的 ${selectedKeys.length} 条记忆？`} onConfirm={() => forget(selectedKeys.map(Number))} okText="遗忘" okButtonProps={{ danger: true }}><Button danger icon={<DeleteOutlined />} disabled={selectedKeys.length === 0 || deleting}>遗忘所选{selectedKeys.length > 0 ? ` (${selectedKeys.length})` : ''}</Button></Popconfirm>}><Table rowKey="id" dataSource={rows} pagination={{ pageSize: 10 }} rowSelection={canDelete ? { selectedRowKeys: selectedKeys, preserveSelectedRowKeys: true, onChange: (keys) => { if (keys.length > 100) message.warning('每次最多选择 100 条'); setSelectedKeys(keys.slice(0, 100)) }, getCheckboxProps: (r) => ({ disabled: !canForget(r) }) } : undefined} columns={[
      { title: '类型', dataIndex: 'type', render: (v: string, r: AgentMemory) => <Tag color={v === 'preference' ? 'blue' : r.source.startsWith('reviewed:') ? 'green' : 'orange'}>{v === 'preference' ? '用户偏好' : r.source.startsWith('reviewed:') ? '已验证运维知识' : '历史待核验知识'}</Tag> },
      { title: '内容', dataIndex: 'content' }, { title: '来源', dataIndex: 'source', render: (v: string) => v.startsWith('reviewed:') ? v.slice(9) : v }, { title: '置信度', dataIndex: 'confidence', render: (v) => `${Math.round(v * 100)}%` }, { title: '有效期', dataIndex: 'expires_at', render: (v) => v || '长期' },
      { title: '操作', render: (_, r) => <>{canEdit && r.user_id === user?.id && <Button type="link" icon={<PushpinOutlined />} onClick={async () => { await pinAgentMemory(r.id); load() }}>{r.is_pinned ? '取消固定' : '固定'}</Button>}{canForget(r) && <Popconfirm title="遗忘这条记忆？" onConfirm={() => forget([r.id])}><Button type="link" danger icon={<DeleteOutlined />} disabled={deleting}>遗忘</Button></Popconfirm>}</> }
    ]} /></Card>
    <Card style={{ marginTop: 16 }} title="记忆操作审计（最近 100 条）"><Table rowKey="id" dataSource={audits} pagination={{ pageSize: 10 }} columns={[{ title: '时间', dataIndex: 'created_at' }, { title: '记忆 ID', dataIndex: 'memory_id' }, { title: '操作人 ID', dataIndex: 'actor_id' }, { title: '操作', dataIndex: 'action' }, { title: '详情', dataIndex: 'detail' }]} /></Card>
    {canReview && <Card style={{ marginTop: 16 }} title="错误陈述人工审核（最近 20 条待审）"><Table rowKey="id" dataSource={samples} pagination={{ pageSize: 5 }} columns={[{ title: '会话 ID', dataIndex: 'conversation_id' }, { title: '用户问题', dataIndex: 'question', render: (v: string) => <Typography.Paragraph ellipsis={{ rows: 3, expandable: true }}>{v}</Typography.Paragraph> }, { title: 'AI 回答', dataIndex: 'answer', render: (v: string) => <Typography.Paragraph ellipsis={{ rows: 3, expandable: true }}>{v}</Typography.Paragraph> }, { title: '工具证据', dataIndex: 'evidence', render: (v: string) => <Typography.Paragraph ellipsis={{ rows: 3, expandable: true }}>{v || '请查看原会话工具轨迹'}</Typography.Paragraph> }, { title: '审核', render: (_: unknown, r: MetricReviewSample) => <><Popconfirm title="确认这轮回答没有错误陈述？" onConfirm={() => review(r.id, false)}><Button type="link">无错误</Button></Popconfirm><Popconfirm title="确认这轮回答有错误陈述？" onConfirm={() => review(r.id, true)}><Button type="link" danger>有错误</Button></Popconfirm></> }]} /><Typography.Text type="secondary">请对照工具证据和真实集群状态核验；每轮只能标注一次。</Typography.Text></Card>}
    <Modal title="新增长期记忆" open={open} onOk={create} onCancel={() => setOpen(false)}><Form form={form} layout="vertical" initialValues={{ type: 'preference', confidence: 0.8 }}><Form.Item name="type" label="类型"><Select options={[{ value: 'preference', label: '用户偏好' }, { value: 'verified_knowledge', label: '已验证运维知识' }]} /></Form.Item><Form.Item name="content" label="内容" rules={[{ required: true, max: 800 }]}><Input.TextArea rows={4} /></Form.Item><Form.Item name="source" label="人工核验来源" rules={[{ required: memoryType === 'verified_knowledge', message: '已验证运维知识须填写核验来源' }]}><Input placeholder="例如：变更单编号、巡检报告或 kubectl 查询结果" /></Form.Item><Form.Item name="cluster_id" label="集群 ID（可选）"><InputNumber min={1} style={{ width: '100%' }} /></Form.Item><Form.Item name="expires_at" label="有效期（知识默认 30 天）"><DatePicker showTime style={{ width: '100%' }} /></Form.Item><Form.Item name="confidence" label="置信度"><InputNumber min={0.1} max={1} step={0.1} style={{ width: '100%' }} /></Form.Item></Form></Modal>
  </div>
}
export default MemoryCenter
