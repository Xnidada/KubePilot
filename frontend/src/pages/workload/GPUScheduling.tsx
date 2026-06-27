import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, Select, Tag, Progress, Row, Col, Statistic, Empty
} from 'antd'
import { ReloadOutlined, CloudServerOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNodes } from '../../api/workload'
import { get } from '../../api/request'

const { Title } = Typography

interface GPUInfo {
  node: string
  gpu_type: string
  total: number
  allocated: number
  free: number
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
      // 获取节点信息，提取 GPU 资源
      const nodesRes = await getNodes(selectedCluster)
      const nodes = nodesRes.data || []

      const gpuList: GPUInfo[] = []

      for (const node of nodes) {
        // 从节点获取 GPU 信息（通过 metrics API 或直接查询）
        try {
          const nodeDetail = await get<{ code: number; data: any }>(
            `/clusters/${selectedCluster}/workloads/nodes/${node.name}`
          )
          if (nodeDetail.data) {
            const capacity = nodeDetail.data.capacity || {}
            const allocated = nodeDetail.data.allocatable || {}

            // 检查 nvidia.com/gpu
            const gpuCapacity = parseInt(capacity['nvidia.com/gpu'] || '0')
            const gpuAllocatable = parseInt(allocated['nvidia.com/gpu'] || '0')

            if (gpuCapacity > 0) {
              gpuList.push({
                node: node.name,
                gpu_type: 'nvidia.com/gpu',
                total: gpuCapacity,
                allocated: gpuCapacity - gpuAllocatable,
                free: gpuAllocatable,
              })
            }
          }
        } catch (e) {
          // 节点没有 GPU
        }
      }

      setGpus(gpuList)
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const totalGPU = gpus.reduce((sum, g) => sum + g.total, 0)
  const allocatedGPU = gpus.reduce((sum, g) => sum + g.allocated, 0)
  const freeGPU = totalGPU - allocatedGPU

  const columns: ColumnsType<GPUInfo> = [
    { title: '节点', dataIndex: 'node', key: 'node' },
    { title: 'GPU 类型', dataIndex: 'gpu_type', key: 'type', render: (v) => <Tag color="blue">{v}</Tag> },
    { title: '总数', dataIndex: 'total', key: 'total' },
    { title: '已分配', dataIndex: 'allocated', key: 'allocated', render: (v) => <Tag color={v > 0 ? 'warning' : 'default'}>{v}</Tag> },
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
            <Statistic title="空闲" value={freeGPU} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
      </Row>

      <Card title="节点 GPU 信息">
        {gpus.length === 0 ? (
          <Empty description="未发现 GPU 资源。请确保节点已安装 NVIDIA 设备插件。" />
        ) : (
          <Table columns={columns} dataSource={gpus} rowKey="node" loading={loading} pagination={false} />
        )}
      </Card>
    </div>
  )
}

export default GPUScheduling
