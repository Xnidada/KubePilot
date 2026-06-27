import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, message, Tag, Modal, Form, Input, Select,
  Switch, Popconfirm, Tabs
} from 'antd'
import {
  PlusOutlined, DeleteOutlined, EditOutlined, SendOutlined,
  HistoryOutlined, BellOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { get, post, put, del } from '../../api/request'

const { Title } = Typography

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
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null)
  const [form] = Form.useForm()

  useEffect(() => { fetchWebhooks(); fetchLogs() }, [])

  const fetchWebhooks = async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: Webhook[] }>('/webhooks')
      setWebhooks(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const fetchLogs = async () => {
    try {
      const res = await get<{ code: number; data: WebhookLog[] }>('/webhooks/logs')
      setLogs(res.data || [])
    } catch (e) { console.error(e) }
  }

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
    } catch (e) { message.error('测试失败') }
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
  ]

  return (
    <div>
      <Title level={4}><BellOutlined /> Webhook 通知</Title>

      <Tabs
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
              <Card>
                <Table columns={logColumns} dataSource={logs} rowKey="id" />
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
