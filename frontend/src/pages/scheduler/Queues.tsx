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
  InputNumber,
  Switch,
  Tooltip,
  Popconfirm,
  Progress,
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listQueues,
  createQueue,
  updateQueue,
  deleteQueue,
  TaskQueue,
} from '../../api/scheduler'

const { Title, Text } = Typography

const Queues: React.FC = () => {
  const [queues, setQueues] = useState<TaskQueue[]>([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingQueue, setEditingQueue] = useState<TaskQueue | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchQueues()
  }, [])

  const fetchQueues = async () => {
    setLoading(true)
    try {
      const res = await listQueues()
      setQueues(res.data || [])
    } catch (error) {
      console.error('Failed to fetch queues:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setEditingQueue(null)
    form.resetFields()
    form.setFieldsValue({
      priority: 0,
      weight: 1,
      max_tasks: 100,
      policy: 'fifo',
      preemption: false,
    })
    setModalVisible(true)
  }

  const handleEdit = (queue: TaskQueue) => {
    setEditingQueue(queue)
    form.setFieldsValue(queue)
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingQueue) {
        await updateQueue(editingQueue.id, values)
        message.success('队列已更新')
      } else {
        await createQueue(values)
        message.success('队列已创建')
      }
      setModalVisible(false)
      form.resetFields()
      fetchQueues()
    } catch (error) {
      message.error('操作失败')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteQueue(id)
      message.success('队列已删除')
      fetchQueues()
    } catch (error) {
      message.error('删除失败')
    }
  }

  const columns: ColumnsType<TaskQueue> = [
    {
      title: '队列名称',
      key: 'name',
      render: (_, record) => (
        <div>
          <Text strong>{record.display_name || record.name}</Text>
          {record.description && (
            <div>
              <Text type="secondary" style={{ fontSize: 12 }}>{record.description}</Text>
            </div>
          )}
        </div>
      ),
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      sorter: (a, b) => a.priority - b.priority,
    },
    {
      title: '权重',
      dataIndex: 'weight',
      key: 'weight',
    },
    {
      title: '调度策略',
      dataIndex: 'policy',
      key: 'policy',
      render: (policy) => {
        const policyMap: Record<string, string> = {
          fifo: '先进先出',
          priority: '优先级',
          fair: '公平调度',
        }
        return <Tag>{policyMap[policy] || policy}</Tag>
      },
    },
    {
      title: '资源配额',
      key: 'quota',
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          {record.max_cpu && <Text type="secondary">CPU: {record.max_cpu}</Text>}
          {record.max_memory && <Text type="secondary">内存: {record.max_memory}</Text>}
          {record.max_gpu > 0 && <Text type="secondary">GPU: {record.max_gpu}</Text>}
        </Space>
      ),
    },
    {
      title: '任务数',
      key: 'tasks',
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          <Text>运行: {record.running_tasks}</Text>
          <Text type="secondary">等待: {record.pending_tasks}</Text>
          <Text type="secondary">上限: {record.max_tasks}</Text>
        </Space>
      ),
    },
    {
      title: '使用率',
      key: 'usage',
      render: (_, record) => {
        const usage = record.max_tasks > 0 ? Math.round((record.running_tasks / record.max_tasks) * 100) : 0
        return (
          <Progress
            percent={usage}
            size="small"
            status={usage >= 90 ? 'exception' : usage >= 70 ? 'active' : 'normal'}
          />
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={status === 'active' ? 'success' : status === 'paused' ? 'warning' : 'default'}>
          {status === 'active' ? '活跃' : status === 'paused' ? '暂停' : status}
        </Tag>
      ),
    },
    {
      title: '抢占',
      dataIndex: 'preemption',
      key: 'preemption',
      render: (preemption) => preemption ? <Tag color="orange">是</Tag> : <Tag>否</Tag>,
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
          <Popconfirm
            title="确定删除此队列？"
            onConfirm={() => handleDelete(record.id)}
          >
            <Tooltip title="删除">
              <Button type="link" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>队列管理</Title>
        <Space>
          <Button icon={<SyncOutlined />} onClick={fetchQueues}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            创建队列
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={queues} rowKey="id" loading={loading} />
      </Card>

      <Modal
        title={editingQueue ? '编辑队列' : '创建队列'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="name" label="队列标识" rules={[{ required: true }]}>
            <Input placeholder="输入队列标识" disabled={!!editingQueue} />
          </Form.Item>
          <Form.Item name="display_name" label="显示名称">
            <Input placeholder="输入显示名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="输入队列描述" />
          </Form.Item>
          <Space>
            <Form.Item name="priority" label="优先级">
              <InputNumber min={0} max={1000} style={{ width: 100 }} />
            </Form.Item>
            <Form.Item name="weight" label="权重">
              <InputNumber min={1} max={100} style={{ width: 100 }} />
            </Form.Item>
            <Form.Item name="max_tasks" label="最大任务数">
              <InputNumber min={1} style={{ width: 100 }} />
            </Form.Item>
          </Space>
          <Space>
            <Form.Item name="max_cpu" label="最大 CPU">
              <Input placeholder="例如: 8" style={{ width: 120 }} />
            </Form.Item>
            <Form.Item name="max_memory" label="最大内存">
              <Input placeholder="例如: 16Gi" style={{ width: 120 }} />
            </Form.Item>
            <Form.Item name="max_gpu" label="最大 GPU">
              <InputNumber min={0} style={{ width: 100 }} />
            </Form.Item>
          </Space>
          <Form.Item name="policy" label="调度策略">
            <Select
              options={[
                { label: '先进先出 (FIFO)', value: 'fifo' },
                { label: '优先级调度', value: 'priority' },
                { label: '公平调度', value: 'fair' },
              ]}
            />
          </Form.Item>
          <Form.Item name="preemption" label="允许抢占" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Queues
