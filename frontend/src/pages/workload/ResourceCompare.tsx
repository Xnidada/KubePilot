import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Tag, Row, Col, List, Empty
} from 'antd'
import {
  SwapOutlined, CheckCircleOutlined, CloseCircleOutlined, MinusCircleOutlined
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { compareResources, CompareResult } from '../../api/workload'

const { Title, Text } = Typography

const ResourceCompare: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [clusterA, setClusterA] = useState<number>(0)
  const [clusterB, setClusterB] = useState<number>(0)
  const [resourceType, setResourceType] = useState<string>('deployment')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<CompareResult | null>(null)

  useEffect(() => { fetchClusters() }, [])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data && res.data.length >= 2) {
        setClusterA(res.data[0].id)
        setClusterB(res.data[1].id)
      } else if (res.data && res.data.length === 1) {
        setClusterA(res.data[0].id)
        setClusterB(res.data[0].id)
      }
    } catch (e) { console.error(e) }
  }

  const handleCompare = async () => {
    if (!clusterA || !clusterB) {
      message.warning('请选择两个集群')
      return
    }
    setLoading(true)
    try {
      const res = await compareResources({
        cluster_a: clusterA,
        cluster_b: clusterB,
        resource_type: resourceType,
      })
      setResult(res.data)
    } catch (e) { message.error('对比失败') }
    finally { setLoading(false) }
  }

  const getClusterName = (id: number) => {
    return clusters.find(c => c.id === id)?.display_name || clusters.find(c => c.id === id)?.name || `集群 ${id}`
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>资源对比</Title>
      </div>

      <Card style={{ marginBottom: 16 }}>
        <Space size="large" wrap>
          <div>
            <Text strong>集群 A</Text>
            <Select value={clusterA} onChange={setClusterA} style={{ width: 200, display: 'block', marginTop: 8 }}
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          </div>
          <Button icon={<SwapOutlined />} style={{ marginTop: 24 }}
            onClick={() => { setClusterA(clusterB); setClusterB(clusterA) }}>
            交换
          </Button>
          <div>
            <Text strong>集群 B</Text>
            <Select value={clusterB} onChange={setClusterB} style={{ width: 200, display: 'block', marginTop: 8 }}
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          </div>
          <div>
            <Text strong>资源类型</Text>
            <Select value={resourceType} onChange={setResourceType} style={{ width: 150, display: 'block', marginTop: 8 }}
              options={[
                { label: 'Deployment', value: 'deployment' },
                { label: 'Service', value: 'service' },
                { label: 'Pod', value: 'pod' },
                { label: 'ConfigMap', value: 'configmap' },
                { label: 'Namespace', value: 'namespace' },
              ]} />
          </div>
          <Button type="primary" onClick={handleCompare} loading={loading} style={{ marginTop: 24 }}>
            开始对比
          </Button>
        </Space>
      </Card>

      {result && (
        <Row gutter={16}>
          <Col span={8}>
            <Card
              title={<><MinusCircleOutlined style={{ color: '#1890ff' }} /> 仅在 {getClusterName(result.cluster_a)} ({result.only_in_a.length})</>}
              size="small"
            >
              {result.only_in_a.length > 0 ? (
                <List
                  size="small"
                  dataSource={result.only_in_a}
                  renderItem={(item) => (
                    <List.Item><Tag color="blue">{item}</Tag></List.Item>
                  )}
                />
              ) : (
                <Empty description="无" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              )}
            </Card>
          </Col>
          <Col span={8}>
            <Card
              title={<><CheckCircleOutlined style={{ color: '#52c41a' }} /> 两者共有 ({result.in_both.length})</>}
              size="small"
            >
              {result.in_both.length > 0 ? (
                <List
                  size="small"
                  dataSource={result.in_both}
                  renderItem={(item) => (
                    <List.Item><Tag color="green">{item}</Tag></List.Item>
                  )}
                />
              ) : (
                <Empty description="无" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              )}
            </Card>
          </Col>
          <Col span={8}>
            <Card
              title={<><CloseCircleOutlined style={{ color: '#ff4d4f' }} /> 仅在 {getClusterName(result.cluster_b)} ({result.only_in_b.length})</>}
              size="small"
            >
              {result.only_in_b.length > 0 ? (
                <List
                  size="small"
                  dataSource={result.only_in_b}
                  renderItem={(item) => (
                    <List.Item><Tag color="red">{item}</Tag></List.Item>
                  )}
                />
              ) : (
                <Empty description="无" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              )}
            </Card>
          </Col>
        </Row>
      )}
    </div>
  )
}

export default ResourceCompare
