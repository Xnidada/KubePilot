import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, Tag, Table, Progress, Row, Col
} from 'antd'
import {
  ReloadOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNodePressure, NodePressure } from '../../api/ops'

const { Title, Text } = Typography

const NodePressurePage: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [nodes, setNodes] = useState<NodePressure[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchNodes() }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchNodes = async () => {
    setLoading(true)
    try {
      const res = await getNodePressure(selectedCluster)
      setNodes(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const getPressureTag = (level: string) => {
    const map: Record<string, { color: string; text: string }> = {
      critical: { color: 'error', text: '严重' },
      high: { color: 'warning', text: '高' },
      medium: { color: 'processing', text: '中' },
      low: { color: 'success', text: '低' },
    }
    const cfg = map[level] || map.low
    return <Tag color={cfg.color}>{cfg.text}</Tag>
  }

  const columns: ColumnsType<NodePressure> = [
    { title: '节点', dataIndex: 'name', key: 'name' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => <Tag color={s === 'Ready' ? 'success' : 'error'}>{s}</Tag>
    },
    {
      title: '压力等级', dataIndex: 'pressure_level', key: 'pressure_level',
      render: (level) => getPressureTag(level)
    },
    {
      title: 'CPU', key: 'cpu',
      render: (_, r) => (
        <div style={{ width: 200 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <Text>{r.cpu_allocated}/{r.cpu_capacity}</Text>
            <Text strong>{r.cpu_percent.toFixed(1)}%</Text>
          </div>
          <Progress percent={r.cpu_percent} size="small"
            strokeColor={r.cpu_percent > 90 ? '#ff4d4f' : r.cpu_percent > 75 ? '#faad14' : '#52c41a'}
            showInfo={false} />
        </div>
      )
    },
    {
      title: '内存', key: 'memory',
      render: (_, r) => (
        <div style={{ width: 200 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <Text>{r.mem_allocated}/{r.mem_capacity}</Text>
            <Text strong>{r.mem_percent.toFixed(1)}%</Text>
          </div>
          <Progress percent={r.mem_percent} size="small"
            strokeColor={r.mem_percent > 90 ? '#ff4d4f' : r.mem_percent > 75 ? '#faad14' : '#52c41a'}
            showInfo={false} />
        </div>
      )
    },
    {
      title: 'Pod', key: 'pods',
      render: (_, r) => (
        <div style={{ width: 150 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <Text>{r.pod_count}/{r.pod_capacity}</Text>
            <Text strong>{r.pod_percent.toFixed(1)}%</Text>
          </div>
          <Progress percent={r.pod_percent} size="small"
            strokeColor={r.pod_percent > 90 ? '#ff4d4f' : r.pod_percent > 75 ? '#faad14' : '#52c41a'}
            showInfo={false} />
        </div>
      )
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>🖥️ 节点压力可视化</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchNodes}>刷新</Button>
        </Space>
      </div>

      {/* 概览卡片 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 32, fontWeight: 'bold' }}>{nodes.length}</div>
              <Text type="secondary">节点总数</Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 32, fontWeight: 'bold', color: '#ff4d4f' }}>
                {nodes.filter(n => n.pressure_level === 'critical' || n.pressure_level === 'high').length}
              </div>
              <Text type="secondary">高压力节点</Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 32, fontWeight: 'bold', color: '#52c41a' }}>
                {nodes.filter(n => n.status === 'Ready').length}
              </div>
              <Text type="secondary">就绪节点</Text>
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 32, fontWeight: 'bold', color: '#ff4d4f' }}>
                {nodes.filter(n => n.status !== 'Ready').length}
              </div>
              <Text type="secondary">异常节点</Text>
            </div>
          </Card>
        </Col>
      </Row>

      <Card>
        <Table columns={columns} dataSource={nodes} rowKey="name" loading={loading} />
      </Card>
    </div>
  )
}

export default NodePressurePage
