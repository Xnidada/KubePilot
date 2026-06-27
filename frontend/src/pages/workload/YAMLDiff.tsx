import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, Row, Col, Input, message
} from 'antd'
import { SwapOutlined, DiffOutlined } from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getDeployments, getPods, getServices } from '../../api/workload'
import { get } from '../../api/request'

const { Title } = Typography
const { TextArea } = Input

const YAMLDiff: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [resourceType, setResourceType] = useState('deployment')
  const [namespace, setNamespace] = useState('default')
  const [resourceName, setResourceName] = useState('')
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [resourceNames, setResourceNames] = useState<string[]>([])
  const [yamlA, setYamlA] = useState('')
  const [yamlB, setYamlB] = useState('')
  const [diffResult, setDiffResult] = useState<string[]>([])

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchNamespaces() }, [selectedCluster])
  useEffect(() => { if (selectedCluster && resourceType) fetchResourceNames() }, [selectedCluster, resourceType, namespace])

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

  const fetchResourceNames = async () => {
    try {
      let names: string[] = []
      switch (resourceType) {
        case 'deployment':
          const d = await getDeployments(selectedCluster, namespace)
          names = (d.data || []).map((r: any) => r.name)
          break
        case 'pod':
          const p = await getPods(selectedCluster, namespace)
          names = (p.data || []).map((r: any) => r.name)
          break
        case 'service':
          const s = await getServices(selectedCluster, namespace)
          names = (s.data || []).map((r: any) => r.name)
          break
      }
      setResourceNames(names)
    } catch (e) { console.error(e) }
  }

  const fetchYAML = async () => {
    if (!resourceName) {
      message.warning('请选择资源')
      return
    }
    // 转换资源类型为复数形式
    const typeMap: Record<string, string> = {
      'pod': 'pods',
      'deployment': 'deployments',
      'service': 'services',
    }
    const apiType = typeMap[resourceType] || resourceType
    try {
      const res = await get<{ code: number; data: { yaml: string } }>(
        `/clusters/${selectedCluster}/workloads/yaml/${apiType}/${namespace}/${resourceName}`
      )
      if (res.data?.yaml) {
        setYamlA(res.data.yaml)
        setYamlB(res.data.yaml)
      }
    } catch (e) { message.error('获取 YAML 失败') }
  }

  const computeDiff = () => {
    const linesA = yamlA.split('\n')
    const linesB = yamlB.split('\n')
    const diff: string[] = []

    const maxLen = Math.max(linesA.length, linesB.length)
    for (let i = 0; i < maxLen; i++) {
      const a = linesA[i] || ''
      const b = linesB[i] || ''
      if (a !== b) {
        if (a) diff.push(`- ${a}`)
        if (b) diff.push(`+ ${b}`)
      } else {
        diff.push(`  ${a}`)
      }
    }

    setDiffResult(diff)
  }

  return (
    <div>
      <Title level={4}>📝 YAML Diff 对比</Title>

      <Card style={{ marginBottom: 16 }}>
        <Space wrap>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={resourceType} onChange={setResourceType} style={{ width: 150 }}
            options={[
              { label: 'Deployment', value: 'deployment' },
              { label: 'Pod', value: 'pod' },
              { label: 'Service', value: 'service' },
            ]} />
          <Select value={namespace} onChange={setNamespace} style={{ width: 150 }}
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Select value={resourceName} onChange={setResourceName} style={{ width: 200 }}
            showSearch placeholder="选择资源"
            options={resourceNames.map(n => ({ label: n, value: n }))} />
          <Button icon={<SwapOutlined />} onClick={fetchYAML}>获取 YAML</Button>
          <Button type="primary" icon={<DiffOutlined />} onClick={computeDiff}>对比</Button>
        </Space>
      </Card>

      <Row gutter={16}>
        <Col span={12}>
          <Card title="原始 YAML" size="small">
            <TextArea
              value={yamlA}
              onChange={(e) => setYamlA(e.target.value)}
              rows={15}
              style={{ fontFamily: 'monospace', fontSize: 12 }}
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="修改后 YAML" size="small">
            <TextArea
              value={yamlB}
              onChange={(e) => setYamlB(e.target.value)}
              rows={15}
              style={{ fontFamily: 'monospace', fontSize: 12 }}
            />
          </Card>
        </Col>
      </Row>

      {diffResult.length > 0 && (
        <Card title="差异结果" style={{ marginTop: 16 }}>
          <pre style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 8,
            overflow: 'auto',
            maxHeight: 400,
          }}>
            {diffResult.map((line, i) => (
              <div key={i} style={{
                color: line.startsWith('+') ? '#4ec9b0' : line.startsWith('-') ? '#f44747' : '#d4d4d4',
              }}>
                {line}
              </div>
            ))}
          </pre>
        </Card>
      )}
    </div>
  )
}

export default YAMLDiff
