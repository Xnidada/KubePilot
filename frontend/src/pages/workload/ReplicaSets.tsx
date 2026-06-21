import { useEffect, useState, useCallback } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, message, Popconfirm,
  Modal, Descriptions, Form, InputNumber, Tooltip, Row, Col, Divider
} from 'antd'
import {
  SyncOutlined, DeleteOutlined, SearchOutlined, EyeOutlined, ColumnHeightOutlined, PlusOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import StatusTag from '../../components/StatusTag'
import { get, post, del } from '../../api/request'
import { usePolling, hasTerminatingResource } from '../../hooks/usePolling'

const { Title } = Typography

interface ReplicaSet {
  name: string
  namespace: string
  status: string
  ready: string
  replicas: number
  age: string
  images: string[]
  owner: string
}

interface ReplicaSetDetail {
  name: string
  namespace: string
  labels: Record<string, string>
  selector: Record<string, string>
  replicas: number
  ready: number
  available: number
  pods: { name: string; status: string; ip: string; node: string }[]
  created_at: string
}

const ReplicaSetManagement: React.FC = () => {
  const [replicaSets, setReplicaSets] = useState<ReplicaSet[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [detailVisible, setDetailVisible] = useState(false)
  const [scaleVisible, setScaleVisible] = useState(false)
  const [createVisible, setCreateVisible] = useState(false)
  const [selectedRS, setSelectedRS] = useState<ReplicaSetDetail | null>(null)
  const [form] = Form.useForm()
  const [createForm] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchData() } }, [selectedCluster, selectedNamespace])

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

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const params = selectedNamespace ? `?ns=${selectedNamespace}` : ''
      const res = await get<{ code: number; data: ReplicaSet[] }>(`/clusters/${selectedCluster}/workloads/replicasets${params}`)
      setReplicaSets(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }, [selectedCluster, selectedNamespace])

  usePolling(fetchData, hasTerminatingResource(replicaSets), { interval: 3000 })

  const handleViewDetail = async (record: ReplicaSet) => {
    try {
      const res = await get<{ code: number; data: ReplicaSetDetail }>(
        `/clusters/${selectedCluster}/workloads/replicasets/${record.namespace}/${record.name}`
      )
      setSelectedRS(res.data)
      setDetailVisible(true)
    } catch (e) {
      message.error('获取详情失败')
    }
  }

  const handleScale = (record: ReplicaSet) => {
    setSelectedRS({
      name: record.name,
      namespace: record.namespace,
      replicas: record.replicas,
    } as ReplicaSetDetail)
    form.setFieldsValue({ replicas: record.replicas })
    setScaleVisible(true)
  }

  const handleScaleSubmit = async (values: any) => {
    if (!selectedRS) return
    try {
      await post(`/clusters/${selectedCluster}/workloads/replicasets/${selectedRS.namespace}/${selectedRS.name}/scale`, {
        replicas: values.replicas,
      })
      message.success('伸缩成功')
      setScaleVisible(false)
      form.resetFields()
      fetchData()
    } catch (e) {
      message.error('伸缩失败')
    }
  }

  const handleCreate = async (values: any) => {
    try {
      const yaml = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: ${values.name}
  namespace: ${values.namespace}
  labels:
    app: ${values.name}
spec:
  replicas: ${values.replicas || 1}
  selector:
    matchLabels:
      app: ${values.name}
  template:
    metadata:
      labels:
        app: ${values.name}
    spec:
      containers:
      - name: ${values.name}
        image: ${values.image}
        ports:
        - containerPort: ${values.port || 80}
        resources:
          requests:
            memory: "${values.memory_request || '128Mi'}"
            cpu: "${values.cpu_request || '100m'}"
          limits:
            memory: "${values.memory_limit || '256Mi'}"
            cpu: "${values.cpu_limit || '200m'}"
`
      const token = getAuthToken()
      const response = await fetch('/api/v1/aiops/kubectl', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ cluster_id: selectedCluster, command: 'apply', yaml }),
      })
      const res = await response.json()
      if (res.code === 0 && res.data?.success) {
        message.success('ReplicaSet 创建成功')
        setCreateVisible(false)
        createForm.resetFields()
        fetchData()
      } else {
        message.error(res.data?.error || '创建失败')
      }
    } catch (e) {
      message.error('创建失败')
    }
  }

  const handleDelete = async (record: ReplicaSet) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/replicasets/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchData()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<ReplicaSet> = [
    { title: '名称', dataIndex: 'name', key: 'name', filteredValue: searchText ? [searchText] : null, onFilter: (v, r) => r.name.includes(v as string) },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s) => <StatusTag status={s} /> },
    { title: '就绪', dataIndex: 'ready', key: 'ready' },
    { title: '副本数', dataIndex: 'replicas', key: 'replicas' },
    { title: '镜像', dataIndex: 'images', key: 'images', render: (imgs: string[]) => imgs?.map(i => <Tag key={i}>{i}</Tag>) },
    { title: 'Owner', dataIndex: 'owner', key: 'owner', render: (v) => v || '-' },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 200,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
          </Tooltip>
          <Tooltip title="伸缩">
            <Button type="link" icon={<ColumnHeightOutlined />} onClick={() => handleScale(record)} />
          </Tooltip>
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record)}>
            <Tooltip title="删除">
              <Button type="link" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>ReplicaSet</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchData}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={replicaSets} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* Create Modal */}
      <Modal
        title="创建 ReplicaSet"
        open={createVisible}
        onCancel={() => { setCreateVisible(false); createForm.resetFields() }}
        onOk={() => createForm.submit()}
        width={700}
      >
        <Form form={createForm} layout="vertical" onFinish={handleCreate}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
                <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true }]}>
                <Input placeholder="例如: my-replicaset" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
                <Input placeholder="例如: nginx:latest" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="replicas" label="副本数" initialValue={1}>
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="port" label="端口" initialValue={80}>
                <InputNumber min={1} max={65535} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Divider>资源配额</Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item name="cpu_request" label="CPU 请求" initialValue="100m">
                <Input placeholder="100m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="cpu_limit" label="CPU 限制" initialValue="200m">
                <Input placeholder="200m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memory_request" label="内存请求" initialValue="128Mi">
                <Input placeholder="128Mi" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memory_limit" label="内存限制" initialValue="256Mi">
                <Input placeholder="256Mi" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* Detail Modal */}
      <Modal
        title={`ReplicaSet: ${selectedRS?.name}`}
        open={detailVisible}
        onCancel={() => { setDetailVisible(false); setSelectedRS(null) }}
        footer={null}
        width={800}
      >
        {selectedRS && (
          <>
            <Descriptions bordered column={2} size="small">
              <Descriptions.Item label="名称">{selectedRS.name}</Descriptions.Item>
              <Descriptions.Item label="命名空间">{selectedRS.namespace}</Descriptions.Item>
              <Descriptions.Item label="副本数">{selectedRS.replicas}</Descriptions.Item>
              <Descriptions.Item label="就绪">{selectedRS.ready}</Descriptions.Item>
              <Descriptions.Item label="可用">{selectedRS.available}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{selectedRS.created_at}</Descriptions.Item>
              <Descriptions.Item label="标签" span={2}>
                <Space size={[0, 4]} wrap>
                  {Object.entries(selectedRS.labels || {}).map(([k, v]) => (
                    <Tag key={k}>{k}={v}</Tag>
                  ))}
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="选择器" span={2}>
                <Space size={[0, 4]} wrap>
                  {Object.entries(selectedRS.selector || {}).map(([k, v]) => (
                    <Tag key={k} color="blue">{k}={v}</Tag>
                  ))}
                </Space>
              </Descriptions.Item>
            </Descriptions>

            <Title level={5} style={{ marginTop: 16, marginBottom: 8 }}>关联 Pod</Title>
            <Table
              size="small"
              pagination={false}
              dataSource={selectedRS.pods || []}
              rowKey="name"
              columns={[
                { title: '名称', dataIndex: 'name', key: 'name' },
                { title: '状态', dataIndex: 'status', key: 'status', render: (s) => <StatusTag status={s} /> },
                { title: 'IP', dataIndex: 'ip', key: 'ip' },
                { title: '节点', dataIndex: 'node', key: 'node' },
              ]}
            />
          </>
        )}
      </Modal>

      {/* Scale Modal */}
      <Modal
        title={`伸缩 ReplicaSet: ${selectedRS?.name}`}
        open={scaleVisible}
        onCancel={() => { setScaleVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={handleScaleSubmit}>
          <Form.Item name="replicas" label="副本数" rules={[{ required: true }]}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

function getAuthToken(): string {
  const token = localStorage.getItem('auth-storage')
  if (token) {
    try {
      const authData = JSON.parse(token)
      return authData?.state?.token || ''
    } catch { return '' }
  }
  return ''
}

export default ReplicaSetManagement
