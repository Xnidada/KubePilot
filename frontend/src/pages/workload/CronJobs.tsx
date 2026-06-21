import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, message, Popconfirm,
  Modal, Form, InputNumber, Divider, Row, Col, Switch
} from 'antd'
import {
  SyncOutlined, DeleteOutlined, SearchOutlined, PlusOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { get, del } from '../../api/request'

const { Title } = Typography

interface CronJob {
  name: string
  namespace: string
  schedule: string
  suspend: boolean
  active: number
  last_schedule: string
  age: string
  images: string[]
}

const CronJobManagement: React.FC = () => {
  const [cronJobs, setCronJobs] = useState<CronJob[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
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
      const res = await get<{ code: number; data: CronJob[] }>(`/clusters/${selectedCluster}/workloads/cronjobs${params}`)
      setCronJobs(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleCreate = async (values: any) => {
    try {
      const yaml = `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: ${values.name}
  namespace: ${values.namespace}
spec:
  schedule: "${values.schedule}"
  suspend: ${values.suspend || false}
  successfulJobsHistoryLimit: ${values.successfulHistory || 3}
  failedJobsHistoryLimit: ${values.failedHistory || 1}
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: ${values.name}
            image: ${values.image}
            command: ${JSON.stringify(values.command ? values.command.split(' ') : ['echo', 'hello'])}
            resources:
              requests:
                memory: "${values.memoryRequest || '128Mi'}"
                cpu: "${values.cpuRequest || '100m'}"
              limits:
                memory: "${values.memoryLimit || '256Mi'}"
                cpu: "${values.cpuLimit || '200m'}"
          restartPolicy: OnFailure
`
      const token = getAuthToken()
      const response = await fetch('/api/v1/aiops/kubectl', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ cluster_id: selectedCluster, command: 'apply', yaml }),
      })
      const res = await response.json()
      if (res.code === 0 && res.data?.success) {
        message.success('CronJob 创建成功')
        setCreateModalVisible(false)
        form.resetFields()
        fetchData()
      } else {
        message.error(res.data?.error || '创建失败')
      }
    } catch (e) { message.error('创建失败') }
  }

  const handleDelete = async (record: CronJob) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/cronjobs/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchData()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<CronJob> = [
    { title: '名称', dataIndex: 'name', key: 'name', filteredValue: searchText ? [searchText] : null, onFilter: (v, r) => r.name.includes(v as string) },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: 'Schedule', dataIndex: 'schedule', key: 'schedule', render: (s) => <Tag>{s}</Tag> },
    { title: 'Suspend', dataIndex: 'suspend', key: 'suspend', render: (v) => v ? <Tag color="warning">暂停</Tag> : <Tag color="success">运行中</Tag> },
    { title: 'Active', dataIndex: 'active', key: 'active' },
    { title: 'Last Schedule', dataIndex: 'last_schedule', key: 'last_schedule', render: (v) => v || '-' },
    { title: '镜像', dataIndex: 'images', key: 'images', render: (imgs: string[]) => imgs?.map(i => <Tag key={i}>{i}</Tag>) },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 100,
      render: (_, record) => (
        <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record)}>
          <Button type="link" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>CronJob</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchData}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={cronJobs} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal title="创建 CronJob" open={createModalVisible} onCancel={() => { setCreateModalVisible(false); form.resetFields() }} onOk={() => form.submit()} width={700}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
                <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true }]}>
                <Input placeholder="例如: daily-backup" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="schedule" label="调度规则" rules={[{ required: true }]} help="Cron 表达式，例如: 0 2 * * * (每天凌晨2点)">
                <Input placeholder="0 2 * * *" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
                <Input placeholder="例如: busybox:latest" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="command" label="命令" help="空格分隔的命令参数">
            <Input placeholder="例如: /bin/sh -c echo hello" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="suspend" label="暂停" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="successfulHistory" label="成功历史数" initialValue={3}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="failedHistory" label="失败历史数" initialValue={1}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
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
    </div>
  )
}

function getAuthToken(): string {
  const token = localStorage.getItem("auth-storage")
  if (token) {
    try {
      const authData = JSON.parse(token)
      return authData?.state?.token || ""
    } catch { return "" }
  }
  return ""
}
export default CronJobManagement
