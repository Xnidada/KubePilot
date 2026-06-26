import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Tag, Descriptions, List, Row, Col, Divider
} from 'antd'
import {
  SearchOutlined, WarningOutlined, CheckCircleOutlined, CloseCircleOutlined,
  ThunderboltOutlined
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getPods } from '../../api/workload'
import { diagnosePod, PodDiagnosis } from '../../api/ops'

const { Title, Text } = Typography

const PodDiagnosisPage: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [pods, setPods] = useState<any[]>([])
  const [selectedPod, setSelectedPod] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [diagnosis, setDiagnosis] = useState<PodDiagnosis | null>(null)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces() } }, [selectedCluster])
  useEffect(() => { if (selectedCluster && selectedNamespace) { fetchPods() } }, [selectedCluster, selectedNamespace])

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

  const fetchPods = async () => {
    try {
      const res = await getPods(selectedCluster, selectedNamespace)
      setPods(res.data || [])
    } catch (e) { console.error(e) }
  }

  const handleDiagnose = async () => {
    if (!selectedPod || !selectedNamespace) {
      message.warning('请选择 Pod')
      return
    }
    setLoading(true)
    try {
      const res = await diagnosePod(selectedCluster, selectedNamespace, selectedPod)
      setDiagnosis(res.data)
    } catch (e) { message.error('诊断失败') }
    finally { setLoading(false) }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'Running': return 'success'
      case 'Succeeded': return 'default'
      case 'Pending': return 'processing'
      case 'Failed': case 'CrashLoopBackOff': return 'error'
      default: return 'warning'
    }
  }

  return (
    <div>
      <Title level={4}>🔍 Pod 诊断面板</Title>

      <Card style={{ marginBottom: 16 }}>
        <Space wrap>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="选择命名空间" options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Select value={selectedPod} onChange={setSelectedPod} style={{ width: 250 }}
            placeholder="选择 Pod" showSearch
            options={pods.map(p => ({ label: `${p.name} (${p.status})`, value: p.name }))} />
          <Button type="primary" icon={<SearchOutlined />} onClick={handleDiagnose} loading={loading}>
            一键诊断
          </Button>
        </Space>
      </Card>

      {diagnosis && (
        <>
          {/* 基本信息 */}
          <Card title="📋 Pod 基本信息" style={{ marginBottom: 16 }}>
            <Descriptions column={3} size="small">
              <Descriptions.Item label="名称">{diagnosis.pod_name}</Descriptions.Item>
              <Descriptions.Item label="命名空间">{diagnosis.namespace}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={getStatusColor(diagnosis.status)}>{diagnosis.status}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="节点">{diagnosis.node}</Descriptions.Item>
              <Descriptions.Item label="IP">{diagnosis.ip}</Descriptions.Item>
              <Descriptions.Item label="重启次数">
                <Tag color={diagnosis.restarts > 5 ? 'error' : diagnosis.restarts > 0 ? 'warning' : 'success'}>
                  {diagnosis.restarts}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="年龄">{diagnosis.age}</Descriptions.Item>
              <Descriptions.Item label="QoS">{diagnosis.qos_class}</Descriptions.Item>
              <Descriptions.Item label="Owner">{diagnosis.owner_ref || '-'}</Descriptions.Item>
            </Descriptions>
          </Card>

          {/* 问题和建议 */}
          {diagnosis.problems && diagnosis.problems.length > 0 && (
            <Card title="⚠️ 发现问题" style={{ marginBottom: 16 }}>
              <List
                dataSource={diagnosis.problems}
                renderItem={(problem: string) => (
                  <List.Item>
                    <Space>
                      <WarningOutlined style={{ color: '#ff4d4f' }} />
                      <Text type="danger">{problem}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </Card>
          )}

          {diagnosis.suggestions && diagnosis.suggestions.length > 0 && (
            <Card title="💡 建议" style={{ marginBottom: 16 }}>
              <List
                dataSource={diagnosis.suggestions}
                renderItem={(suggestion: string) => (
                  <List.Item>
                    <Space>
                      <ThunderboltOutlined style={{ color: '#1890ff' }} />
                      <Text>{suggestion}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </Card>
          )}

          <Row gutter={16}>
            {/* 容器状态 */}
            <Col span={12}>
              <Card title="🐳 容器状态" style={{ marginBottom: 16 }}>
                {diagnosis.containers.map((c, i) => (
                  <div key={i} style={{ marginBottom: 12, padding: 12, background: '#f5f5f5', borderRadius: 8 }}>
                    <Space>
                      <Text strong>{c.name}</Text>
                      <Tag color={c.ready ? 'success' : 'error'}>{c.ready ? 'Ready' : 'Not Ready'}</Tag>
                      <Tag>{c.state}</Tag>
                      {c.restart_count > 0 && <Tag color="warning">重启 {c.restart_count} 次</Tag>}
                    </Space>
                    <div style={{ marginTop: 4 }}>
                      <Text type="secondary">镜像: {c.image}</Text>
                    </div>
                    {c.reason && (
                      <div style={{ marginTop: 4 }}>
                        <Text type="danger">原因: {c.reason}</Text>
                      </div>
                    )}
                  </div>
                ))}
              </Card>
            </Col>

            {/* 资源使用 */}
            <Col span={12}>
              <Card title="📊 资源配置" style={{ marginBottom: 16 }}>
                <Descriptions column={2} size="small">
                  <Descriptions.Item label="CPU 请求">{diagnosis.resource_usage.cpu_request || '-'}</Descriptions.Item>
                  <Descriptions.Item label="CPU 限制">{diagnosis.resource_usage.cpu_limit || '-'}</Descriptions.Item>
                  <Descriptions.Item label="内存请求">{diagnosis.resource_usage.mem_request || '-'}</Descriptions.Item>
                  <Descriptions.Item label="内存限制">{diagnosis.resource_usage.mem_limit || '-'}</Descriptions.Item>
                </Descriptions>
                <Divider />
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Text>Scheduled</Text>
                    {diagnosis.resource_usage.pod_scheduled ? <CheckCircleOutlined style={{ color: '#52c41a' }} /> : <CloseCircleOutlined style={{ color: '#ff4d4f' }} />}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Text>Initialized</Text>
                    {diagnosis.resource_usage.initialized ? <CheckCircleOutlined style={{ color: '#52c41a' }} /> : <CloseCircleOutlined style={{ color: '#ff4d4f' }} />}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Text>Containers Ready</Text>
                    {diagnosis.resource_usage.containers_ready ? <CheckCircleOutlined style={{ color: '#52c41a' }} /> : <CloseCircleOutlined style={{ color: '#ff4d4f' }} />}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Text>Ready</Text>
                    {diagnosis.resource_usage.ready ? <CheckCircleOutlined style={{ color: '#52c41a' }} /> : <CloseCircleOutlined style={{ color: '#ff4d4f' }} />}
                  </div>
                </Space>
              </Card>
            </Col>
          </Row>

          {/* 事件 */}
          <Card title="📅 事件" style={{ marginBottom: 16 }}>
            <List
              size="small"
              dataSource={diagnosis.events || []}
              locale={{ emptyText: '暂无事件' }}
              renderItem={(event: any) => (
                <List.Item>
                  <Space style={{ width: '100%' }}>
                    <Tag color={event.type === 'Warning' ? 'warning' : 'default'}>{event.type}</Tag>
                    <Tag>{event.reason}</Tag>
                    <Text>{event.message}</Text>
                    <Text type="secondary" style={{ marginLeft: 'auto' }}>{event.last_time}</Text>
                  </Space>
                </List.Item>
              )}
            />
          </Card>
        </>
      )}
    </div>
  )
}

export default PodDiagnosisPage
