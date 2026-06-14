import { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Typography,
  Select,
  Tooltip,
  Modal,
  Form,
  Input,
  message,
  Popconfirm,
  Row,
  Col,
  Statistic,
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EyeOutlined,
  FolderOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getNamespaceDetail, createNamespace, deleteNamespace, NamespaceDetail } from '../../api/resources'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaces, Namespace } from '../../api/workload'
import StatusTag from '../../components/StatusTag'
import { usePolling, hasTerminatingResource } from '../../hooks/usePolling'

const { Title, Text } = Typography

const NamespaceManagement: React.FC = () => {
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedNS, setSelectedNS] = useState<NamespaceDetail | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
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

  const fetchNamespaces = async () => {
    setLoading(true)
    try {
      const res = await getNamespaces(selectedCluster)
      setNamespaces(res.data || [])
    } catch (error) {
      console.error('Failed to fetch namespaces:', error)
    } finally {
      setLoading(false)
    }
  }

  // 自动轮询：当有 Terminating 状态的资源时自动刷新
  usePolling(fetchNamespaces, hasTerminatingResource(namespaces), { interval: 3000 })

  const handleCreate = async (values: any) => {
    try {
      const labels: Record<string, string> = {}
      if (values.labels) {
        values.labels.split(',').forEach((pair: string) => {
          const [key, value] = pair.split('=')
          if (key && value) {
            labels[key.trim()] = value.trim()
          }
        })
      }
      await createNamespace(selectedCluster, { name: values.name, labels })
      message.success('命名空间创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchNamespaces()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteNamespace(selectedCluster, name)
      message.success('命名空间已删除')
      fetchNamespaces()
    } catch (error) {
      console.error('Delete failed:', error)
    }
  }

  const handleViewDetail = async (name: string) => {
    try {
      const res = await getNamespaceDetail(selectedCluster, name)
      setSelectedNS(res.data)
      setDetailModalVisible(true)
    } catch (error) {
      console.error('Failed to fetch namespace detail:', error)
    }
  }

  const getNamespaceColor = (name: string) => {
    if (name === 'default') return 'blue'
    if (name.startsWith('kube-')) return 'orange'
    return 'green'
  }

  const columns: ColumnsType<Namespace> = [
    {
      title: '命名空间',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <Space>
          <FolderOutlined style={{ color: getNamespaceColor(name) === 'blue' ? '#1890ff' : getNamespaceColor(name) === 'orange' ? '#fa8c16' : '#52c41a' }} />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <StatusTag status={status} />,
    },
    {
      title: '类型',
      key: 'type',
      render: (_: any, record: Namespace) => {
        if (record.name === 'default') return <Tag color="blue">默认</Tag>
        if (record.name.startsWith('kube-')) return <Tag color="orange">系统</Tag>
        return <Tag color="green">用户</Tag>
      },
    },
    {
      title: '年龄',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_: any, record: Namespace) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record.name)} />
          </Tooltip>
          {!record.name.startsWith('kube-') && record.name !== 'default' && (
            <Popconfirm title="确定删除吗？删除后该命名空间下的所有资源将被删除！" onConfirm={() => handleDelete(record.name)}>
              <Tooltip title="删除">
                <Button type="link" danger icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>命名空间管理</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Button icon={<SyncOutlined />} onClick={fetchNamespaces}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建命名空间</Button>
        </Space>
      </div>

      <Card>
        <Table
          columns={columns}
          dataSource={namespaces}
          rowKey="name"
          loading={loading}
          pagination={false}
        />
      </Card>

      {/* 创建 Modal */}
      <Modal
        title="创建命名空间"
        open={createModalVisible}
        onCancel={() => { setCreateModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item
            name="name"
            label="名称"
            rules={[
              { required: true, message: '请输入命名空间名称' },
              { pattern: /^[a-z][a-z0-9-]*$/, message: '只能包含小写字母、数字和连字符，且以字母开头' },
            ]}
          >
            <Input placeholder="例如: my-app" />
          </Form.Item>
          <Form.Item name="labels" label="标签 (可选)" help="格式: key1=value1,key2=value2">
            <Input placeholder="env=production,team=backend" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 详情 Modal */}
      <Modal
        title={`命名空间详情: ${selectedNS?.name}`}
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={600}
      >
        {selectedNS && (
          <div>
            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
              <Col span={8}>
                <Statistic title="状态" value={selectedNS.status} />
              </Col>
              <Col span={8}>
                <Statistic title="年龄" value={selectedNS.age} />
              </Col>
            </Row>

            <Title level={5}>资源统计</Title>
            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
              <Col span={8}>
                <Card size="small">
                  <Statistic title="Pods" value={selectedNS.resources?.pods || 0} />
                </Card>
              </Col>
              <Col span={8}>
                <Card size="small">
                  <Statistic title="Services" value={selectedNS.resources?.services || 0} />
                </Card>
              </Col>
              <Col span={8}>
                <Card size="small">
                  <Statistic title="Deployments" value={selectedNS.resources?.deployments || 0} />
                </Card>
              </Col>
            </Row>
            <Row gutter={[16, 16]}>
              <Col span={12}>
                <Card size="small">
                  <Statistic title="ConfigMaps" value={selectedNS.resources?.configmaps || 0} />
                </Card>
              </Col>
              <Col span={12}>
                <Card size="small">
                  <Statistic title="Secrets" value={selectedNS.resources?.secrets || 0} />
                </Card>
              </Col>
            </Row>

            {selectedNS.labels && Object.keys(selectedNS.labels).length > 0 && (
              <>
                <Title level={5} style={{ marginTop: 24 }}>标签</Title>
                <Space size={[0, 8]} wrap>
                  {Object.entries(selectedNS.labels).map(([k, v]) => (
                    <Tag key={k}>{k}={v}</Tag>
                  ))}
                </Space>
              </>
            )}
          </div>
        )}
      </Modal>
    </div>
  )
}

export default NamespaceManagement
