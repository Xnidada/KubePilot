import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, Select, Tag, Progress, Row, Col, Statistic
} from 'antd'
import { ReloadOutlined, CloudServerOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNodes } from '../../api/workload'

const { Title } = Typography

interface GPUInfo {
  node: string
  gpu_type: string
  total: number
  allocated: number
  free: number
  pods: string[]
}

const GPUScheduling: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [gpus, setGpus] = useState<GPUInfo[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchGPUInfo() }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchGPUInfo = async () => {
    setLoading(true)
    try {
      const res = await getNodes(selectedCluster)
      const nodes = res.data || []
      const gpuList: GPUInfo[] = nodes.map((node: any) => ({
        node: node.name,
        gpu_type: 'nvidia.com/gpu',
        total: Math.floor(Math.random() * 4), // 模拟数据
        allocated: Math.floor(Math.random() * 2),
        free: 0,
        pods: [],
      }))
      gpuList.forEach(g => g.free = g.total - g.allocated)
      setGpus(gpuList)
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const totalGPU = gpus.reduce((sum, g) => sum + g.total, 0)
  const allocatedGPU = gpus.reduce((sum, g) => sum + g.allocated, 0)

  const columns: ColumnsType<GPUInfo> = [
    { title: '节点', dataIndex: 'node', key: 'node' },
    { title: 'GPU 类型', dataIndex: 'gpu_type', key: 'type', render: (v) => <Tag>{v}</Tag> },
    { title: '总数', dataIndex: 'total', key: 'total' },
    { title: '已分配', dataIndex: 'allocated', key: 'allocated' },
    { title: '空闲', dataIndex: 'free', key: 'free', render: (v) => <Tag color={v > 0 ? 'success' : 'error'}>{v}</Tag> },
    {
      title: '使用率', key: 'usage',
      render: (_, r) => (
        <Progress
          percent={r.total > 0 ? Math.round((r.allocated / r.total) * 100) : 0}
          size="small"
          status={r.allocated >= r.total ? 'exception' : 'active'}
        />
      )
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>🎮 GPU 调度管理</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchGPUInfo}>刷新</Button>
        </Space>
      </div>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={8}>
          <Card>
            <Statistic title="GPU 总数" value={totalGPU} prefix={<CloudServerOutlined />} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="已分配" value={allocatedGPU} valueStyle={{ color: '#faad14' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="空闲" value={totalGPU - allocatedGPU} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
      </Row>

      <Card>
        <Table columns={columns} dataSource={gpus} rowKey="node" loading={loading} />
      </Card>
    </div>
  )
}

export default GPUScheduling
