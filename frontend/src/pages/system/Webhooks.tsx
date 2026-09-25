import { useState, useEffect, useCallback } from 'react'
import {
  Card, Table, Button, Space, Typography, message, Tag, Modal, Form, Input, Select,
  Switch, Popconfirm, Tabs
} from 'antd'
import {
  PlusOutlined, DeleteOutlined, EditOutlined, SendOutlined,
  HistoryOutlined, BellOutlined, ReloadOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { get, post, put, del } from '../../api/request'
import { useQueryTab } from '../../hooks/useQueryTab'
import { useInterval } from '../../hooks/useInterval'
import { ModuleHealthAlert } from '../../components/ModuleHealthAlert'
import { useAuthStore } from '../../stores/auth'

const { Title, Text } = Typography

const WEBHOOK_TABS = ['webhooks', 'logs'] as const
const REFRESH_MS = 10000

interface Webhook {
  id: number
  name: string
  type: string
  url: string
  events: string
  namespaces: string
  severity: string
  enabled: boolean
  last_fired_at: string
  created_at: string
}

interface WebhookLog {
  id: number
  webhook_id: number
  event_type: string
  status: string
  status_code: number
  error: string
  created_at: string
}

const Webhooks: React.FC = () => {
  const [webhooks, setWebhooks] = useState<Webhook[]>([])
  const [logs, setLogs] = useState<WebhookLog[]>([])
  const [logPage, setLogPage] = useState(1)
  const [logTotal, setLogTotal] = useState(0)
  const [selectedLogKeys, setSelectedLogKeys] = useState<React.Key[]>([])
  const [deletingLogs, setDeletingLogs] = useState(false)
  const canDeleteLogs = useAuthStore((state) => state.hasPermission('webhooks', 'delete'))
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [activeTab, setActiveTab] = useQueryTab(WEBHOOK_TABS, 'webhooks')
  const [form] = Form.useForm()

  const fetchWebhooks = useCallback(async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: Webhook[] }>('/webhooks')
      setWebhooks(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }, [])

  const fetchLogs = useCallback(async (page = 1) => {
    try {
      const res = await get<{ code: number; data: WebhookLog[]; total: number }>('/webhooks/logs', { params: { page, size: 20 } })
      setLogs(res.data || [])
      setLogTotal(res.total || 0)
      setLogPage(page)
    } catch (e) { console.error(e) }
  }, [])

  useEffect(() => { fetchWebhooks(); fetchLogs() }, [fetchWebhooks, fetchLogs])

  useInterval(() => {
    if (activeTab === 'logs' && !deletingLogs) fetchLogs(logPage)
    else fetchWebhooks()
  }, REFRESH_MS, autoRefresh)

  const handleCreate = () => {
    setEditingWebhook(null)
    form.resetFields()
    form.setFieldsValue({ enabled: true, events: ['alert'] })
    setModalVisible(true)
  }

  const handleEdit = (webhook: Webhook) => {
    setEditingWebhook(webhook)
    form.setFieldsValue({
      ...webhook,
      events: webhook.events ? JSON.parse(webhook.events) : [],
      namespaces: webhook.namespaces ? JSON.parse(webhook.namespaces) : [],
    })
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      const data = {
        ...values,
        events: values.events || [],
        namespaces: values.namespaces || [],
      }
      if (editingWebhook) {
        await put(`/webhooks/${editingWebhook.id}`, data)
        message.success('Webhook 已更新')
      } else {
        await post('/webhooks', data)
        message.success('Webhook 已创建')
      }
      setModalVisible(false)
      form.resetFields()
      fetchWebhooks()
    } catch (e) { message.error('操作失败') }
  }

  const handleDelete = async (id: number) => {
    try {
      await del(`/webhooks/${id}`)
      message.success('Webhook 已删除')
      fetchWebhooks()
    } catch (e) { message.error('删除失败') }
  }

  const handleTest = async (id: number) => {
    try {
      await post(`/webhooks/${id}/test`)
      message.success('测试消息已发送')
      fetchLogs()
      setActiveTab('logs')
    } catch (e) { message.error('测试失败') }
  }

  const logProtected = (log: WebhookLog) => new Date(log.created_at).getTime() > Date.now() - 24 * 60 * 60 * 1000
  const handleDeleteLogs = async (ids: number[]) => {
    if (ids.length === 0) return
    setDeletingLogs(true)
    try {
      if (ids.length === 1) await del(`/webhooks/logs/${ids[0]}`)
      else await post('/webhooks/logs/batch-delete', { ids })
      message.success(`已删除 ${ids.length} 条调用日志`)
      setSelectedLogKeys([])
      await fetchLogs(1)
    } catch { message.error('删除调用日志失败') }
    finally { setDeletingLogs(false) }
  }

  const webhookColumns: ColumnsType<Webhook> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '类型', dataIndex: 'type', key: 'type',
      render: (t) => {
        const colorMap: Record<string, string> = {
          slack: '#4A154B', teams: '#6264A7', dingtalk: '#0089FF', custom: '#666'
        }
        return <Tag color={colorMap[t] || '#666'}>{t.toUpperCase()}</Tag>
      }
    },
    { title: 'URL', dataIndex: 'url', key: 'url', ellipsis: true },
    {
      title: '事件', dataIndex: 'events', key: 'events',
      render: (v) => {
        if (!v) return <Tag>全部</Tag>
        try {
          const events = JSON.parse(v)
          return events.map((e: string) => <Tag key={e}>{e}</Tag>)
        } catch { return <Tag>全部</Tag> }
      }
    },
    {
      title: '状态', dataIndex: 'enabled', key: 'enabled',
      render: (v) => <Tag color={v ? 'success' : 'default'}>{v ? '启用' : '禁用'}</Tag>
    },
    {
      title: '最后触发', dataIndex: 'last_fired_at', key: 'last_fired',
      render: (t) => t ? new Date(t).toLocaleString() : '从未'
    },
    {
      title: '操作', key: 'action', width: 150,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" icon={<SendOutlined />} onClick={() => handleTest(record.id)} />
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const logColumns: ColumnsType<WebhookLog> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '事件类型', dataIndex: 'event_type', key: 'event_type' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => <Tag color={s === 'success' ? 'success' : 'error'}>{s === 'success' ? '成功' : '失败'}</Tag>
    },
    { title: 'HTTP 状态码', dataIndex: 'status_code', key: 'status_code' },
    { title: '错误', dataIndex: 'error', key: 'error', ellipsis: true },
    {
      title: '时间', dataIndex: 'created_at', key: 'created_at',
      render: (t) => new Date(t).toLocaleString()
    },
    ...(canDeleteLogs ? [{
      title: '操作', key: 'action', width: 80,
      render: (_: unknown, log: WebhookLog) => <Popconfirm title="删除这条调用日志？" onConfirm={() => handleDeleteLogs([log.id])} disabled={logProtected(log) || deletingLogs}><Button type="link" danger icon={<DeleteOutlined />} disabled={logProtected(log) || deletingLogs} /></Popconfirm>,
    }] : []),
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}><BellOutlined /> Webhook 通知</Title>
        <Space>
          <Space size={6}>
            <Text type="secondary" style={{ fontSize: 12 }}>自动刷新</Text>
            <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
          </Space>
      <Button icon={<ReloadOutlined />} onClick={() => { fetchWebhooks(); fetchLogs(logPage) }}>
            刷新
          </Button>
        </Space>
      </div>

      <ModuleHealthAlert module="webhook" title="Webhook 模块异常" />

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'webhooks',
            label: <span><BellOutlined /> Webhook 配置</span>,
            children: (
              <Card
                extra={
                  <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
                    创建 Webhook
                  </Button>
                }
              >
                <Table columns={webhookColumns} dataSource={webhooks} rowKey="id" loading={loading} />
              </Card>
            ),
          },
          {
            key: 'logs',
            label: <span><HistoryOutlined /> 调用日志</span>,
            children: (
              <Card extra={canDeleteLogs && <Popconfirm title={`删除选中的 ${selectedLogKeys.length} 条调用日志？`} onConfirm={() => handleDeleteLogs(selectedLogKeys.map(Number))} okText="删除" okButtonProps={{ danger: true }}><Button danger icon={<DeleteOutlined />} disabled={selectedLogKeys.length === 0 || deletingLogs}>删除所选{selectedLogKeys.length > 0 ? ` (${selectedLogKeys.length})` : ''}</Button></Popconfirm>}>
                <Text type="secondary">近 24 小时日志用于模块健康统计，暂不可删除。</Text>
                <Table columns={logColumns} dataSource={logs} rowKey="id" rowSelection={canDeleteLogs ? { selectedRowKeys: selectedLogKeys, preserveSelectedRowKeys: true, onChange: (keys) => { if (keys.length > 100) message.warning('每次最多选择 100 条'); setSelectedLogKeys(keys.slice(0, 100)) }, getCheckboxProps: (log) => ({ disabled: logProtected(log) }) } : undefined} pagination={{ current: logPage, pageSize: 20, total: logTotal, onChange: fetchLogs }} />
              </Card>
            ),
          },
        ]}
      />

      <Modal
        title={editingWebhook ? '编辑 Webhook' : '创建 Webhook'}
        open={modalVisible}
        onCancel={() => { setModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="Webhook 名称" />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select options={[
              { label: 'Slack', value: 'slack' },
              { label: 'Teams', value: 'teams' },
              { label: '钉钉', value: 'dingtalk' },
              { label: '自定义', value: 'custom' },
            ]} />
          </Form.Item>
          <Form.Item name="url" label="Webhook URL" rules={[{ required: true }]}>
            <Input placeholder="https://hooks.slack.com/..." />
          </Form.Item>
          <Form.Item name="secret" label="密钥（可选）">
            <Input.Password placeholder="用于验证请求" />
          </Form.Item>
          <Form.Item name="events" label="触发事件">
            <Select mode="multiple" options={[
              { label: '告警', value: 'alert' },
              { label: '事件', value: 'event' },
              { label: '备份', value: 'backup' },
              { label: '任务', value: 'task' },
            ]} />
          </Form.Item>
          <Form.Item name="severity" label="最低告警级别">
            <Select options={[
              { label: '全部', value: '' },
              { label: '信息', value: 'info' },
              { label: '警告', value: 'warning' },
              { label: '错误', value: 'error' },
              { label: '严重', value: 'critical' },
            ]} />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Webhooks
