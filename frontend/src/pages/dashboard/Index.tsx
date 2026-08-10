import { useEffect, useState, useCallback } from 'react'
import { Row, Col, Card, Statistic, Table, Tag, Typography, Space, Progress, Button } from 'antd'
import {
  ClusterOutlined,
  CloudServerOutlined,
  AlertOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  CloseCircleOutlined,
  AppstoreOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import { getClusterList, Cluster } from '../../api/cluster'
import { getClusterOverview, ClusterOverview } from '../../api/metrics'
import { listModules, ModuleStatus } from '../../api/modules'
import { useInterval } from '../../hooks/useInterval'
import { moduleHomePath } from '../../constants/modules'

const { Title, Text } = Typography

const MODULE_REFRESH_MS = 20000

const Dashboard: React.FC = () => {
  const navigate = useNavigate()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [overview, setOverview] = useState<ClusterOverview | null>(null)
  const [loading, setLoading] = useState(false)
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [modules, setModules] = useState<ModuleStatus[]>([])

  const fetchModules = useCallback(async () => {
    try {
      const res = await listModules()
      setModules(res.data || [])
    } catch (error) {
      console.error('Failed to fetch modules:', error)
    }
  }, [])

  useEffect(() => {
    fetchClusters()
    fetchModules()
  }, [fetchModules])

  useInterval(fetchModules, MODULE_REFRESH_MS, true)

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

  const enabledMods = modules.filter((m) => m.enabled)
  const unhealthy = enabledMods.filter((m) => !m.healthy)
  const warned = enabledMods.filter((m) => m.healthy && typeof m.details?.health_warning === 'string')

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

      {enabledMods.length > 0 && (
        <Card
          size="small"
          style={{ marginBottom: 24 }}
          title={
            <Space>
              <AppstoreOutlined />
              <span>模块健康</span>
            </Space>
          }
          extra={
            <Button type="link" size="small" onClick={() => navigate('/system/modules')}>
              全部模块
            </Button>
          }
        >
          <Space wrap size={[8, 8]}>
            <Tag color="success">健康 {enabledMods.length - unhealthy.length}/{enabledMods.length}</Tag>
            {unhealthy.map((m) => (
              <Tag
                key={m.name}
                color="error"
                icon={<CloseCircleOutlined />}
                style={{ cursor: 'pointer' }}
                onClick={() => navigate(moduleHomePath(m.name))}
              >
                {m.name}: {m.health_error || '异常'}
              </Tag>
            ))}
            {warned.map((m) => (
              <Tag
                key={`w-${m.name}`}
                color="warning"
                icon={<WarningOutlined />}
                style={{ cursor: 'pointer' }}
                onClick={() => navigate(moduleHomePath(m.name))}
              >
                {m.name}: {String(m.details?.health_warning)}
              </Tag>
            ))}
            {unhealthy.length === 0 && warned.length === 0 && (
              <Text type="secondary">所有已启用模块运行正常</Text>
            )}
          </Space>
        </Card>
      )}

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
