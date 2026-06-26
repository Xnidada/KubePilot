import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Checkbox, Alert, Tag, Row, Col
} from 'antd'
import { CopyOutlined } from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getDeployments, getServices } from '../../api/workload'
import { post } from '../../api/request'

const { Title, Text } = Typography

const EnvClone: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [sourceNamespace, setSourceNamespace] = useState('')
  const [targetNamespace, setTargetNamespace] = useState('')
  const [selectedTypes, setSelectedTypes] = useState<string[]>(['deployments', 'services', 'configmaps'])
  const [resources, setResources] = useState<Record<string, string[]>>({})
  const [loading, setLoading] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchNamespaces() }, [selectedCluster])
  useEffect(() => { if (selectedCluster && sourceNamespace) fetchResources() }, [selectedCluster, sourceNamespace])

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

  const fetchResources = async () => {
    try {
      const result: Record<string, string[]> = {}

      try {
        const deploys = await getDeployments(selectedCluster, sourceNamespace)
        result['deployments'] = (deploys.data || []).map((d: any) => d.name)
      } catch (e) {}

      try {
        const svcs = await getServices(selectedCluster, sourceNamespace)
        result['services'] = (svcs.data || []).map((s: any) => s.name)
      } catch (e) {}

      setResources(result)
    } catch (e) { console.error(e) }
  }

  const handleClone = async () => {
    if (!sourceNamespace || !targetNamespace) {
      message.warning('请选择源和目标命名空间')
      return
    }
    if (sourceNamespace === targetNamespace) {
      message.warning('源和目标命名空间不能相同')
      return
    }

    setLoading(true)
    try {
      // 调用后端克隆 API
      await post(`/clusters/${selectedCluster}/workloads/namespaces/clone`, {
        source: sourceNamespace,
        target: targetNamespace,
        types: selectedTypes,
      })
      message.success('环境克隆成功')
    } catch (e) {
      message.error('克隆失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <Title level={4}>📋 环境克隆</Title>

      <Alert
        message="环境克隆说明"
        description="从源命名空间克隆资源配置到目标命名空间。支持 Deployment、Service、ConfigMap、Secret 等资源。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Row gutter={16}>
        <Col span={12}>
          <Card title="源配置">
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>集群</Text>
                <Select
                  value={selectedCluster}
                  onChange={setSelectedCluster}
                  style={{ width: '100%', marginTop: 8 }}
                  options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
                />
              </div>
              <div>
                <Text strong>源命名空间</Text>
                <Select
                  value={sourceNamespace}
                  onChange={setSourceNamespace}
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择源命名空间"
                  options={namespaces.map(ns => ({ label: ns, value: ns }))}
                />
              </div>
              <div>
                <Text strong>目标命名空间</Text>
                <Select
                  value={targetNamespace}
                  onChange={setTargetNamespace}
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择目标命名空间"
                  options={namespaces.map(ns => ({ label: ns, value: ns }))}
                />
              </div>
              <div>
                <Text strong>资源类型</Text>
                <div style={{ marginTop: 8 }}>
                  <Checkbox.Group
                    value={selectedTypes}
                    onChange={(checked) => setSelectedTypes(checked as string[])}
                    options={[
                      { label: 'Deployment', value: 'deployments' },
                      { label: 'Service', value: 'services' },
                      { label: 'ConfigMap', value: 'configmaps' },
                      { label: 'Secret', value: 'secrets' },
                    ]}
                  />
                </div>
              </div>
              <Button
                type="primary"
                icon={<CopyOutlined />}
                onClick={handleClone}
                loading={loading}
                block
              >
                开始克隆
              </Button>
            </Space>
          </Card>
        </Col>

        <Col span={12}>
          <Card title="源资源列表">
            {Object.entries(resources).map(([type, names]) => (
              <div key={type} style={{ marginBottom: 16 }}>
                <Text strong style={{ textTransform: 'capitalize' }}>{type}</Text>
                <div style={{ marginTop: 8 }}>
                  {names.map(name => (
                    <Tag key={name} style={{ margin: '0 4px 4px 0' }}>{name}</Tag>
                  ))}
                  {names.length === 0 && <Text type="secondary">无</Text>}
                </div>
              </div>
            ))}
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default EnvClone
