import { useState, useEffect } from 'react'
import {
  Card,
  Tabs,
  Input,
  Button,
  Select,
  Space,
  Typography,
  message,
  List,
  Progress,
  Alert,
  Divider,
  Row,
  Col,
  Tooltip,
} from 'antd'
import {
  SearchOutlined,
  TranslationOutlined,
  FileTextOutlined,
  BookOutlined,
  CopyOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  SafetyOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getPods, getDeployments, getServices, getNodes } from '../../api/workload'
import {
  explainText,
  getResourceGuide,
  translateYAML,
  ExplainResponse,
  ResourceGuideResponse,
  TranslateYAMLResponse,
} from '../../api/aiops'
import MarkdownRenderer from '../../components/MarkdownRenderer'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

// 划词解释组件
const ExplainTab: React.FC<{ clusters: Cluster[] }> = ({ clusters }) => {
  const [text, setText] = useState('')
  const [context, setContext] = useState('')
  const [clusterId, setClusterId] = useState<number>(0)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ExplainResponse | null>(null)

  const handleExplain = async () => {
    if (!text.trim()) {
      message.warning('请输入需要解释的内容')
      return
    }
    setLoading(true)
    try {
      const res = await explainText({
        text: text.trim(),
        cluster_id: clusterId,
        context: context.trim(),
      })
      setResult(res.data)
    } catch (error) {
      message.error('解释失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCopy = () => {
    if (result?.explanation) {
      navigator.clipboard.writeText(result.explanation)
      message.success('已复制到剪贴板')
    }
  }

  return (
    <div>
      <Card title="划词解释" style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <div>
            <Text strong>需要解释的内容</Text>
            <TextArea
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="输入 K8S 概念、命令、YAML 配置或错误信息..."
              autoSize={{ minRows: 4, maxRows: 8 }}
              style={{ marginTop: 8 }}
            />
          </div>
          <div>
            <Text strong>上下文信息（可选）</Text>
            <Input
              value={context}
              onChange={(e) => setContext(e.target.value)}
              placeholder="提供额外上下文帮助解释..."
              style={{ marginTop: 8 }}
            />
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Select
              value={clusterId}
              onChange={setClusterId}
              style={{ width: 200 }}
              placeholder="选择集群（可选）"
              options={[
                { label: '不指定集群', value: 0 },
                ...clusters.map((c) => ({
                  label: c.display_name || c.name,
                  value: c.id,
                })),
              ]}
            />
            <Button
              type="primary"
              icon={<SearchOutlined />}
              onClick={handleExplain}
              loading={loading}
            >
              解释
            </Button>
          </div>
        </Space>
      </Card>

      {result && (
        <Card
          title="解释结果"
          extra={
            <Tooltip title="复制内容">
              <Button icon={<CopyOutlined />} onClick={handleCopy} />
            </Tooltip>
          }
        >
          <div className="markdown-body">
            <MarkdownRenderer content={result.explanation} />
          </div>
        </Card>
      )}
    </div>
  )
}

// 资源指南组件
const ResourceGuideTab: React.FC<{ clusters: Cluster[] }> = ({ clusters }) => {
  const [clusterId, setClusterId] = useState<number>(0)
  const [resourceType, setResourceType] = useState('pod')
  const [resourceName, setResourceName] = useState('')
  const [namespace, setNamespace] = useState('')
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [resourceNames, setResourceNames] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ResourceGuideResponse | null>(null)

  const resourceTypes = [
    { label: 'Pod', value: 'pod' },
    { label: 'Deployment', value: 'deployment' },
    { label: 'Service', value: 'service' },
    { label: 'Node', value: 'node' },
    { label: 'ConfigMap', value: 'configmap' },
    { label: 'Secret', value: 'secret' },
    { label: 'Ingress', value: 'ingress' },
  ]

  useEffect(() => {
    if (clusterId) {
      fetchNamespaces()
    }
  }, [clusterId])

  useEffect(() => {
    if (clusterId && resourceType) {
      fetchResourceNames()
    }
  }, [clusterId, resourceType, namespace])

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(clusterId)
      setNamespaces(res.data || [])
    } catch (e) { console.error(e) }
  }

  const fetchResourceNames = async () => {
    try {
      let names: string[] = []
      switch (resourceType) {
        case 'pod':
          const podsRes = await getPods(clusterId, namespace || undefined)
          names = (podsRes.data || []).map((p: any) => p.name)
          break
        case 'deployment':
          const deploysRes = await getDeployments(clusterId, namespace || undefined)
          names = (deploysRes.data || []).map((d: any) => d.name)
          break
        case 'service':
          const svcsRes = await getServices(clusterId, namespace || undefined)
          names = (svcsRes.data || []).map((s: any) => s.name)
          break
        case 'node':
          const nodesRes = await getNodes(clusterId)
          names = (nodesRes.data || []).map((n: any) => n.name)
          break
      }
      setResourceNames(names)
    } catch (e) { console.error(e) }
  }

  const handleAnalyze = async () => {
    if (!clusterId) {
      message.warning('请选择集群')
      return
    }
    setLoading(true)
    try {
      const res = await getResourceGuide({
        cluster_id: clusterId,
        resource_type: resourceType,
        resource_name: resourceName,
        namespace: namespace,
      })
      setResult(res.data)
    } catch (error) {
      message.error('获取资源指南失败')
    } finally {
      setLoading(false)
    }
  }

  const getHealthColor = (score: number) => {
    if (score >= 80) return '#52c41a'
    if (score >= 60) return '#faad14'
    if (score >= 40) return '#ff7a45'
    return '#ff4d4f'
  }

  return (
    <div>
      <Card title="资源指南" style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Row gutter={16}>
            <Col span={8}>
              <Text strong>集群</Text>
              <Select
                value={clusterId}
                onChange={setClusterId}
                style={{ width: '100%', marginTop: 8 }}
                placeholder="选择集群"
                options={clusters.map((c) => ({
                  label: c.display_name || c.name,
                  value: c.id,
                }))}
              />
            </Col>
            <Col span={8}>
              <Text strong>资源类型</Text>
              <Select
                value={resourceType}
                onChange={setResourceType}
                style={{ width: '100%', marginTop: 8 }}
                options={resourceTypes}
              />
            </Col>
            <Col span={8}>
              <Text strong>命名空间</Text>
              <Select
                value={namespace}
                onChange={setNamespace}
                style={{ width: '100%', marginTop: 8 }}
                placeholder="选择或输入命名空间"
                showSearch
                allowClear
                options={namespaces.map(ns => ({ label: ns, value: ns }))}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={16}>
              <Text strong>资源名称（可选，留空分析所有）</Text>
              <Select
                value={resourceName}
                onChange={setResourceName}
                style={{ width: '100%', marginTop: 8 }}
                placeholder="选择或输入资源名称"
                showSearch
                allowClear
                options={resourceNames.map(name => ({ label: name, value: name }))}
              />
            </Col>
            <Col span={8} style={{ display: 'flex', alignItems: 'flex-end' }}>
              <Button
                type="primary"
                icon={<BookOutlined />}
                onClick={handleAnalyze}
                loading={loading}
                block
                style={{ marginTop: 8 }}
              >
                分析
              </Button>
            </Col>
          </Row>
        </Space>
      </Card>

      {result && (
        <>
          <Card style={{ marginBottom: 16 }}>
            <Row gutter={16} align="middle">
              <Col span={18}>
                <Title level={4} style={{ margin: 0 }}>
                  资源分析结果
                </Title>
              </Col>
              <Col span={6} style={{ textAlign: 'right' }}>
                <Progress
                  type="circle"
                  percent={result.health_score}
                  size={80}
                  strokeColor={getHealthColor(result.health_score)}
                  format={(percent) => `${percent}分`}
                />
              </Col>
            </Row>
          </Card>

          <Row gutter={16}>
            <Col span={12}>
              <Card title="概述">
                <Paragraph>{result.overview}</Paragraph>
              </Card>
            </Col>
            <Col span={12}>
              <Card title="状态分析">
                <Paragraph>{result.status}</Paragraph>
              </Card>
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col span={8}>
              <Card title={<><ThunderboltOutlined /> 优化建议</>} size="small">
                <List
                  size="small"
                  dataSource={result.suggestions}
                  renderItem={(item) => <List.Item>{item}</List.Item>}
                />
              </Card>
            </Col>
            <Col span={8}>
              <Card title={<><FileTextOutlined /> 常用操作</>} size="small">
                <List
                  size="small"
                  dataSource={result.operations}
                  renderItem={(item) => (
                    <List.Item>
                      <Text code>{item}</Text>
                    </List.Item>
                  )}
                />
              </Card>
            </Col>
            <Col span={8}>
              <Card title={<><WarningOutlined /> 潜在风险</>} size="small">
                <List
                  size="small"
                  dataSource={result.warnings}
                  renderItem={(item) => (
                    <List.Item>
                      <Space>
                        <SafetyOutlined style={{ color: '#faad14' }} />
                        {item}
                      </Space>
                    </List.Item>
                  )}
                />
              </Card>
            </Col>
          </Row>
        </>
      )}
    </div>
  )
}

// YAML 翻译组件
const TranslateYAMLTab: React.FC = () => {
  const [yaml, setYaml] = useState('')
  const [direction, setDirection] = useState('to_chinese')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<TranslateYAMLResponse | null>(null)

  const handleTranslate = async () => {
    if (!yaml.trim()) {
      message.warning('请输入 YAML 内容')
      return
    }
    setLoading(true)
    try {
      const res = await translateYAML({
        yaml: yaml.trim(),
        direction: direction,
      })
      setResult(res.data)
    } catch (error) {
      message.error('翻译失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCopy = () => {
    if (result?.translated) {
      navigator.clipboard.writeText(result.translated)
      message.success('已复制到剪贴板')
    }
  }

  const sampleYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"`

  return (
    <div>
      <Card title="YAML 翻译" style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
              <Text strong>YAML 内容</Text>
              <Button
                size="small"
                onClick={() => setYaml(sampleYAML)}
              >
                加载示例
              </Button>
            </div>
            <TextArea
              value={yaml}
              onChange={(e) => setYaml(e.target.value)}
              placeholder="粘贴 YAML 配置..."
              autoSize={{ minRows: 10, maxRows: 20 }}
              style={{ fontFamily: 'monospace' }}
            />
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Select
              value={direction}
              onChange={setDirection}
              style={{ width: 200 }}
              options={[
                { label: '翻译成中文（添加注释）', value: 'to_chinese' },
                { label: '翻译成英文', value: 'to_english' },
              ]}
            />
            <Button
              type="primary"
              icon={<TranslationOutlined />}
              onClick={handleTranslate}
              loading={loading}
            >
              翻译
            </Button>
          </div>
        </Space>
      </Card>

      {result && (
        <Card
          title="翻译结果"
          extra={
            <Tooltip title="复制内容">
              <Button icon={<CopyOutlined />} onClick={handleCopy} />
            </Tooltip>
          }
        >
          <div className="markdown-body">
            <MarkdownRenderer content={result.translated} />
          </div>
          {result.notes && (
            <>
              <Divider />
              <Alert message="翻译说明" description={result.notes} type="info" />
            </>
          )}
        </Card>
      )}
    </div>
  )
}

// 主页面
const AITools: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])

  useEffect(() => {
    fetchClusters()
  }, [])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const tabItems = [
    {
      key: 'explain',
      label: (
        <span>
          <SearchOutlined />
          划词解释
        </span>
      ),
      children: <ExplainTab clusters={clusters} />,
    },
    {
      key: 'resource-guide',
      label: (
        <span>
          <BookOutlined />
          资源指南
        </span>
      ),
      children: <ResourceGuideTab clusters={clusters} />,
    },
    {
      key: 'translate-yaml',
      label: (
        <span>
          <TranslationOutlined />
          YAML 翻译
        </span>
      ),
      children: <TranslateYAMLTab />,
    },
  ]

  return (
    <div style={{ padding: 24 }}>
      <Title level={3} style={{ marginBottom: 24 }}>
        AI 智能工具
      </Title>
      <Tabs defaultActiveKey="explain" items={tabItems} size="large" />
    </div>
  )
}

export default AITools
