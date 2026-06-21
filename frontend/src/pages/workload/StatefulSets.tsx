import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, message, Popconfirm,
  Modal, Form, InputNumber, Divider, Row, Col, Tooltip
} from 'antd'
import {
  SyncOutlined, DeleteOutlined, SearchOutlined, PlusOutlined, EyeOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import StatusTag from '../../components/StatusTag'
import { get, del } from '../../api/request'

const { Title } = Typography

interface StatefulSet {
  name: string
  namespace: string
  status: string
  ready: string
  replicas: number
  age: string
  images: string[]
}

const StatefulSetManagement: React.FC = () => {
  const [statefulSets, setStatefulSets] = useState<StatefulSet[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedSTS, setSelectedSTS] = useState<any>(null)
  const [form] = Form.useForm()

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

  const fetchData = async () => {
    setLoading(true)
    try {
      const params = selectedNamespace ? `?ns=${selectedNamespace}` : ''
      const res = await get<{ code: number; data: StatefulSet[] }>(`/clusters/${selectedCluster}/workloads/statefulsets${params}`)
      setStatefulSets(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleCreate = async (values: any) => {
    try {
      const yaml = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: ${values.name}
  namespace: ${values.namespace}
spec:
  serviceName: ${values.serviceName || values.name}
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
            memory: "${values.memoryRequest || '128Mi'}"
            cpu: "${values.cpuRequest || '100m'}"
          limits:
            memory: "${values.memoryLimit || '256Mi'}"
            cpu: "${values.cpuLimit || '200m'}"
`
      const token = getAuthToken()
      const response = await fetch('/api/v1/aiops/kubectl', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ cluster_id: selectedCluster, command: 'apply', yaml }),
      })
      const res = await response.json()
      if (res.code === 0 && res.data?.success) {
        message.success('StatefulSet 创建成功')
        setCreateModalVisible(false)
        form.resetFields()
        fetchData()
      } else {
        message.error(res.data?.error || '创建失败')
      }
    } catch (e) { message.error('创建失败') }
  }

  const handleDelete = async (record: StatefulSet) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/statefulsets/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchData()
    } catch (e) { console.error(e) }
  }

  const handleViewDetail = (record: StatefulSet) => {
    setSelectedSTS(record)
    setDetailModalVisible(true)
  }

  const columns: ColumnsType<StatefulSet> = [
    { title: '名称', dataIndex: 'name', key: 'name', filteredValue: searchText ? [searchText] : null, onFilter: (v, r) => r.name.includes(v as string) },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s) => <StatusTag status={s} /> },
    { title: '就绪', dataIndex: 'ready', key: 'ready' },
    { title: '副本数', dataIndex: 'replicas', key: 'replicas' },
    { title: '镜像', dataIndex: 'images', key: 'images', render: (imgs: string[]) => imgs?.map(i => <Tag key={i}>{i}</Tag>) },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 150,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
          </Tooltip>
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>StatefulSet</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchData}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={statefulSets} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal title="创建 StatefulSet" open={createModalVisible} onCancel={() => { setCreateModalVisible(false); form.resetFields() }} onOk={() => form.submit()} width={700}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
                <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true }]}>
                <Input placeholder="例如: mysql" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
                <Input placeholder="例如: mysql:8.0" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="replicas" label="副本数" initialValue={1}>
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="port" label="端口" initialValue={3306}>
                <InputNumber min={1} max={65535} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="serviceName" label="关联 Service 名称">
            <Input placeholder="留空则使用名称" />
          </Form.Item>
          <Divider>资源配额</Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item name="cpuRequest" label="CPU 请求" initialValue="100m">
                <Input placeholder="100m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="cpuLimit" label="CPU 限制" initialValue="200m">
                <Input placeholder="200m" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memoryRequest" label="内存请求" initialValue="128Mi">
                <Input placeholder="128Mi" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="memoryLimit" label="内存限制" initialValue="256Mi">
                <Input placeholder="256Mi" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* 详情 Modal */}
      <Modal title={`StatefulSet: ${selectedSTS?.name}`} open={detailModalVisible} onCancel={() => setDetailModalVisible(false)} footer={null} width={600}>
        {selectedSTS && (
          <div>
            <p><strong>命名空间:</strong> {selectedSTS.namespace}</p>
            <p><strong>状态:</strong> <StatusTag status={selectedSTS.status} /></p>
            <p><strong>就绪:</strong> {selectedSTS.ready}</p>
            <p><strong>副本数:</strong> {selectedSTS.replicas}</p>
            <p><strong>镜像:</strong> {selectedSTS.images?.join(', ')}</p>
          </div>
        )}
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

export default StatefulSetManagement
