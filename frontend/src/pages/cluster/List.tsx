import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Table,
  Card,
  Button,
  Space,
  Tag,
  Modal,
  Form,
  Input,
  message,
  Popconfirm,
  Typography,
  Tooltip,
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, createCluster, updateCluster, deleteCluster, healthCheckCluster, Cluster } from '../../api/cluster'

const { Title } = Typography
const { TextArea } = Input

const ClusterList: React.FC = () => {
  const navigate = useNavigate()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingCluster, setEditingCluster] = useState<Cluster | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [page, pageSize])

  const fetchClusters = async () => {
    setLoading(true)
    try {
      const res = await getClusterList(page, pageSize)
      setClusters(res.data || [])
      setTotal(res.total || 0)
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setEditingCluster(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEdit = (record: Cluster) => {
    setEditingCluster(record)
    form.setFieldsValue({
      name: record.name,
      display_name: record.display_name,
      description: record.description,
      api_server: record.api_server,
      tags: record.tags,
    })
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingCluster) {
        await updateCluster(editingCluster.id, {
          display_name: values.display_name,
          description: values.description,
          api_server: values.api_server,
          kubeconfig: values.kubeconfig,
          tags: values.tags,
        })
        message.success('集群更新成功')
      } else {
        await createCluster(values)
        message.success('集群添加成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchClusters()
    } catch (error) {
      console.error('Failed to save cluster:', error)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteCluster(id)
      message.success('集群已删除')
      fetchClusters()
    } catch (error) {
      console.error('Failed to delete cluster:', error)
    }
  }

  const handleHealthCheck = async (id: number) => {
    try {
      await healthCheckCluster(id)
      message.success('健康检查完成')
      fetchClusters()
    } catch (error) {
      console.error('Health check failed:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string; icon: React.ReactNode; text: string }> = {
      connected: { color: 'success', icon: <CheckCircleOutlined />, text: '已连接' },
      error: { color: 'error', icon: <CloseCircleOutlined />, text: '错误' },
      unknown: { color: 'default', icon: <WarningOutlined />, text: '未知' },
      disconnected: { color: 'warning', icon: <WarningOutlined />, text: '已断开' },
    }
    const config = statusMap[status] || statusMap.unknown
    return (
      <Tag color={config.color} icon={config.icon}>
        {config.text}
      </Tag>
    )
  }

  const columns: ColumnsType<Cluster> = [
    {
      title: '集群名称',
      key: 'name',
      render: (_, record) => (
        <a onClick={() => navigate(`/clusters/${record.id}`)}>
          {record.display_name || record.name}
        </a>
      ),
    },
    {
      title: 'API Server',
      dataIndex: 'api_server',
      key: 'api_server',
      ellipsis: true,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => getStatusTag(status),
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      render: (version) => version || '-',
    },
    {
      title: '节点数',
      dataIndex: 'node_count',
      key: 'node_count',
    },
    {
      title: 'CPU',
      dataIndex: 'cpu_capacity',
      key: 'cpu_capacity',
      render: (text) => text || '-',
    },
    {
      title: '内存',
      dataIndex: 'memory_capacity',
      key: 'memory_capacity',
      render: (text) => text || '-',
    },
    {
      title: '最后检查',
      dataIndex: 'last_health_check',
      key: 'last_health_check',
      render: (time) => time || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button
              type="link"
              icon={<EyeOutlined />}
              onClick={() => navigate(`/clusters/${record.id}`)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Tooltip title="健康检查">
            <Button
              type="link"
              icon={<SyncOutlined />}
              onClick={() => handleHealthCheck(record.id)}
            />
          </Tooltip>
          <Popconfirm
            title="确定要删除这个集群吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
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
        <Title level={4}>集群管理</Title>
        <Space>
          <Button icon={<SyncOutlined />} onClick={fetchClusters}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            添加集群
          </Button>
        </Space>
      </div>

      <Card>
        <Table
          columns={columns}
          dataSource={clusters}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPage(page)
              setPageSize(pageSize)
            },
          }}
        />
      </Card>

      <Modal
        title={editingCluster ? '编辑集群' : '添加集群'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="name"
            label="集群名称"
            rules={[{ required: !editingCluster, message: '请输入集群名称' }]}
          >
            <Input placeholder="请输入集群名称" disabled={!!editingCluster} />
          </Form.Item>
          <Form.Item name="display_name" label="显示名称">
            <Input placeholder="请输入显示名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input placeholder="请输入集群描述" />
          </Form.Item>
          <Form.Item
            name="api_server"
            label="API Server"
            rules={[{ required: true, message: '请输入 API Server 地址' }]}
          >
            <Input placeholder="https://kubernetes.default.svc" />
          </Form.Item>
          <Form.Item
            name="kubeconfig"
            label="Kubeconfig"
            rules={[{ required: !editingCluster, message: '请粘贴 kubeconfig 内容' }]}
          >
            <TextArea rows={8} placeholder={editingCluster ? '留空则保持原有 kubeconfig 不变' : '请粘贴 kubeconfig 文件内容'} />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Input placeholder="多个标签用逗号分隔" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ClusterList
