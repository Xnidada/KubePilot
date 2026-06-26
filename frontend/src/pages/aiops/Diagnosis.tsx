import { useState, useEffect } from 'react'
import {
  Card,
  Form,
  Input,
  Select,
  Button,
  Space,
  Typography,
  message,
  Spin,
  Divider,
  List,
  Tag,
  Segmented,
  Row,
  Col,
  Badge,
  Alert,
} from 'antd'
import {
  SearchOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  CodeOutlined,
  BugOutlined,
  FileTextOutlined,
  SafetyOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons'
import { diagnoseResource, DiagnosisResponse, analyzeLogs, AnalyzeLogsResponse } from '../../api/aiops'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getPods, getDeployments, getServices, getNodes } from '../../api/workload'
import MarkdownRenderer from '../../components/MarkdownRenderer'

const { Title, Text } = Typography
const { TextArea } = Input

const resourceTypes = [
  { label: 'Pod', value: 'pod' },
  { label: 'Deployment', value: 'deployment' },
  { label: 'Service', value: 'service' },
  { label: 'Node', value: 'node' },
  { label: 'StatefulSet', value: 'statefulset' },
  { label: 'ConfigMap', value: 'configmap' },
  { label: 'Secret', value: 'secret' },
  { label: 'Ingress', value: 'ingress' },
  { label: 'PersistentVolume', value: 'pv' },
  { label: 'PersistentVolumeClaim', value: 'pvc' },
]

const AIDiagnosis: React.FC = () => {
  const [form] = Form.useForm()
  const [logForm] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [mode, setMode] = useState<string>('problem')
  const [result, setResult] = useState<DiagnosisResponse | null>(null)
  const [logResult, setLogResult] = useState<AnalyzeLogsResponse | null>(null)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [resourceNames, setResourceNames] = useState<string[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [selectedResourceType, setSelectedResourceType] = useState<string>('pod')
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')

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

  const fetchNamespaces = async (clusterId: number) => {
    try {
      const res = await getNamespaceNames(clusterId)
      setNamespaces(res.data || [])
    } catch (error) {
      console.error('Failed to fetch namespaces:', error)
    }
  }

  const fetchResourceNames = async (clusterId: number, resourceType: string, namespace?: string) => {
    try {
      let names: string[] = []
      switch (resourceType) {
        case 'pod':
          const podsRes = await getPods(clusterId, namespace)
          names = (podsRes.data || []).map((p: any) => p.name)
          break
        case 'deployment':
          const deploysRes = await getDeployments(clusterId, namespace)
          names = (deploysRes.data || []).map((d: any) => d.name)
          break
        case 'service':
          const svcsRes = await getServices(clusterId, namespace)
          names = (svcsRes.data || []).map((s: any) => s.name)
          break
        case 'node':
          const nodesRes = await getNodes(clusterId)
          names = (nodesRes.data || []).map((n: any) => n.name)
          break
      }
      setResourceNames(names)
    } catch (error) {
      console.error('Failed to fetch resource names:', error)
    }
  }

  const handleClusterChange = (clusterId: number) => {
    setSelectedCluster(clusterId)
    fetchNamespaces(clusterId)
    fetchResourceNames(clusterId, selectedResourceType, selectedNamespace)
  }

  const handleResourceTypeChange = (resourceType: string) => {
    setSelectedResourceType(resourceType)
    if (selectedCluster) {
      fetchResourceNames(selectedCluster, resourceType, selectedNamespace)
    }
  }

  const handleNamespaceChange = (namespace: string) => {
    setSelectedNamespace(namespace)
    if (selectedCluster) {
      fetchResourceNames(selectedCluster, selectedResourceType, namespace)
    }
  }

  // 问题诊断
  const handleSubmit = async (values: any) => {
    setLoading(true)
    setResult(null)
    setLogResult(null)

    try {
      const res = await diagnoseResource({
        cluster_id: values.cluster_id,
        resource_type: values.resource_type,
        resource_name: values.resource_name,
        namespace: values.namespace,
        problem: values.problem || '', // 问题描述可选
      })

      if (res.code === 0) {
        setResult(res.data)
      } else {
        message.error('诊断失败')
      }
    } catch (error) {
      console.error('Diagnosis error:', error)
      message.error('诊断服务不可用，请检查 LLM 配置')
    } finally {
      setLoading(false)
    }
  }

  // 日志问诊
  const handleLogSubmit = async (values: any) => {
    setLoading(true)
    setResult(null)
    setLogResult(null)

    try {
      const res = await analyzeLogs({
        cluster_id: values.cluster_id,
        resource_name: values.resource_name,
        namespace: values.namespace || 'default',
        container: values.container,
        lines: values.lines || 100,
        logs: values.logs,
      })

      if (res.code === 0) {
        setLogResult(res.data)
      } else {
        message.error('分析失败')
      }
    } catch (error) {
      console.error('Log analysis error:', error)
      message.error('日志分析服务不可用，请检查 LLM 配置')
    } finally {
      setLoading(false)
    }
  }

  // 问题诊断结果
  const renderProblemResult = () => {
    if (!result) return null

    return (
      <Card title="🔍 诊断结果" style={{ marginTop: 24 }}>
        <div style={{ marginBottom: 24 }}>
          <Title level={5}>📋 原因分析</Title>
          <div style={{ background: '#f6f8fa', padding: 16, borderRadius: 8 }}>
            <MarkdownRenderer content={result.analysis} />
          </div>
        </div>

        {result.steps && result.steps.length > 0 && (
          <>
            <Divider />
            <div style={{ marginBottom: 24 }}>
              <Title level={5}>🔎 排查步骤</Title>
              <List
                dataSource={result.steps}
                renderItem={(step, index) => (
                  <List.Item>
                    <Space>
                      <Tag color="blue">{index + 1}</Tag>
                      <Text>{step}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          </>
        )}

        {result.solutions && result.solutions.length > 0 && (
          <>
            <Divider />
            <div style={{ marginBottom: 24 }}>
              <Title level={5}>✅ 解决方案</Title>
              <List
                dataSource={result.solutions}
                renderItem={(solution) => (
                  <List.Item>
                    <Space>
                      <CheckCircleOutlined style={{ color: '#52c41a' }} />
                      <Text>{solution}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          </>
        )}

        {result.commands && result.commands.length > 0 && (
          <>
            <Divider />
            <div style={{ marginBottom: 24 }}>
              <Title level={5}>💻 相关命令</Title>
              <List
                dataSource={result.commands}
                renderItem={(cmd) => (
                  <List.Item>
                    <Space>
                      <CodeOutlined />
                      <Text code copyable>{cmd}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          </>
        )}

        {result.prevention && result.prevention.length > 0 && (
          <>
            <Divider />
            <div>
              <Title level={5}>🛡️ 预防措施</Title>
              <List
                dataSource={result.prevention}
                renderItem={(item) => (
                  <List.Item>
                    <Space>
                      <ExclamationCircleOutlined style={{ color: '#faad14' }} />
                      <Text>{item}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          </>
        )}
      </Card>
    )
  }

  // 日志问诊结果
  const renderLogResult = () => {
    if (!logResult) return null

    const getSeverityColor = (severity: string) => {
      switch (severity) {
        case 'critical': return '#ff4d4f'
        case 'high': return '#ff7a45'
        case 'medium': return '#faad14'
        case 'low': return '#52c41a'
        default: return '#1890ff'
      }
    }

    const getSeverityText = (severity: string) => {
      switch (severity) {
        case 'critical': return '严重'
        case 'high': return '高'
        case 'medium': return '中'
        case 'low': return '低'
        default: return '未知'
      }
    }

    return (
      <div style={{ marginTop: 24 }}>
        <Card style={{ marginBottom: 16 }}>
          <Row gutter={16} align="middle">
            <Col span={18}>
              <Title level={4} style={{ margin: 0 }}>
                🔍 日志诊断结果
              </Title>
            </Col>
            <Col span={6} style={{ textAlign: 'right' }}>
              <Badge
                count={getSeverityText(logResult.severity)}
                style={{
                  backgroundColor: getSeverityColor(logResult.severity),
                  fontSize: 14,
                  padding: '0 12px',
                  height: 28,
                  lineHeight: '28px',
                }}
              />
            </Col>
          </Row>
        </Card>

        <Card title="📋 日志摘要" style={{ marginBottom: 16 }}>
          <Text>{logResult.summary}</Text>
        </Card>

        <Row gutter={16}>
          <Col span={12}>
            <Card title="🎯 根因分析" style={{ marginBottom: 16 }}>
              <Alert
                message={logResult.root_cause}
                type="warning"
                showIcon
              />
            </Card>
          </Col>
          <Col span={12}>
            <Card title="📊 发现的模式" style={{ marginBottom: 16 }}>
              <List
                size="small"
                dataSource={logResult.patterns}
                renderItem={(item) => (
                  <List.Item>
                    <Tag color="purple">{item}</Tag>
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={8}>
            <Card title="❌ 错误信息" size="small">
              <List
                size="small"
                dataSource={logResult.errors}
                renderItem={(item) => (
                  <List.Item>
                    <Space>
                      <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                      <Text type="danger">{item}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </Card>
          </Col>
          <Col span={8}>
            <Card title="✅ 解决方案" size="small">
              <List
                size="small"
                dataSource={logResult.solutions}
                renderItem={(item) => (
                  <List.Item>
                    <Space>
                      <SafetyOutlined style={{ color: '#52c41a' }} />
                      {item}
                    </Space>
                  </List.Item>
                )}
              />
            </Card>
          </Col>
          <Col span={8}>
            <Card title="💻 排查命令" size="small">
              <List
                size="small"
                dataSource={logResult.commands}
                renderItem={(item) => (
                  <List.Item>
                    <Text code copyable>{item}</Text>
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        </Row>
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <Title level={4} style={{ margin: 0 }}>🔍 AI 智能诊断</Title>
        <Segmented
          value={mode}
          onChange={(val) => {
            setMode(val as string)
            setResult(null)
            setLogResult(null)
          }}
          options={[
            {
              label: <span><FileTextOutlined /> 问题诊断</span>,
              value: 'problem',
            },
            {
              label: <span><BugOutlined /> 日志问诊</span>,
              value: 'logs',
            },
          ]}
        />
      </div>

      {/* 问题诊断模式 */}
      {mode === 'problem' && (
        <Card>
          <Form
            form={form}
            layout="vertical"
            onFinish={handleSubmit}
          >
            <Row gutter={16}>
              <Col span={8}>
                <Form.Item
                  name="cluster_id"
                  label="集群"
                  rules={[{ required: true, message: '请选择集群' }]}
                >
                  <Select
                    placeholder="选择集群"
                    options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
                    onChange={handleClusterChange}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="resource_type"
                  label="资源类型"
                  rules={[{ required: true, message: '请选择资源类型' }]}
                >
                  <Select
                    placeholder="选择资源类型"
                    options={resourceTypes}
                    onChange={handleResourceTypeChange}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="namespace"
                  label="命名空间"
                >
                  <Select
                    placeholder="选择命名空间（可选）"
                    allowClear
                    showSearch
                    options={namespaces.map(ns => ({ label: ns, value: ns }))}
                    onChange={handleNamespaceChange}
                  />
                </Form.Item>
              </Col>
            </Row>

            <Form.Item
              name="resource_name"
              label="资源名称"
              rules={[{ required: true, message: '请输入资源名称' }]}
            >
              <Select
                placeholder="输入或选择资源名称"
                showSearch
                allowClear
                options={resourceNames.map(name => ({ label: name, value: name }))}
                dropdownRender={(menu) => menu}
                mode={undefined}
                filterOption={(input, option) =>
                  (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
                }
              />
            </Form.Item>

            <Alert
              message="提示"
              description="问题描述为可选项。如果不填写，AI 将自动获取资源的 describe 信息进行全面分析。"
              type="info"
              showIcon
              icon={<InfoCircleOutlined />}
              style={{ marginBottom: 16 }}
            />

            <Form.Item
              name="problem"
              label="问题描述（可选）"
            >
              <TextArea
                rows={4}
                placeholder="请详细描述遇到的问题，例如：&#10;- Pod 一直 CrashLoopBackOff&#10;- Deployment 部署失败&#10;- 节点 NotReady&#10;&#10;留空则 AI 自动分析资源状态"
              />
            </Form.Item>

            <Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                icon={<SearchOutlined />}
                loading={loading}
                size="large"
              >
                开始诊断
              </Button>
            </Form.Item>
          </Form>
        </Card>
      )}

      {/* 日志问诊模式 */}
      {mode === 'logs' && (
        <Card>
          <Form
            form={logForm}
            layout="vertical"
            onFinish={handleLogSubmit}
          >
            <Row gutter={16}>
              <Col span={8}>
                <Form.Item
                  name="cluster_id"
                  label="集群"
                  rules={[{ required: true, message: '请选择集群' }]}
                >
                  <Select
                    placeholder="选择集群"
                    options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
                    onChange={handleClusterChange}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="resource_name"
                  label="Pod 名称"
                  rules={[{ required: true, message: '请输入 Pod 名称' }]}
                >
                  <Select
                    placeholder="输入或选择 Pod 名称"
                    showSearch
                    options={resourceNames.map(name => ({ label: name, value: name }))}
                    filterOption={(input, option) =>
                      (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
                    }
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="namespace"
                  label="命名空间"
                  initialValue="default"
                >
                  <Select
                    placeholder="选择命名空间"
                    allowClear
                    options={namespaces.map(ns => ({ label: ns, value: ns }))}
                  />
                </Form.Item>
              </Col>
            </Row>

            <Row gutter={16}>
              <Col span={8}>
                <Form.Item
                  name="container"
                  label="容器名称（可选）"
                >
                  <Input placeholder="留空使用第一个容器" />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="lines"
                  label="日志行数"
                  initialValue={100}
                >
                  <Select
                    options={[
                      { label: '50 行', value: 50 },
                      { label: '100 行', value: 100 },
                      { label: '200 行', value: 200 },
                      { label: '500 行', value: 500 },
                    ]}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item label=" " name="auto_fetch">
                  <Text type="secondary">留空下方日志内容将自动从集群获取</Text>
                </Form.Item>
              </Col>
            </Row>

            <Form.Item
              name="logs"
              label="日志内容（可选，留空自动获取）"
            >
              <TextArea
                rows={6}
                placeholder="粘贴日志内容，或留空自动从集群获取..."
                style={{ fontFamily: 'monospace' }}
              />
            </Form.Item>

            <Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                icon={<BugOutlined />}
                loading={loading}
                size="large"
              >
                开始分析
              </Button>
            </Form.Item>
          </Form>
        </Card>
      )}

      {loading && (
        <Card style={{ marginTop: 24, textAlign: 'center' }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>
            <Text type="secondary">
              {mode === 'problem' ? 'AI 正在分析问题，请稍候...' : 'AI 正在分析日志，请稍候...'}
            </Text>
          </div>
        </Card>
      )}

      {mode === 'problem' && renderProblemResult()}
      {mode === 'logs' && renderLogResult()}
    </div>
  )
}

export default AIDiagnosis
