import { useState, useEffect } from 'react'
import {
  Card,
  Table,
  Button,
  Space,
  Typography,
  message,
  Tag,
  Modal,
  Form,
  Input,
  Select,
  Switch,
  Tabs,
  Tooltip,
} from 'antd'
import {
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  ReloadOutlined,
  SendOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LinkOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import {
  listEventForwardRules,
  createEventForwardRule,
  updateEventForwardRule,
  deleteEventForwardRule,
  testEventForwardRule,
  listEventForwardLogs,
  EventForwardRule,
  EventForwardLog,
} from '../../api/system'

const { Title } = Typography

const EventForward: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [rules, setRules] = useState<EventForwardRule[]>([])
  const [logs, setLogs] = useState<EventForwardLog[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [showRuleModal, setShowRuleModal] = useState(false)
  const [editingRule, setEditingRule] = useState<EventForwardRule | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchRules()
      fetchLogs()
    }
  }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data && res.data.length > 0) {
        setSelectedCluster(res.data[0].id)
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const fetchRules = async () => {
    setLoading(true)
    try {
      const res = await listEventForwardRules(selectedCluster)
      setRules(res.data || [])
    } catch (error) {
      console.error('Failed to fetch rules:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchLogs = async () => {
    try {
      const res = await listEventForwardLogs()
      setLogs(res.data || [])
    } catch (error) {
      console.error('Failed to fetch logs:', error)
    }
  }

  const handleCreate = () => {
    setEditingRule(null)
    form.resetFields()
    form.setFieldsValue({
      cluster_id: selectedCluster,
      event_types: ['Normal', 'Warning'],
      enabled: true,
    })
    setShowRuleModal(true)
  }

  const handleEdit = (rule: EventForwardRule) => {
    setEditingRule(rule)
    form.setFieldsValue({
      ...rule,
      namespaces: rule.namespaces ? JSON.parse(rule.namespaces) : [],
      resources: rule.resources ? JSON.parse(rule.resources) : [],
      event_types: rule.event_types ? JSON.parse(rule.event_types) : [],
      reasons: rule.reasons ? JSON.parse(rule.reasons) : [],
    })
    setShowRuleModal(true)
  }

  const handleSave = async () => {
    try {
      const values = await form.validateFields()
      const data = {
        ...values,
        namespaces: JSON.stringify(values.namespaces || []),
        resources: JSON.stringify(values.resources || []),
        event_types: JSON.stringify(values.event_types || []),
        reasons: JSON.stringify(values.reasons || []),
      }
      if (editingRule) {
        await updateEventForwardRule(editingRule.id, data)
        message.success('规则已更新')
      } else {
        await createEventForwardRule(data)
        message.success('规则已创建')
      }
      setShowRuleModal(false)
      fetchRules()
    } catch (error) {
      message.error('保存失败')
    }
  }

  const handleDelete = async (id: number) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除此转发规则吗？',
      onOk: async () => {
        await deleteEventForwardRule(id)
        message.success('规则已删除')
        fetchRules()
      },
    })
  }

  const handleTest = async (id: number) => {
    try {
      await testEventForwardRule(id)
      message.success('测试消息已发送')
      fetchLogs()
    } catch (error) {
      message.error('测试失败')
    }
  }

  const ruleColumns = [
    { title: '规则名称', dataIndex: 'name', key: 'name' },
    {
      title: 'Webhook URL',
      dataIndex: 'webhook_url',
      key: 'webhook_url',
      ellipsis: true,
      render: (url: string) => (
        <Tooltip title={url}>
          <Space>
            <LinkOutlined />
            {url}
          </Space>
        </Tooltip>
      ),
    },
    {
      title: '事件类型',
      dataIndex: 'event_types',
      key: 'event_types',
      render: (v: string) => {
        if (!v) return '全部'
        try {
          const types = JSON.parse(v)
          return types.map((t: string) => (
            <Tag key={t} color={t === 'Warning' ? 'warning' : 'default'}>{t}</Tag>
          ))
        } catch {
          return v
        }
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: EventForwardRule) => (
        <Space>
          <Tooltip title="测试">
            <Button
              type="link"
              icon={<SendOutlined />}
              onClick={() => handleTest(record.id)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Tooltip title="删除">
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
              onClick={() => handleDelete(record.id)}
            />
          </Tooltip>
        </Space>
      ),
    },
  ]

  const logColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag
          icon={status === 'success' ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
          color={status === 'success' ? 'success' : 'error'}
        >
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: '资源', dataIndex: 'resource', key: 'resource' },
    { title: '事件类型', dataIndex: 'event_type', key: 'event_type' },
    { title: '原因', dataIndex: 'reason', key: 'reason' },
    { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (t: string) => new Date(t).toLocaleString(),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>Event 转发</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Button icon={<ReloadOutlined />} onClick={() => { fetchRules(); fetchLogs() }}>
            刷新
          </Button>
        </Space>
      </div>

      <Tabs
        items={[
          {
            key: 'rules',
            label: '转发规则',
            children: (
              <Card
                title="转发规则"
                extra={
                  <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
                    新建规则
                  </Button>
                }
              >
                <Table
                  columns={ruleColumns}
                  dataSource={rules}
                  rowKey="id"
                  loading={loading}
                />
              </Card>
            ),
          },
          {
            key: 'logs',
            label: '转发日志',
            children: (
              <Card title="转发日志">
                <Table
                  columns={logColumns}
                  dataSource={logs}
                  rowKey="id"
                  size="small"
                />
              </Card>
            ),
          },
        ]}
      />

      {/* 规则编辑弹窗 */}
      <Modal
        title={editingRule ? '编辑规则' : '新建规则'}
        open={showRuleModal}
        onOk={handleSave}
        onCancel={() => setShowRuleModal(false)}
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="cluster_id" hidden>
            <Input />
          </Form.Item>
          <Form.Item name="name" label="规则名称" rules={[{ required: true }]}>
            <Input placeholder="输入规则名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="输入规则描述" />
          </Form.Item>
          <Form.Item name="webhook_url" label="Webhook URL" rules={[{ required: true }]}>
            <Input placeholder="https://example.com/webhook" />
          </Form.Item>
          <Form.Item name="event_types" label="事件类型">
            <Select
              mode="multiple"
              placeholder="选择事件类型（留空表示全部）"
              options={[
                { label: 'Normal', value: 'Normal' },
                { label: 'Warning', value: 'Warning' },
              ]}
            />
          </Form.Item>
          <Form.Item name="resources" label="资源类型">
            <Select
              mode="multiple"
              placeholder="选择资源类型（留空表示全部）"
              options={[
                { label: 'Pod', value: 'Pod' },
                { label: 'Deployment', value: 'Deployment' },
                { label: 'Service', value: 'Service' },
                { label: 'Node', value: 'Node' },
                { label: 'ConfigMap', value: 'ConfigMap' },
                { label: 'Secret', value: 'Secret' },
              ]}
            />
          </Form.Item>
          <Form.Item name="namespaces" label="命名空间">
            <Select
              mode="multiple"
              placeholder="选择命名空间（留空表示全部）"
              options={[
                { label: 'default', value: 'default' },
                { label: 'kube-system', value: 'kube-system' },
                { label: 'kube-public', value: 'kube-public' },
              ]}
            />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default EventForward
