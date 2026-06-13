import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Card,
  Descriptions,
  Table,
  Tag,
  Button,
  Space,
  Spin,
  Typography,
  Row,
  Col,
  Statistic,
  Tabs,
  message,
} from 'antd'
import {
  ArrowLeftOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  WarningOutlined,
  CloudServerOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterDetail, getClusterInfo, healthCheckCluster, Cluster, ClusterNode } from '../../api/cluster'

const { Title } = Typography

const ClusterDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [cluster, setCluster] = useState<Cluster | null>(null)
  const [nodes, setNodes] = useState<ClusterNode[]>([])
  const [loading, setLoading] = useState(false)
  const [nodesLoading, setNodesLoading] = useState(false)

  useEffect(() => {
    if (id) {
      fetchClusterDetail(parseInt(id))
      fetchClusterNodes(parseInt(id))
    }
  }, [id])

  const fetchClusterDetail = async (clusterId: number) => {
    setLoading(true)
    try {
      const res = await getClusterDetail(clusterId)
      setCluster(res.data)
    } catch (error) {
      console.error('Failed to fetch cluster detail:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchClusterNodes = async (clusterId: number) => {
    setNodesLoading(true)
    try {
      const res = await getClusterInfo(clusterId)
      setNodes(res.data.nodes || [])
    } catch (error) {
      console.error('Failed to fetch cluster nodes:', error)
    } finally {
      setNodesLoading(false)
    }
  }

  const handleHealthCheck = async () => {
    if (!id) return
    try {
      await healthCheckCluster(parseInt(id))
      message.success('健康检查完成')
      fetchClusterDetail(parseInt(id))
    } catch (error) {
      console.error('Health check failed:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string; icon: React.ReactNode; text: string }> = {
      connected: { color: 'success', icon: <CheckCircleOutlined />, text: '已连接' },
      error: { color: 'error', icon: <CloseCircleOutlined />, text: '错误' },
      unknown: { color: 'default', icon: <WarningOutlined />, text: '未知' },
    }
    const config = statusMap[status] || statusMap.unknown
    return (
      <Tag color={config.color} icon={config.icon}>
        {config.text}
      </Tag>
    )
  }

  const nodeColumns: ColumnsType<ClusterNode> = [
    {
      title: '节点名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '状态',
      dataIndex: 'ready',
      key: 'ready',
      render: (ready) => (
        <Tag color={ready ? 'success' : 'error'} icon={ready ? <CheckCircleOutlined /> : <CloseCircleOutlined />}>
          {ready ? 'Ready' : 'NotReady'}
        </Tag>
      ),
    },
    {
      title: 'CPU',
      dataIndex: 'cpu_capacity',
      key: 'cpu_capacity',
    },
    {
      title: '内存',
      dataIndex: 'mem_capacity',
      key: 'mem_capacity',
    },
    {
      title: '操作系统',
      dataIndex: 'os',
      key: 'os',
      ellipsis: true,
    },
    {
      title: '容器运行时',
      dataIndex: 'container_rt',
      key: 'container_rt',
      ellipsis: true,
    },
    {
      title: 'Kubelet',
      dataIndex: 'kubelet_ver',
      key: 'kubelet_ver',
    },
  ]

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (!cluster) {
    return <div>集群不存在</div>
  }

  const readyNodes = nodes.filter((n) => n.ready).length

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/clusters')}>
            返回
          </Button>
          <Title level={4} style={{ margin: 0 }}>
            {cluster.display_name || cluster.name}
          </Title>
          {getStatusTag(cluster.status)}
        </Space>
        <Button icon={<SyncOutlined />} onClick={handleHealthCheck}>
          健康检查
        </Button>
      </div>

      <Tabs
        defaultActiveKey="overview"
        items={[
          {
            key: 'overview',
            label: '概览',
            children: (
              <Row gutter={[24, 24]}>
                <Col xs={24} lg={16}>
                  <Card title="基本信息">
                    <Descriptions column={{ xs: 1, sm: 2 }} bordered>
                      <Descriptions.Item label="集群名称">{cluster.name}</Descriptions.Item>
                      <Descriptions.Item label="显示名称">{cluster.display_name || '-'}</Descriptions.Item>
                      <Descriptions.Item label="API Server" span={2}>
                        {cluster.api_server}
                      </Descriptions.Item>
                      <Descriptions.Item label="状态">{getStatusTag(cluster.status)}</Descriptions.Item>
                      <Descriptions.Item label="版本">{cluster.version || '-'}</Descriptions.Item>
                      <Descriptions.Item label="节点数">{cluster.node_count}</Descriptions.Item>
                      <Descriptions.Item label="描述" span={2}>
                        {cluster.description || '-'}
                      </Descriptions.Item>
                      <Descriptions.Item label="标签" span={2}>
                        {cluster.tags || '-'}
                      </Descriptions.Item>
                      <Descriptions.Item label="创建时间">{cluster.created_at}</Descriptions.Item>
                      <Descriptions.Item label="最后检查">{cluster.last_health_check || '-'}</Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
                <Col xs={24} lg={8}>
                  <Card title="资源概览">
                    <Space direction="vertical" size="large" style={{ width: '100%' }}>
                      <Statistic
                        title="节点总数"
                        value={cluster.node_count}
                        prefix={<CloudServerOutlined />}
                      />
                      <Statistic title="CPU 容量" value={cluster.cpu_capacity || '-'} />
                      <Statistic title="内存容量" value={cluster.memory_capacity || '-'} />
                      <Statistic
                        title="健康节点"
                        value={readyNodes}
                        suffix={`/ ${nodes.length}`}
                        valueStyle={{ color: readyNodes === nodes.length ? '#3f8600' : '#cf1322' }}
                      />
                    </Space>
                  </Card>
                </Col>
              </Row>
            ),
          },
          {
            key: 'nodes',
            label: `节点 (${nodes.length})`,
            children: (
              <Card>
                <Table
                  columns={nodeColumns}
                  dataSource={nodes}
                  rowKey="name"
                  loading={nodesLoading}
                  pagination={false}
                />
              </Card>
            ),
          },
        ]}
      />
    </div>
  )
}

export default ClusterDetail
