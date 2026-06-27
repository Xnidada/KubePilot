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
  Tooltip,
  Descriptions,
  List,
  Alert,
  Segmented,
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  StopOutlined,
  ReloadOutlined,
  EyeOutlined,
  FileTextOutlined,
  FormOutlined,
  CodeOutlined,
  CopyOutlined,
  DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import {
  listTasks,
  createTask,
  cancelTask,
  retryTask,
  deleteTask,
  getTask,
  listQueues,
  Task,
  TaskQueue,
  TaskLog,
} from '../../api/scheduler'

const { Title, Text } = Typography
const { TextArea } = Input

// YAML 示例模板
const YAML_TEMPLATES = {
  job: `# KubePilot 任务调度 - Job 模板
name: my-task
queue_id: 1
cluster_id: 1
task_type: job
priority: 100
image: nginx:latest
command: ["/bin/sh", "-c"]
args: ["echo hello && sleep 10"]
cpu: "100m"
memory: "128Mi"
gpu: 0
replicas: 1
namespace: default
timeout: 3600
max_retry: 3`,
  cronjob: `# KubePilot 任务调度 - CronJob 模板
name: my-cron-task
queue_id: 1
cluster_id: 1
task_type: cronjob
priority: 50
image: busybox:latest
command: ["/bin/sh", "-c"]
args: ["echo scheduled task"]
cpu: "50m"
memory: "64Mi"
replicas: 1
namespace: default
timeout: 1800
max_retry: 2`,
  gpu: `# KubePilot 任务调度 - GPU 训练任务模板
name: training-task
queue_id: 1
cluster_id: 1
task_type: job
priority: 200
image: nvidia/cuda:11.8-base
command: ["/bin/sh", "-c"]
args: ["nvidia-smi && python train.py"]
cpu: "2"
memory: "4Gi"
gpu: 1
gpu_type: "nvidia.com/gpu"
replicas: 1
namespace: default
timeout: 7200
max_retry: 1`,
}

// 简单 YAML 解析器（将 YAML 转换为 JSON）
const parseYAML = (yamlStr: string): any => {
  const result: any = {}
  const lines = yamlStr.split('\n')

  for (const line of lines) {
    // 跳过注释和空行
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue

    // 解析 key: value
    const colonIndex = trimmed.indexOf(':')
    if (colonIndex === -1) continue

    const key = trimmed.substring(0, colonIndex).trim()
    let value: any = trimmed.substring(colonIndex + 1).trim()

    // 处理特殊值
    if (value === 'true') value = true
    else if (value === 'false') value = false
    else if (value === 'null' || value === '') value = null
    else if (/^\d+$/.test(value)) value = parseInt(value)
    else if (/^\d+\.\d+$/.test(value)) value = parseFloat(value)
    // 处理数组格式 [item1, item2]
    else if (value.startsWith('[') && value.endsWith(']')) {
      try {
        value = JSON.parse(value)
      } catch {
        // 保持原样
      }
    }
    // 去除引号
    else if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }

    result[key] = value
  }

  return result
}

const Tasks: React.FC = () => {
  const [tasks, setTasks] = useState<Task[]>([])
  const [queues, setQueues] = useState<TaskQueue[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [queueFilter, setQueueFilter] = useState<number | undefined>()
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedTask, setSelectedTask] = useState<Task | null>(null)
  const [taskLogs, setTaskLogs] = useState<TaskLog[]>([])
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [form] = Form.useForm()

  // YAML 模式状态
  const [createMode, setCreateMode] = useState<'form' | 'yaml'>('form')
  const [yamlContent, setYamlContent] = useState(YAML_TEMPLATES.job)
  const [yamlError, setYamlError] = useState<string>('')
  const [selectedTemplate, setSelectedTemplate] = useState<string>('job')

  useEffect(() => {
    fetchClusters()
    fetchQueues()
  }, [])

  useEffect(() => {
    fetchTasks()
  }, [page, pageSize, statusFilter, queueFilter])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const fetchQueues = async () => {
    try {
      const res = await listQueues()
      setQueues(res.data || [])
    } catch (error) {
      console.error('Failed to fetch queues:', error)
    }
  }

  const fetchTasks = async () => {
    setLoading(true)
    try {
      const res = await listTasks({
        page,
        size: pageSize,
        queue_id: queueFilter,
        status: statusFilter,
      })
      setTasks(res.data || [])
      setTotal(res.total || 0)
    } catch (error) {
      console.error('Failed to fetch tasks:', error)
    } finally {
      setLoading(false)
    }
  }

  // 表单模式提交
  const handleFormSubmit = async (values: any) => {
    try {
      await createTask({
        ...values,
        command: values.command ? values.command.split(' ') : [],
        args: values.args ? values.args.split(' ') : [],
      })
      message.success('任务已提交')
      setCreateModalVisible(false)
      form.resetFields()
      fetchTasks()
    } catch (error) {
      message.error('提交失败')
    }
  }

  // YAML 模式提交
  const handleYamlSubmit = async () => {
    setYamlError('')

    try {
      // 解析 YAML
      const parsed = parseYAML(yamlContent)

      // 验证必填字段
      if (!parsed.name) {
        setYamlError('缺少必填字段: name')
        return
      }
      if (!parsed.queue_id) {
        setYamlError('缺少必填字段: queue_id')
        return
      }
      if (!parsed.cluster_id) {
        setYamlError('缺少必填字段: cluster_id')
        return
      }
      if (!parsed.task_type) {
        setYamlError('缺少必填字段: task_type')
        return
      }
      if (!parsed.image) {
        setYamlError('缺少必填字段: image')
        return
      }

      // 处理命令和参数
      let command = parsed.command
      let args = parsed.args

      if (typeof command === 'string') {
        command = command.split(' ')
      }
      if (typeof args === 'string') {
        args = args.split(' ')
      }

      await createTask({
        name: parsed.name,
        queue_id: parsed.queue_id,
        cluster_id: parsed.cluster_id,
        task_type: parsed.task_type || 'job',
        priority: parsed.priority || 0,
        image: parsed.image,
        command: command || [],
        args: args || [],
        cpu: parsed.cpu || '100m',
        memory: parsed.memory || '128Mi',
        gpu: parsed.gpu || 0,
        gpu_type: parsed.gpu_type || '',
        replicas: parsed.replicas || 1,
        min_replicas: parsed.min_replicas || 1,
        namespace: parsed.namespace || 'default',
        timeout: parsed.timeout || 3600,
        max_retry: parsed.max_retry || 3,
        env_vars: parsed.env_vars,
      })

      message.success('任务已提交')
      setCreateModalVisible(false)
      setYamlContent(YAML_TEMPLATES.job)
      fetchTasks()
    } catch (error: any) {
      setYamlError(error.message || '提交失败')
    }
  }

  // 复制 YAML
  const handleCopyYaml = () => {
    navigator.clipboard.writeText(yamlContent)
    message.success('已复制到剪贴板')
  }

  // 切换模板
  const handleTemplateChange = (template: string) => {
    setSelectedTemplate(template)
    setYamlContent(YAML_TEMPLATES[template as keyof typeof YAML_TEMPLATES] || YAML_TEMPLATES.job)
    setYamlError('')
  }

  const handleCancel = async (id: number) => {
    Modal.confirm({
      title: '确认取消',
      content: '确定要取消此任务吗？',
      onOk: async () => {
        try {
          await cancelTask(id)
          message.success('任务已取消')
          fetchTasks()
        } catch (error) {
          message.error('取消失败')
        }
      },
    })
  }

  const handleRetry = async (id: number) => {
    try {
      await retryTask(id)
      message.success('任务已重新提交')
      fetchTasks()
    } catch (error) {
      message.error('重试失败')
    }
  }

  const handleDelete = async (id: number) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除此任务吗？',
      onOk: async () => {
        try {
          await deleteTask(id)
          message.success('任务已删除')
          fetchTasks()
        } catch (error) {
          message.error('删除失败')
        }
      },
    })
  }

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的任务')
      return
    }
    Modal.confirm({
      title: '批量删除',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个任务吗？`,
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        let success = 0
        for (const key of selectedRowKeys) {
          try {
            await deleteTask(Number(key))
            success++
          } catch (e) { /* ignore */ }
        }
        message.success(`成功删除 ${success} 个任务`)
        setSelectedRowKeys([])
        fetchTasks()
      },
    })
  }

  const handleViewDetail = async (task: Task) => {
    setSelectedTask(task)
    try {
      const res = await getTask(task.id)
      setSelectedTask(res.data.task)
      setTaskLogs(res.data.logs || [])
    } catch (error) {
      console.error('Failed to fetch task detail:', error)
    }
    setDetailModalVisible(true)
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string; text: string }> = {
      pending: { color: 'default', text: '等待中' },
      queued: { color: 'processing', text: '队列中' },
      running: { color: 'processing', text: '运行中' },
      succeeded: { color: 'success', text: '成功' },
      failed: { color: 'error', text: '失败' },
      cancelled: { color: 'warning', text: '已取消' },
    }
    const config = statusMap[status] || { color: 'default', text: status }
    return <Tag color={config.color}>{config.text}</Tag>
  }

  const columns: ColumnsType<Task> = [
    {
      title: '任务名称',
      dataIndex: 'name',
      key: 'name',
      render: (name, record) => (
        <a onClick={() => handleViewDetail(record)}>{name}</a>
      ),
    },
    {
      title: '队列',
      dataIndex: ['queue', 'display_name'],
      key: 'queue',
      render: (_, record) => record.queue?.display_name || record.queue?.name || '-',
    },
    {
      title: '类型',
      dataIndex: 'task_type',
      key: 'task_type',
      render: (type) => <Tag>{type}</Tag>,
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      sorter: (a, b) => a.priority - b.priority,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => getStatusTag(status),
    },
    {
      title: '资源',
      key: 'resources',
      render: (_, record) => (
        <Space>
          <Text type="secondary">{record.cpu}</Text>
          <Text type="secondary">{record.memory}</Text>
          {record.gpu > 0 && <Text type="secondary">{record.gpu} GPU</Text>}
        </Space>
      ),
    },
    {
      title: 'K8S Job',
      dataIndex: 'k8s_job_name',
      key: 'k8s_job_name',
      render: (name) => name ? <Tag color="blue">{name}</Tag> : '-',
    },
    {
      title: '提交时间',
      dataIndex: 'submitted_at',
      key: 'submitted_at',
      render: (time) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button
              type="link"
              icon={<EyeOutlined />}
              onClick={() => handleViewDetail(record)}
            />
          </Tooltip>
          {(record.status === 'pending' || record.status === 'queued' || record.status === 'running') && (
            <Tooltip title="取消">
              <Button
                type="link"
                danger
                icon={<StopOutlined />}
                onClick={() => handleCancel(record.id)}
              />
            </Tooltip>
          )}
          {(record.status === 'failed' || record.status === 'cancelled') && (
            <Tooltip title="重试">
              <Button
                type="link"
                icon={<ReloadOutlined />}
                onClick={() => handleRetry(record.id)}
              />
            </Tooltip>
          )}
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

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>任务管理</Title>
        <Space>
          <Select
            placeholder="队列筛选"
            allowClear
            style={{ width: 150 }}
            value={queueFilter}
            onChange={setQueueFilter}
            options={queues.map(q => ({ label: q.display_name || q.name, value: q.id }))}
          />
          <Select
            placeholder="状态筛选"
            allowClear
            style={{ width: 120 }}
            value={statusFilter}
            onChange={setStatusFilter}
            options={[
              { label: '等待中', value: 'pending' },
              { label: '队列中', value: 'queued' },
              { label: '运行中', value: 'running' },
              { label: '成功', value: 'succeeded' },
              { label: '失败', value: 'failed' },
              { label: '已取消', value: 'cancelled' },
            ]}
          />
          <Button icon={<SyncOutlined />} onClick={fetchTasks}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
            提交任务
          </Button>
        </Space>
      </div>

      {/* 批量操作栏 */}
      {selectedRowKeys.length > 0 && (
        <Card style={{ marginBottom: 16 }}>
          <Space>
            <Text strong>已选择 {selectedRowKeys.length} 项</Text>
            <Button danger icon={<DeleteOutlined />} onClick={handleBatchDelete}>批量删除</Button>
            <Button type="link" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
          </Space>
        </Card>
      )}

      <Card>
        <Table
          columns={columns}
          dataSource={tasks}
          rowKey="id"
          loading={loading}
          rowSelection={{
            selectedRowKeys,
            onChange: (keys) => setSelectedRowKeys(keys),
          }}
          pagination={{
            current: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPage(page)
              setPageSize(pageSize)
            },
          }}
        />
      </Card>

      {/* 创建任务弹窗 */}
      <Modal
        title={
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingRight: 32 }}>
            <span>提交任务</span>
            <Segmented
              value={createMode}
              onChange={(val) => setCreateMode(val as 'form' | 'yaml')}
              options={[
                {
                  label: (
                    <Space>
                      <FormOutlined />
                      基础模式
                    </Space>
                  ),
                  value: 'form',
                },
                {
                  label: (
                    <Space>
                      <CodeOutlined />
                      YAML模式
                    </Space>
                  ),
                  value: 'yaml',
                },
              ]}
            />
          </div>
        }
        open={createModalVisible}
        onCancel={() => {
          setCreateModalVisible(false)
          form.resetFields()
          setYamlError('')
        }}
        onOk={() => createMode === 'form' ? form.submit() : handleYamlSubmit()}
        width={800}
        okText={createMode === 'form' ? '提交' : '解析并提交'}
      >
        {/* 基础模式 */}
        {createMode === 'form' && (
          <Form form={form} layout="vertical" onFinish={handleFormSubmit}>
            <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
              <Input placeholder="输入任务名称" />
            </Form.Item>
            <Form.Item name="queue_id" label="队列" rules={[{ required: true }]}>
              <Select
                placeholder="选择队列"
                options={queues.filter(q => q.status === 'active').map(q => ({
                  label: `${q.display_name || q.name} (${q.policy})`,
                  value: q.id,
                }))}
              />
            </Form.Item>
            <Form.Item name="cluster_id" label="集群" rules={[{ required: true }]}>
              <Select
                placeholder="选择集群"
                options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
              />
            </Form.Item>
            <Form.Item name="task_type" label="任务类型" rules={[{ required: true }]}>
              <Select
                options={[
                  { label: 'Job', value: 'job' },
                  { label: 'CronJob', value: 'cronjob' },
                ]}
              />
            </Form.Item>
            <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
              <Input placeholder="例如: nginx:latest" />
            </Form.Item>
            <Form.Item name="command" label="命令">
              <Input placeholder="例如: /bin/sh -c" />
            </Form.Item>
            <Form.Item name="args" label="参数">
              <TextArea rows={2} placeholder="例如: echo hello" />
            </Form.Item>
            <Space style={{ width: '100%' }}>
              <Form.Item name="cpu" label="CPU" initialValue="100m">
                <Input placeholder="100m" style={{ width: 120 }} />
              </Form.Item>
              <Form.Item name="memory" label="内存" initialValue="128Mi">
                <Input placeholder="128Mi" style={{ width: 120 }} />
              </Form.Item>
              <Form.Item name="gpu" label="GPU" initialValue={0}>
                <InputNumber min={0} style={{ width: 100 }} />
              </Form.Item>
              <Form.Item name="priority" label="优先级" initialValue={0}>
                <InputNumber min={0} max={1000} style={{ width: 100 }} />
              </Form.Item>
            </Space>
            <Form.Item name="namespace" label="命名空间" initialValue="default">
              <Input placeholder="default" />
            </Form.Item>
          </Form>
        )}

        {/* YAML 模式 */}
        {createMode === 'yaml' && (
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12 }}>
              <Space>
                <Text strong>模板:</Text>
                <Select
                  value={selectedTemplate}
                  onChange={handleTemplateChange}
                  style={{ width: 160 }}
                  options={[
                    { label: 'Job 模板', value: 'job' },
                    { label: 'CronJob 模板', value: 'cronjob' },
                    { label: 'GPU 训练模板', value: 'gpu' },
                  ]}
                />
                <Button size="small" icon={<CopyOutlined />} onClick={handleCopyYaml}>
                  复制
                </Button>
              </Space>
            </div>

            <div style={{ position: 'relative' }}>
              <TextArea
                value={yamlContent}
                onChange={(e) => {
                  setYamlContent(e.target.value)
                  setYamlError('')
                }}
                rows={20}
                style={{
                  fontFamily: 'Monaco, Menlo, "Courier New", monospace',
                  fontSize: 13,
                  lineHeight: 1.5,
                  backgroundColor: '#1e1e1e',
                  color: '#d4d4d4',
                  padding: '16px',
                  borderRadius: '8px',
                  border: yamlError ? '1px solid #ff4d4f' : '1px solid #303030',
                }}
                spellCheck={false}
              />
            </div>

            {yamlError && (
              <Alert
                message="YAML 解析错误"
                description={yamlError}
                type="error"
                showIcon
                style={{ marginTop: 12 }}
              />
            )}

            <Alert
              message="YAML 模式说明"
              description={
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  <li>必填字段: name, queue_id, cluster_id, task_type, image</li>
                  <li>command 和 args 支持数组格式: ["/bin/sh", "-c"]</li>
                  <li>支持注释（以 # 开头）</li>
                  <li>可使用模板快速开始</li>
                </ul>
              }
              type="info"
              showIcon
              style={{ marginTop: 12 }}
            />
          </div>
        )}
      </Modal>

      {/* 任务详情弹窗 */}
      <Modal
        title="任务详情"
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={800}
      >
        {selectedTask && (
          <>
            <Descriptions bordered column={2} size="small">
              <Descriptions.Item label="任务ID">{selectedTask.task_id}</Descriptions.Item>
              <Descriptions.Item label="名称">{selectedTask.name}</Descriptions.Item>
              <Descriptions.Item label="状态">{getStatusTag(selectedTask.status)}</Descriptions.Item>
              <Descriptions.Item label="队列">{selectedTask.queue?.display_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="类型">{selectedTask.task_type}</Descriptions.Item>
              <Descriptions.Item label="优先级">{selectedTask.priority}</Descriptions.Item>
              <Descriptions.Item label="镜像">{selectedTask.image}</Descriptions.Item>
              <Descriptions.Item label="命名空间">{selectedTask.namespace}</Descriptions.Item>
              <Descriptions.Item label="CPU">{selectedTask.cpu}</Descriptions.Item>
              <Descriptions.Item label="内存">{selectedTask.memory}</Descriptions.Item>
              <Descriptions.Item label="提交时间">{selectedTask.submitted_at ? new Date(selectedTask.submitted_at).toLocaleString() : '-'}</Descriptions.Item>
              <Descriptions.Item label="开始时间">{selectedTask.started_at ? new Date(selectedTask.started_at).toLocaleString() : '-'}</Descriptions.Item>
              <Descriptions.Item label="完成时间">{selectedTask.completed_at ? new Date(selectedTask.completed_at).toLocaleString() : '-'}</Descriptions.Item>
              <Descriptions.Item label="K8S Job" span={2}>
                {selectedTask.k8s_job_name ? (
                  <Space>
                    <Tag color="blue">{selectedTask.k8s_job_name}</Tag>
                    <Button
                      type="link"
                      size="small"
                      icon={<EyeOutlined />}
                      onClick={() => window.open(`/workloads/jobs?name=${selectedTask.k8s_job_name}`, '_blank')}
                    >
                      查看 K8S Job
                    </Button>
                  </Space>
                ) : '-'}
              </Descriptions.Item>
            </Descriptions>
            {selectedTask.message && (
              <Alert message={selectedTask.message} type="info" style={{ marginTop: 16 }} />
            )}
            <Title level={5} style={{ marginTop: 16 }}>
              <FileTextOutlined /> 任务日志
            </Title>
            <List
              size="small"
              dataSource={taskLogs}
              renderItem={(log) => (
                <List.Item>
                  <Space>
                    <Tag color={log.level === 'error' ? 'error' : log.level === 'warn' ? 'warning' : 'default'}>
                      {log.level}
                    </Tag>
                    <Text>{log.message}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {new Date(log.created_at).toLocaleString()}
                    </Text>
                  </Space>
                </List.Item>
              )}
            />
          </>
        )}
      </Modal>
    </div>
  )
}

export default Tasks
