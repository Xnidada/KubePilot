import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, message, Popconfirm,
  Modal, Form, InputNumber, Divider, Row, Col, Switch, Radio, Tooltip
} from 'antd'

const { TextArea } = Input
import {
  SyncOutlined, DeleteOutlined, SearchOutlined, PlusOutlined, EditOutlined, CodeOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { get, post, put, del } from '../../api/request'

const { Title, Text } = Typography

interface CronJob {
  name: string
  namespace: string
  schedule: string
  suspend: boolean
  active: number
  last_schedule: string
  age: string
  images: string[]
}

// 根据表单值生成 cron 表达式
const buildCronExpression = (values: any): string => {
  const { scheduleType, minute, hour, dayOfMonth, month, dayOfWeek,
    everyNMinutes, everyNHours, specificDays, specificTime } = values

  switch (scheduleType) {
    case 'every_minute':
      return `*/${everyNMinutes || 1} * * * *`
    case 'every_hour':
      return `0 */${everyNHours || 1} * * *`
    case 'daily':
      return `${specificTime?.minute || 0} ${specificTime?.hour || 0} * * *`
    case 'weekly':
      return `${specificTime?.minute || 0} ${specificTime?.hour || 0} * * ${specificDays || 0}`
    case 'monthly':
      return `${specificTime?.minute || 0} ${specificTime?.hour || 0} ${dayOfMonth || 1} * *`
    case 'custom':
      return `${minute || '*'} ${hour || '*'} ${dayOfMonth || '*'} ${month || '*'} ${dayOfWeek || '*'}`
    default:
      return '0 * * * *'
  }
}

// 解析 cron 表达式为中文描述
const parseCronToText = (cron: string): string => {
  if (!cron) return ''
  const parts = cron.split(' ')
  if (parts.length !== 5) return cron

  const [min, hour, dom, month, dow] = parts
  const descriptions: string[] = []

  if (min === '*') descriptions.push('每分钟')
  else if (min.startsWith('*/')) descriptions.push(`每 ${min.slice(2)} 分钟`)
  else descriptions.push(`第 ${min} 分钟`)

  if (hour === '*') descriptions.push('每小时')
  else if (hour.startsWith('*/')) descriptions.push(`每 ${hour.slice(2)} 小时`)
  else descriptions.push(`${hour} 时`)

  if (dom !== '*') descriptions.push(`${dom} 日`)
  if (month !== '*') descriptions.push(`${month} 月`)

  if (dow !== '*') {
    const weekDays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
    const dayIndex = parseInt(dow)
    if (!isNaN(dayIndex) && dayIndex >= 0 && dayIndex <= 6) {
      descriptions.push(weekDays[dayIndex])
    } else {
      descriptions.push(dow)
    }
  }

  return descriptions.join('，')
}

const CronJobManagement: React.FC = () => {
  const [cronJobs, setCronJobs] = useState<CronJob[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [yamlModalVisible, setYamlModalVisible] = useState(false)
  const [editingJob, setEditingJob] = useState<CronJob | null>(null)
  const [yamlContent, setYamlContent] = useState('')
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()
  const scheduleType = Form.useWatch('scheduleType', form)
  const editScheduleType = Form.useWatch('scheduleType', editForm)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchData() } }, [selectedCluster, selectedNamespace])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(selectedCluster)
      setNamespaces(res.data || [])
    } catch (e) { console.error(e) }
  }

  const fetchData = async () => {
    setLoading(true)
    try {
      const params = selectedNamespace ? `?ns=${selectedNamespace}` : ''
      const res = await get<{ code: number; data: CronJob[] }>(`/clusters/${selectedCluster}/workloads/cronjobs${params}`)
      setCronJobs(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  // 解析命令参数
  const parseCommandArgs = (cmd: string, args: string) => {
    let command: string[] = []
    let parsedArgs: string[] = []

    // 解析 command
    if (cmd) {
      try {
        if (cmd.trim().startsWith('[')) {
          command = JSON.parse(cmd)
        } else {
          command = cmd.trim().split(/\s+/)
        }
      } catch {
        command = cmd.trim().split(/\s+/)
      }
    }

    // 解析 args - 支持 JSON 数组或带引号的字符串
    if (args) {
      try {
        if (args.trim().startsWith('[')) {
          parsedArgs = JSON.parse(args)
        } else {
          // 处理引号内的内容作为单个参数
          parsedArgs = parseQuotedArgs(args.trim())
        }
      } catch {
        parsedArgs = parseQuotedArgs(args.trim())
      }
    }

    return { command, args: parsedArgs }
  }

  // 解析带引号的参数
  const parseQuotedArgs = (input: string): string[] => {
    const result: string[] = []
    let current = ''
    let inQuote = false
    let quoteChar = ''

    for (let i = 0; i < input.length; i++) {
      const char = input[i]
      if (inQuote) {
        if (char === quoteChar) {
          inQuote = false
        } else {
          current += char
        }
      } else {
        if (char === '"' || char === "'") {
          inQuote = true
          quoteChar = char
        } else if (char === ' ' || char === '\t') {
          if (current) {
            result.push(current)
            current = ''
          }
        } else {
          current += char
        }
      }
    }
    if (current) {
      result.push(current)
    }
    return result
  }

  const handleCreate = async (values: any) => {
    const schedule = buildCronExpression(values)
    const { command, args } = parseCommandArgs(values.command || '', values.args || '')

    try {
      await post(`/clusters/${selectedCluster}/workloads/cronjobs`, {
        namespace: values.namespace,
        name: values.name,
        schedule: schedule,
        image: values.image,
        command: command.length > 0 ? command : undefined,
        args: args.length > 0 ? args : undefined,
        suspend: values.suspend || false,
      })
      message.success('CronJob 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchData()
    } catch (e) { message.error('创建失败') }
  }

  const handleEdit = async (record: CronJob) => {
    setEditingJob(record)
    editForm.setFieldsValue({
      namespace: record.namespace,
      name: record.name,
      schedule: record.schedule,
      image: record.images?.[0] || '',
      suspend: record.suspend,
      scheduleType: 'custom',
      customSchedule: record.schedule,
    })
    setEditModalVisible(true)
  }

  const handleEditYAML = async (record: CronJob) => {
    setEditingJob(record)
    try {
      const res = await get<{ code: number; data: { yaml: string } }>(
        `/clusters/${selectedCluster}/workloads/yaml/cronjobs/${record.namespace}/${record.name}`
      )
      if (res.data?.yaml) {
        setYamlContent(res.data.yaml)
        setYamlModalVisible(true)
      }
    } catch (e) { message.error('获取 YAML 失败') }
  }

  const handleUpdate = async (values: any) => {
    if (!editingJob) return
    const schedule = buildCronExpression(values)
    try {
      await put(`/clusters/${selectedCluster}/workloads/cronjobs/${editingJob.namespace}/${editingJob.name}`, {
        schedule: schedule,
        image: values.image,
        suspend: values.suspend || false,
      })
      message.success('CronJob 更新成功')
      setEditModalVisible(false)
      editForm.resetFields()
      fetchData()
    } catch (e) { message.error('更新失败') }
  }

  const handleApplyYAML = async () => {
    try {
      await post('/aiops/kubectl', {
        cluster_id: selectedCluster,
        command: 'apply',
        yaml: yamlContent,
      })
      message.success('YAML 应用成功')
      setYamlModalVisible(false)
      fetchData()
    } catch (e) { message.error('应用失败') }
  }

  const handleDelete = async (record: CronJob) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/cronjobs/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchData()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<CronJob> = [
    { title: '名称', dataIndex: 'name', key: 'name', filteredValue: searchText ? [searchText] : null, onFilter: (v, r) => r.name.includes(v as string) },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: '调度规则', dataIndex: 'schedule', key: 'schedule',
      render: (s) => (
        <div>
          <Tag>{s}</Tag>
          <br />
          <Text type="secondary" style={{ fontSize: 12 }}>{parseCronToText(s)}</Text>
        </div>
      )
    },
    { title: '状态', dataIndex: 'suspend', key: 'suspend', render: (v) => v ? <Tag color="warning">暂停</Tag> : <Tag color="success">运行中</Tag> },
    { title: 'Active', dataIndex: 'active', key: 'active' },
    { title: '最后调度', dataIndex: 'last_schedule', key: 'last_schedule', render: (v) => v || '-' },
    { title: '镜像', dataIndex: 'images', key: 'images', render: (imgs: string[]) => imgs?.map(i => <Tag key={i}>{i}</Tag>) },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 180,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="图形编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
          <Tooltip title="YAML 编辑">
            <Button type="link" icon={<CodeOutlined />} onClick={() => handleEditYAML(record)} />
          </Tooltip>
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // 渲染调度规则表单
  const renderScheduleForm = (_formInstance: any, prefix: string = '') => {
    const type = prefix ? editScheduleType : scheduleType
    return (
      <>
        <Form.Item name="scheduleType" label="调度频率" initialValue="daily">
          <Radio.Group>
            <Radio.Button value="every_minute">每N分钟</Radio.Button>
            <Radio.Button value="every_hour">每N小时</Radio.Button>
            <Radio.Button value="daily">每天</Radio.Button>
            <Radio.Button value="weekly">每周</Radio.Button>
            <Radio.Button value="monthly">每月</Radio.Button>
            <Radio.Button value="custom">自定义</Radio.Button>
          </Radio.Group>
        </Form.Item>

        {type === 'every_minute' && (
          <Form.Item name="everyNMinutes" label="间隔分钟数" initialValue={5}>
            <InputNumber min={1} max={59} style={{ width: 200 }} addonAfter="分钟" />
          </Form.Item>
        )}

        {type === 'every_hour' && (
          <Form.Item name="everyNHours" label="间隔小时数" initialValue={1}>
            <InputNumber min={1} max={23} style={{ width: 200 }} addonAfter="小时" />
          </Form.Item>
        )}

        {type === 'daily' && (
          <Space>
            <Form.Item name={['specificTime', 'hour']} label="小时" initialValue={0}>
              <InputNumber min={0} max={23} style={{ width: 100 }} addonAfter="时" />
            </Form.Item>
            <Form.Item name={['specificTime', 'minute']} label="分钟" initialValue={0}>
              <InputNumber min={0} max={59} style={{ width: 100 }} addonAfter="分" />
            </Form.Item>
          </Space>
        )}

        {type === 'weekly' && (
          <>
            <Form.Item name="specificDays" label="星期几" initialValue="1">
              <Select style={{ width: 200 }} options={[
                { label: '周日', value: '0' },
                { label: '周一', value: '1' },
                { label: '周二', value: '2' },
                { label: '周三', value: '3' },
                { label: '周四', value: '4' },
                { label: '周五', value: '5' },
                { label: '周六', value: '6' },
              ]} />
            </Form.Item>
            <Space>
              <Form.Item name={['specificTime', 'hour']} label="小时" initialValue={0}>
                <InputNumber min={0} max={23} style={{ width: 100 }} addonAfter="时" />
              </Form.Item>
              <Form.Item name={['specificTime', 'minute']} label="分钟" initialValue={0}>
                <InputNumber min={0} max={59} style={{ width: 100 }} addonAfter="分" />
              </Form.Item>
            </Space>
          </>
        )}

        {type === 'monthly' && (
          <>
            <Form.Item name="dayOfMonth" label="几号" initialValue={1}>
              <InputNumber min={1} max={31} style={{ width: 200 }} addonAfter="号" />
            </Form.Item>
            <Space>
              <Form.Item name={['specificTime', 'hour']} label="小时" initialValue={0}>
                <InputNumber min={0} max={23} style={{ width: 100 }} addonAfter="时" />
              </Form.Item>
              <Form.Item name={['specificTime', 'minute']} label="分钟" initialValue={0}>
                <InputNumber min={0} max={59} style={{ width: 100 }} addonAfter="分" />
              </Form.Item>
            </Space>
          </>
        )}

        {type === 'custom' && (
          <Form.Item name={prefix ? 'customSchedule' : 'schedule'} label="Cron 表达式" rules={[{ required: true }]}>
            <Input placeholder="0 2 * * *" addonAfter={<Text type="secondary">分 时 日 月 周</Text>} />
          </Form.Item>
        )}
      </>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>CronJob</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchData}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); form.setFieldsValue({ scheduleType: 'daily', specificTime: { hour: 0, minute: 0 } }); setCreateModalVisible(true) }}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={cronJobs} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal title="创建 CronJob" open={createModalVisible} onCancel={() => { setCreateModalVisible(false); form.resetFields() }} onOk={() => form.submit()} width={700}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
                <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true }]}>
                <Input placeholder="例如: daily-backup" />
              </Form.Item>
            </Col>
          </Row>

          <Divider>调度规则</Divider>
          {renderScheduleForm(form)}

          <Divider>容器配置</Divider>
          <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
            <Input placeholder="例如: busybox:latest" />
          </Form.Item>
          <Form.Item name="command" label="入口命令 (Command)" help="例如: /bin/sh 或 /bin/bash">
            <Input placeholder="/bin/sh" />
          </Form.Item>
          <Form.Item name="args" label="参数 (Args)" help="支持 JSON 数组格式: [&quot;-c&quot;, &quot;echo hello&quot;] 或空格分隔: -c echo hello">
            <TextArea rows={3} placeholder={'例如: -c "for i in 9 8 7 6 5 4 3 2 1; do echo $i; sleep 3; done"'} />
          </Form.Item>

          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="suspend" label="暂停" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="successfulHistory" label="成功历史数" initialValue={3}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="failedHistory" label="失败历史数" initialValue={1}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>

          <Divider>资源配额</Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item name="cpuRequest" label="CPU 请求" initialValue="100m">
                <Input placeholder="100m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="cpuLimit" label="CPU 限制" initialValue="200m">
                <Input placeholder="200m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memoryRequest" label="内存请求" initialValue="128Mi">
                <Input placeholder="128Mi" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memoryLimit" label="内存限制" initialValue="256Mi">
                <Input placeholder="256Mi" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* 编辑 Modal (图形化) */}
      <Modal title="编辑 CronJob" open={editModalVisible} onCancel={() => { setEditModalVisible(false); editForm.resetFields() }} onOk={() => editForm.submit()} width={700}>
        <Form form={editForm} layout="vertical" onFinish={handleUpdate}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="namespace" label="命名空间">
                <Input disabled />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称">
                <Input disabled />
              </Form.Item>
            </Col>
          </Row>

          <Divider>调度规则</Divider>
          {renderScheduleForm(editForm, 'edit')}

          <Divider>容器配置</Divider>
          <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
            <Input placeholder="例如: busybox:latest" />
          </Form.Item>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="suspend" label="暂停" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* YAML 编辑 Modal */}
      <Modal
        title="YAML 编辑"
        open={yamlModalVisible}
        onCancel={() => setYamlModalVisible(false)}
        onOk={handleApplyYAML}
        width={800}
      >
        <TextArea
          value={yamlContent}
          onChange={(e) => setYamlContent(e.target.value)}
          rows={20}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
        />
      </Modal>
    </div>
  )
}

export default CronJobManagement
