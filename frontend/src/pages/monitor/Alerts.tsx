import { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  BellOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  AlertHistory,
  AlertRule,
  NotificationChannel,
  createAlertRule,
  createNotificationChannel,
  deleteAlertRule,
  deleteNotificationChannel,
  getAlertHistory,
  getAlertRules,
  getNotificationChannels,
  parseChannelIDs,
  updateAlertRule,
  updateNotificationChannel,
} from '../../api/alert'
import { Cluster, getClusterList } from '../../api/cluster'
import { useQueryTab } from '../../hooks/useQueryTab'

const { Title, Text } = Typography

const ALERT_TABS = ['rules', 'history', 'channels'] as const

const parseChannelConfig = (raw?: string) => {
  if (!raw) return { webhook_url: '', secret: '' }
  try {
    const parsed = JSON.parse(raw)
    return {
      webhook_url: parsed.webhook_url || parsed.url || '',
      secret: parsed.secret || '',
    }
  } catch {
    return { webhook_url: '', secret: '' }
  }
}

const Alerts: React.FC = () => {
  const [activeTab, setActiveTab] = useQueryTab(ALERT_TABS, 'rules')
  const [loading, setLoading] = useState(false)
  const [rules, setRules] = useState<AlertRule[]>([])
  const [rulesTotal, setRulesTotal] = useState(0)
  const [rulesPage, setRulesPage] = useState(1)
  const [history, setHistory] = useState<AlertHistory[]>([])
  const [historyTotal, setHistoryTotal] = useState(0)
  const [historyPage, setHistoryPage] = useState(1)
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])

  const [ruleModalOpen, setRuleModalOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null)
  const [channelModalOpen, setChannelModalOpen] = useState(false)
  const [editingChannel, setEditingChannel] = useState<NotificationChannel | null>(null)

  const [ruleForm] = Form.useForm()
  const [channelForm] = Form.useForm()
  const channelType = Form.useWatch('type', channelForm)

  const fetchClusters = useCallback(async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
    } catch (e) {
      console.error(e)
    }
  }, [])

  const fetchRules = useCallback(async (page = 1) => {
    setLoading(true)
    try {
      const res = await getAlertRules(page, 10)
      setRules(res.data || [])
      setRulesTotal(res.total || 0)
      setRulesPage(page)
    } catch (e) {
      message.error('加载告警规则失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const fetchHistory = useCallback(async (page = 1) => {
    setLoading(true)
    try {
      const res = await getAlertHistory(page, 20)
      setHistory(res.data || [])
      setHistoryTotal(res.total || 0)
      setHistoryPage(page)
    } catch (e) {
      message.error('加载告警历史失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const fetchChannels = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getNotificationChannels()
      setChannels(res.data || [])
    } catch (e) {
      message.error('加载通知渠道失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchClusters()
    fetchChannels()
  }, [fetchClusters, fetchChannels])

  useEffect(() => {
    if (activeTab === 'rules') fetchRules(1)
    else if (activeTab === 'history') fetchHistory(1)
    else fetchChannels()
    // 仅在切换 Tab 时自动加载；分页由表格 onChange 触发
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab])

  const openCreateRule = () => {
    setEditingRule(null)
    ruleForm.resetFields()
    ruleForm.setFieldsValue({
      metric: 'cpu',
      condition: '>',
      threshold: 80,
      resource: 'node',
      enabled: true,
      channels: [],
    })
    setRuleModalOpen(true)
  }

  const openEditRule = (rule: AlertRule) => {
    setEditingRule(rule)
    ruleForm.setFieldsValue({
      name: rule.name,
      cluster_id: rule.cluster_id,
      namespace: rule.namespace,
      resource: rule.resource,
      metric: rule.metric,
      condition: rule.condition,
      threshold: rule.threshold,
      duration: rule.duration,
      channels: parseChannelIDs(rule.channels),
      enabled: rule.enabled,
    })
    setRuleModalOpen(true)
  }

  const submitRule = async () => {
    try {
      const values = await ruleForm.validateFields()
      if (editingRule) {
        await updateAlertRule(editingRule.id, values)
        message.success('规则已更新')
      } else {
        await createAlertRule(values)
        message.success('规则已创建')
      }
      setRuleModalOpen(false)
      fetchRules(rulesPage)
    } catch (e: any) {
      if (e?.errorFields) return
      message.error('保存规则失败')
    }
  }

  const removeRule = async (id: number) => {
    try {
      await deleteAlertRule(id)
      message.success('规则已删除')
      fetchRules(rulesPage)
    } catch {
      message.error('删除失败')
    }
  }

  const toggleRule = async (rule: AlertRule, enabled: boolean) => {
    try {
      await updateAlertRule(rule.id, { enabled })
      fetchRules(rulesPage)
    } catch {
      message.error('更新失败')
    }
  }

  const openCreateChannel = () => {
    setEditingChannel(null)
    channelForm.resetFields()
    channelForm.setFieldsValue({
      type: 'webhook',
      enabled: true,
      webhook_url: '',
      secret: '',
    })
    setChannelModalOpen(true)
  }

  const openEditChannel = (ch: NotificationChannel) => {
    setEditingChannel(ch)
    const cfg = parseChannelConfig(ch.config)
    channelForm.setFieldsValue({
      name: ch.name,
      type: ch.type,
      enabled: ch.enabled,
      webhook_url: cfg.webhook_url,
      secret: cfg.secret,
    })
    setChannelModalOpen(true)
  }

  const submitChannel = async () => {
    try {
      const values = await channelForm.validateFields()
      const config = JSON.stringify({
        webhook_url: values.webhook_url,
        secret: values.secret || '',
      })
      const payload = {
        name: values.name,
        type: values.type,
        config,
        enabled: values.enabled,
      }
      if (editingChannel) {
        await updateNotificationChannel(editingChannel.id, payload)
        message.success('渠道已更新')
      } else {
        await createNotificationChannel(payload)
        message.success('渠道已创建')
      }
      setChannelModalOpen(false)
      fetchChannels()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error('保存渠道失败')
    }
  }

  const removeChannel = async (id: number) => {
    try {
      await deleteNotificationChannel(id)
      message.success('渠道已删除')
      fetchChannels()
    } catch {
      message.error('删除失败')
    }
  }

  const toggleChannel = async (ch: NotificationChannel, enabled: boolean) => {
    try {
      await updateNotificationChannel(ch.id, { enabled })
      fetchChannels()
    } catch {
      message.error('更新失败')
    }
  }

  const ruleColumns: ColumnsType<AlertRule> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '集群',
      key: 'cluster',
      render: (_, r) => r.cluster?.display_name || r.cluster?.name || r.cluster_id,
    },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace', render: (v) => v || '-' },
    { title: '资源', dataIndex: 'resource', key: 'resource' },
    {
      title: '条件',
      key: 'cond',
      render: (_, r) => (
        <Text code>
          {r.metric} {r.condition} {r.threshold}
        </Text>
      ),
    },
    {
      title: '渠道',
      key: 'channels',
      render: (_, r) => {
        const ids = parseChannelIDs(r.channels)
        if (!ids.length) return '-'
        return (
          <Space wrap size={[4, 4]}>
            {ids.map((id) => {
              const ch = channels.find((c) => c.id === id)
              return <Tag key={id}>{ch?.name || `#${id}`}</Tag>
            })}
          </Space>
        )
      },
    },
    {
      title: '持续',
      dataIndex: 'duration',
      key: 'duration',
      width: 80,
      render: (v) => v || '立即',
    },
    {
      title: '评估',
      key: 'eval',
      width: 160,
      render: (_, r) => {
        if (r.last_eval_error) {
          return <Tag color="error" title={r.last_eval_error}>失败</Tag>
        }
        if (r.last_eval_at) {
          return <Tag color="success" title={new Date(r.last_eval_at).toLocaleString()}>正常</Tag>
        }
        return <Tag>未评估</Tag>
      },
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (v, r) => <Switch checked={v} onChange={(checked) => toggleRule(r, checked)} />,
    },
    {
      title: '最近告警',
      dataIndex: 'last_alert',
      key: 'last_alert',
      render: (v) => (v ? new Date(v).toLocaleString() : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, r) => (
        <Space>
          <Button type="link" icon={<EditOutlined />} onClick={() => openEditRule(r)}>
            编辑
          </Button>
          <Popconfirm title="确认删除该规则？" onConfirm={() => removeRule(r.id)}>
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const historyColumns: ColumnsType<AlertHistory> = [
    {
      title: '时间',
      dataIndex: 'triggered_at',
      key: 'triggered_at',
      render: (v) => (v ? new Date(v).toLocaleString() : '-'),
    },
    {
      title: '规则',
      key: 'rule',
      render: (_, h) => h.rule?.name || (h.rule_id ? `#${h.rule_id}` : '外部'),
    },
    {
      title: '集群',
      key: 'cluster',
      render: (_, h) => h.cluster?.display_name || h.cluster?.name || h.cluster_id || '-',
    },
    { title: '资源', dataIndex: 'resource', key: 'resource', render: (v) => v || '-' },
    { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v) => (
        <Badge status={v === 'firing' ? 'error' : 'success'} text={v || '-'} />
      ),
    },
    {
      title: '已通知',
      dataIndex: 'notified',
      key: 'notified',
      render: (v) => (v ? <Tag color="green">是</Tag> : <Tag>否</Tag>),
    },
  ]

  const channelColumns: ColumnsType<NotificationChannel> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (v) => <Tag color="blue">{v}</Tag>,
    },
    {
      title: 'Webhook',
      key: 'url',
      ellipsis: true,
      render: (_, ch) => parseChannelConfig(ch.config).webhook_url || '-',
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (v, ch) => <Switch checked={v} onChange={(checked) => toggleChannel(ch, checked)} />,
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, ch) => (
        <Space>
          <Button type="link" icon={<EditOutlined />} onClick={() => openEditChannel(ch)}>
            编辑
          </Button>
          <Popconfirm title="确认删除该渠道？" onConfirm={() => removeChannel(ch.id)}>
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>
          <BellOutlined style={{ marginRight: 8 }} />
          告警中心
        </Title>
        <Button
          icon={<ReloadOutlined />}
          onClick={() => {
            if (activeTab === 'rules') fetchRules(rulesPage)
            else if (activeTab === 'history') fetchHistory(historyPage)
            else fetchChannels()
          }}
        >
          刷新
        </Button>
      </div>

      <Card>
        <Tabs
          activeKey={activeTab}
          onChange={(k) => setActiveTab(k as typeof ALERT_TABS[number])}
          items={[
            {
              key: 'rules',
              label: '规则',
              children: (
                <>
                  <div style={{ marginBottom: 12 }}>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreateRule}>
                      新建规则
                    </Button>
                  </div>
                  <Table
                    rowKey="id"
                    loading={loading}
                    columns={ruleColumns}
                    dataSource={rules}
                    pagination={{
                      current: rulesPage,
                      total: rulesTotal,
                      pageSize: 10,
                      onChange: (p) => fetchRules(p),
                    }}
                  />
                </>
              ),
            },
            {
              key: 'history',
              label: (
                <span>
                  <HistoryOutlined /> 历史
                </span>
              ),
              children: (
                <Table
                  rowKey="id"
                  loading={loading}
                  columns={historyColumns}
                  dataSource={history}
                  pagination={{
                    current: historyPage,
                    total: historyTotal,
                    pageSize: 20,
                    onChange: (p) => fetchHistory(p),
                  }}
                />
              ),
            },
            {
              key: 'channels',
              label: '通知渠道',
              children: (
                <>
                  <div style={{ marginBottom: 12 }}>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreateChannel}>
                      新建渠道
                    </Button>
                  </div>
                  <Table
                    rowKey="id"
                    loading={loading}
                    columns={channelColumns}
                    dataSource={channels}
                    pagination={false}
                  />
                </>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title={editingRule ? '编辑规则' : '新建规则'}
        open={ruleModalOpen}
        onCancel={() => setRuleModalOpen(false)}
        onOk={submitRule}
        destroyOnClose
        width={640}
      >
        <Form form={ruleForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如：节点 CPU 过高" />
          </Form.Item>
          <Form.Item name="cluster_id" label="集群" rules={[{ required: true, message: '请选择集群' }]}>
            <Select
              options={clusters.map((c) => ({
                value: c.id,
                label: c.display_name || c.name,
              }))}
              placeholder="选择集群"
            />
          </Form.Item>
          <Space style={{ display: 'flex' }} size="middle">
            <Form.Item name="resource" label="资源类型" rules={[{ required: true }]} style={{ flex: 1 }}>
              <Select
                options={[
                  { value: 'node', label: 'Node' },
                  { value: 'pod', label: 'Pod' },
                  { value: 'deployment', label: 'Deployment' },
                ]}
              />
            </Form.Item>
            <Form.Item name="namespace" label="命名空间" style={{ flex: 1 }}>
              <Input placeholder="pod/deployment 必填" />
            </Form.Item>
          </Space>
          <Space style={{ display: 'flex' }} size="middle">
            <Form.Item name="metric" label="指标" rules={[{ required: true }]} style={{ flex: 1 }}>
              <Select
                options={[
                  { value: 'cpu', label: 'CPU' },
                  { value: 'memory', label: 'Memory' },
                ]}
              />
            </Form.Item>
            <Form.Item name="condition" label="条件" rules={[{ required: true }]} style={{ width: 120 }}>
              <Select
                options={['>', '<', '>=', '<=', '=='].map((v) => ({ value: v, label: v }))}
              />
            </Form.Item>
            <Form.Item name="threshold" label="阈值" rules={[{ required: true }]} style={{ flex: 1 }}>
              <InputNumber style={{ width: '100%' }} placeholder="节点用百分比，Pod 用 m/Mi" />
            </Form.Item>
          </Space>
          <Form.Item
            name="duration"
            label="持续时间"
            extra="条件需持续满足该时长后才触发，如 5m / 10m / 1h；留空表示立即触发"
          >
            <Input placeholder="可选，如 5m" />
          </Form.Item>
          <Form.Item name="channels" label="通知渠道">
            <Select
              mode="multiple"
              allowClear
              placeholder="选择通知渠道"
              options={channels.map((c) => ({
                value: c.id,
                label: `${c.name} (${c.type})`,
              }))}
            />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editingChannel ? '编辑渠道' : '新建渠道'}
        open={channelModalOpen}
        onCancel={() => setChannelModalOpen(false)}
        onOk={submitChannel}
        destroyOnClose
      >
        <Form form={channelForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'webhook', label: 'Webhook' },
                { value: 'dingtalk', label: '钉钉' },
                { value: 'feishu', label: '飞书' },
                { value: 'wechat', label: '企业微信' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="webhook_url"
            label="Webhook URL"
            rules={[{ required: true, message: '请输入 webhook_url' }]}
          >
            <Input placeholder="https://..." />
          </Form.Item>
          {(channelType === 'dingtalk' || channelType === 'webhook') && (
            <Form.Item name="secret" label="加签密钥（可选）">
              <Input.Password placeholder="钉钉 secret，可留空" />
            </Form.Item>
          )}
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Text type="secondary">
            将保存为 JSON：{`{"webhook_url":"...","secret":""}`}
          </Text>
        </Form>
      </Modal>
    </div>
  )
}

export default Alerts
