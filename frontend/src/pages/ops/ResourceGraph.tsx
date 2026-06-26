import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Tag, Empty, Row, Col
} from 'antd'
import {
  ReloadOutlined
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { getResourceGraph, ResourceGraph as ResourceGraphType } from '../../api/ops'

const { Title, Text } = Typography

const ResourceGraphPage: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [graph, setGraph] = useState<ResourceGraphType | null>(null)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchGraph() } }, [selectedCluster, selectedNamespace])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(selectedCluster)
      setNamespaces(res.data || [])
    } catch (e) { console.error(e) }
  }

  const fetchGraph = async () => {
    try {
      const res = await getResourceGraph(selectedCluster, selectedNamespace || undefined)
      setGraph(res.data)
    } catch (e) { message.error('获取资源图失败') }
  }

  const getKindColor = (kind: string) => {
    const map: Record<string, string> = {
      Deployment: '#1890ff',
      ReplicaSet: '#722ed1',
      Pod: '#52c41a',
      Service: '#fa8c16',
    }
    return map[kind] || '#666'
  }

  const getStatusTag = (status: string) => {
    if (status.includes('/') && status.includes('0')) return <Tag color="error">{status}</Tag>
    if (status === 'Running' || status === 'Ready') return <Tag color="success">{status}</Tag>
    if (status === 'Pending') return <Tag color="processing">{status}</Tag>
    if (status === 'Failed') return <Tag color="error">{status}</Tag>
    return <Tag>{status}</Tag>
  }

  // 按类型分组节点
  const groupByKind = (nodes: ResourceGraphType['nodes']) => {
    const groups: Record<string, ResourceGraphType['nodes']> = {}
    nodes.forEach(n => {
      if (!groups[n.kind]) groups[n.kind] = []
      groups[n.kind].push(n)
    })
    return groups
  }

  const groups = graph ? groupByKind(graph.nodes) : {}

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>🔗 资源依赖图</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="所有命名空间" allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchGraph}>刷新</Button>
        </Space>
      </div>

      {!graph || graph.nodes.length === 0 ? (
        <Card><Empty description="暂无资源数据" /></Card>
      ) : (
        <>
          {/* 统计 */}
          <Row gutter={16} style={{ marginBottom: 16 }}>
            {Object.entries(groups).map(([kind, nodes]) => (
              <Col key={kind} span={6}>
                <Card>
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 28, fontWeight: 'bold', color: getKindColor(kind) }}>{nodes.length}</div>
                    <Text type="secondary">{kind}</Text>
                  </div>
                </Card>
              </Col>
            ))}
          </Row>

          {/* 资源列表 */}
          <Row gutter={16}>
            <Col span={8}>
              <Card title="🚀 Deployments" size="small">
                {(groups['Deployment'] || []).map(n => (
                  <div key={n.id} style={{ padding: '8px 0', borderBottom: '1px solid #f0f0f0' }}>
                    <Space>
                      <Text strong>{n.name}</Text>
                      {getStatusTag(n.status)}
                    </Space>
                    <div><Text type="secondary" style={{ fontSize: 12 }}>{n.namespace}</Text></div>
                  </div>
                ))}
              </Card>
            </Col>
            <Col span={8}>
              <Card title="📦 ReplicaSets" size="small">
                {(groups['ReplicaSet'] || []).map(n => (
                  <div key={n.id} style={{ padding: '8px 0', borderBottom: '1px solid #f0f0f0' }}>
                    <Space>
                      <Text>{n.name}</Text>
                      {getStatusTag(n.status)}
                    </Space>
                    <div><Text type="secondary" style={{ fontSize: 12 }}>{n.namespace}</Text></div>
                  </div>
                ))}
              </Card>
            </Col>
            <Col span={8}>
              <Card title="🐳 Pods" size="small">
                {(groups['Pod'] || []).map(n => (
                  <div key={n.id} style={{ padding: '8px 0', borderBottom: '1px solid #f0f0f0' }}>
                    <Space>
                      <Text>{n.name}</Text>
                      {getStatusTag(n.status)}
                    </Space>
                    <div><Text type="secondary" style={{ fontSize: 12 }}>{n.namespace}</Text></div>
                  </div>
                ))}
              </Card>
            </Col>
          </Row>

          {/* 关系图 */}
          <Card title="🔗 资源关系" style={{ marginTop: 16 }}>
            <div style={{ fontFamily: 'monospace', whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: 16, borderRadius: 8 }}>
              {graph.edges.map((edge, i) => (
                <div key={i} style={{ marginBottom: 8 }}>
                  <Tag color="blue">{edge.type}</Tag>
                  <Text code>{edge.source.split('/').pop()}</Text>
                  <Text type="secondary"> → </Text>
                  <Text code>{edge.target.split('/').pop()}</Text>
                </div>
              ))}
            </div>
          </Card>
        </>
      )}
    </div>
  )
}

export default ResourceGraphPage
