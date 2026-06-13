import { useEffect, useState } from 'react'
import { Row, Col, Card, Statistic, Table, Tag, Typography, Space, Progress } from 'antd'
import {
  ClusterOutlined,
  CloudServerOutlined,
  AlertOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getClusterOverview, ClusterOverview } from '../../api/metrics'

const { Title, Text } = Typography

const Dashboard: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [overview, setOverview] = useState<ClusterOverview | null>(null)
  const [loading, setLoading] = useState(false)
  const [selectedCluster, setSelectedCluster] = useState<number>(0)

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchOverview()
    }
  }, [selectedCluster])

  const fetchClusters = async () => {
    setLoading(true)
    try {
      const res = await getClusterList(1, 5)
      setClusters(res.data || [])
      if (res.data && res.data.length > 0) {
        setSelectedCluster(res.data[0].id)
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchOverview = async () => {
    try {
      const res = await getClusterOverview(selectedCluster)
      setOverview(res.data)
    } catch (error) {
      console.error('Failed to fetch overview:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string; icon: React.ReactNode }> = {
      connected: { color: 'success', icon: <CheckCircleOutlined /> },
      error: { color: 'error', icon: <CloseCircleOutlined /> },
      unknown: { color: 'default', icon: <WarningOutlined /> },
    }
    const config = statusMap[status] || statusMap.unknown
    return (
      <Tag color={config.color} icon={config.icon}>
        {status}
      </Tag>
    )
  }

  const columns: ColumnsType<Cluster> = [
    {
      title: '集群名称',
      dataIndex: 'display_name',
      key: 'display_name',
      render: (text, record) => text || record.name,
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
      render: (v) => v || '-',
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
      render: (v) => v || '-',
    },
    {
      title: '内存',
      dataIndex: 'memory_capacity',
      key: 'memory_capacity',
      render: (v) => v || '-',
    },
  ]

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>
        仪表盘
      </Title>
      <Row gutter={[24, 24]}>
        <Col xs={24} sm={12} lg={6}>
          <Card hoverable>
            <Statistic
              title="集群总数"
              value={overview?.node_count || clusters.length}
              prefix={<ClusterOutlined style={{ color: '#1890ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card hoverable>
            <Statistic
              title="Deployment"
              value={overview?.deployment_count || 0}
              prefix={<CloudServerOutlined style={{ color: '#52c41a' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card hoverable>
            <Statistic
              title="Pod 总数"
              value={overview?.pod_count || 0}
              prefix={<CloudServerOutlined style={{ color: '#722ed1' }} />}
              suffix={overview ? <Text type="secondary" style={{ fontSize: 14 }}>/ {overview.pod_running} 运行中</Text> : null}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card hoverable>
            <Statistic
              title="CPU 分配率"
              value={overview?.cpu_allocated_percent || 0}
              precision={1}
              suffix="%"
              prefix={<AlertOutlined style={{ color: (overview?.cpu_allocated_percent || 0) > 80 ? '#ff4d4f' : '#52c41a' }} />}
              valueStyle={{ color: (overview?.cpu_allocated_percent || 0) > 80 ? '#ff4d4f' : '#52c41a' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[24, 24]} style={{ marginTop: 24 }}>
        <Col xs={24} lg={16}>
          <Card title="集群列表" loading={loading}>
            <Table
              columns={columns}
              dataSource={clusters}
              rowKey="id"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="资源使用">
            <Space direction="vertical" style={{ width: '100%' }} size="large">
              <div>
                <Text>CPU 分配率</Text>
                <Progress
                  percent={overview?.cpu_allocated_percent || 0}
                  status={overview && overview.cpu_allocated_percent > 80 ? 'exception' : 'active'}
                  format={(percent) => `${percent?.toFixed(1)}%`}
                />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {overview?.cpu_allocated_m || 0}m / {overview?.cpu_capacity_m || 0}m
                </Text>
              </div>
              <div>
                <Text>内存分配率</Text>
                <Progress
                  percent={overview?.memory_allocated_percent || 0}
                  status={overview && overview.memory_allocated_percent > 80 ? 'exception' : 'active'}
                  format={(percent) => `${percent?.toFixed(1)}%`}
                />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {overview?.memory_allocated_mi || 0}Mi / {overview?.memory_capacity_mi || 0}Mi
                </Text>
              </div>
              <div>
                <Text>Pod 状态</Text>
                <div style={{ marginTop: 8 }}>
                  <Space>
                    <Tag color="success">运行中: {overview?.pod_running || 0}</Tag>
                    <Tag color="warning">等待中: {overview?.pod_pending || 0}</Tag>
                    <Tag color="error">失败: {overview?.pod_failed || 0}</Tag>
                  </Space>
                </div>
              </div>
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Dashboard
