import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, Select, Row, Col, Statistic, Progress
} from 'antd'
import {
  ReloadOutlined, CloudServerOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { get } from '../../api/request'

const { Title, Text } = Typography

interface NamespaceCost {
  namespace: string
  cpu_request: number
  memory_request: number
  pod_count: number
  cpu_cost: number
  memory_cost: number
  total_cost: number
}

const ResourceCost: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [costs, setCosts] = useState<NamespaceCost[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchCosts() }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchCosts = async () => {
    setLoading(true)
    try {
      await get<{ code: number; data: NamespaceCost[] }>(`/clusters/${selectedCluster}/workloads/metrics/overview`)
      // 模拟成本数据（实际应从 metrics API 获取）
      const namespaces = ['default', 'kube-system', 'kubepilot', 'monitoring']
      const mockCosts: NamespaceCost[] = namespaces.map(ns => ({
        namespace: ns,
        cpu_request: Math.floor(Math.random() * 8000),
        memory_request: Math.floor(Math.random() * 16384),
        pod_count: Math.floor(Math.random() * 20) + 1,
        cpu_cost: Math.floor(Math.random() * 500),
        memory_cost: Math.floor(Math.random() * 300),
        total_cost: 0,
      }))
      mockCosts.forEach(c => c.total_cost = c.cpu_cost + c.memory_cost)
      setCosts(mockCosts)
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const totalCost = costs.reduce((sum, c) => sum + c.total_cost, 0)

  const columns: ColumnsType<NamespaceCost> = [
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: 'CPU 请求', dataIndex: 'cpu_request', key: 'cpu',
      render: (v) => `${v}m`
    },
    {
      title: '内存请求', dataIndex: 'memory_request', key: 'memory',
      render: (v) => `${(v / 1024).toFixed(1)}Gi`
    },
    { title: 'Pod 数', dataIndex: 'pod_count', key: 'pods' },
    {
      title: 'CPU 成本', dataIndex: 'cpu_cost', key: 'cpu_cost',
      render: (v) => <Text>¥{v}</Text>
    },
    {
      title: '内存成本', dataIndex: 'memory_cost', key: 'mem_cost',
      render: (v) => <Text>¥{v}</Text>
    },
    {
      title: '总成本', dataIndex: 'total_cost', key: 'total',
      render: (v) => <Text strong>¥{v}</Text>,
      sorter: (a, b) => a.total_cost - b.total_cost,
    },
    {
      title: '占比', key: 'ratio',
      render: (_, r) => (
        <Progress
          percent={totalCost > 0 ? Math.round((r.total_cost / totalCost) * 100) : 0}
          size="small"
          style={{ width: 100 }}
        />
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>💰 资源成本分析</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchCosts}>刷新</Button>
        </Space>
      </div>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={8}>
          <Card>
            <Statistic
              title="总成本（月估算）"
              value={totalCost}
              prefix="¥"
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="命名空间数"
              value={costs.length}
              prefix={<CloudServerOutlined />}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="平均成本"
              value={costs.length > 0 ? Math.round(totalCost / costs.length) : 0}
              prefix="¥"
            />
          </Card>
        </Col>
      </Row>

      <Card>
        <Table columns={columns} dataSource={costs} rowKey="namespace" loading={loading} />
      </Card>
    </div>
  )
}

export default ResourceCost
