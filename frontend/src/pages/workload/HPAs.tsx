import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, message, Popconfirm,
  Modal, Form, Input, InputNumber
} from 'antd'
import {
  SyncOutlined, DeleteOutlined, PlusOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { getHPAs, createHPA, deleteHPA, HPA } from '../../api/workload'

const { Title, Text } = Typography

const HPAManagement: React.FC = () => {
  const [hpas, setHpas] = useState<HPA[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchHPAs() } }, [selectedCluster, selectedNamespace])

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

  const fetchHPAs = async () => {
    setLoading(true)
    try {
      const res = await getHPAs(selectedCluster, selectedNamespace || undefined)
      setHpas(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleCreate = async (values: any) => {
    try {
      await createHPA(selectedCluster, values)
      message.success('HPA 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchHPAs()
    } catch (e) { message.error('创建失败') }
  }

  const handleDelete = async (record: HPA) => {
    try {
      await deleteHPA(selectedCluster, record.namespace, record.name)
      message.success('HPA 删除成功')
      fetchHPAs()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<HPA> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: '目标', dataIndex: 'scale_target_ref', key: 'target' },
    {
      title: '副本范围', key: 'replicas',
      render: (_, r) => <Text>{r.min_replicas || 1} ~ {r.max_replicas}</Text>
    },
    {
      title: 'CPU', key: 'cpu',
      render: (_, r) => (
        <Space>
          <Text type="secondary">当前: {r.current_cpu || '-'}</Text>
          <Text>目标: {r.target_cpu || '-'}</Text>
        </Space>
      )
    },
    {
      title: '内存', key: 'memory',
      render: (_, r) => (
        <Space>
          <Text type="secondary">当前: {r.current_memory || '-'}</Text>
          <Text>目标: {r.target_memory || '-'}</Text>
        </Space>
      )
    },
    {
      title: '当前/期望', key: 'status',
      render: (_, r) => (
        <Tag color={r.current_replicas === r.desired_replicas ? 'success' : 'processing'}>
          {r.current_replicas}/{r.desired_replicas}
        </Tag>
      )
    },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 80,
      render: (_, record) => (
        <Popconfirm title="确定删除此 HPA？" onConfirm={() => handleDelete(record)}>
          <Button type="link" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>HPA 自动伸缩</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="所有命名空间" allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Button icon={<SyncOutlined />} onClick={fetchHPAs}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); setCreateModalVisible(true) }}>
            创建 HPA
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={hpas} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      <Modal title="创建 HPA" open={createModalVisible}
        onCancel={() => { setCreateModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()} width={600}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="HPA 名称" />
          </Form.Item>
          <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
            <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          </Form.Item>
          <Form.Item name="target_kind" label="目标类型" initialValue="Deployment" rules={[{ required: true }]}>
            <Select options={[
              { label: 'Deployment', value: 'Deployment' },
              { label: 'StatefulSet', value: 'StatefulSet' },
            ]} />
          </Form.Item>
          <Form.Item name="target_name" label="目标名称" rules={[{ required: true }]}>
            <Input placeholder="Deployment/StatefulSet 名称" />
          </Form.Item>
          <Space>
            <Form.Item name="min_replicas" label="最小副本" initialValue={1}>
              <InputNumber min={1} style={{ width: 100 }} />
            </Form.Item>
            <Form.Item name="max_replicas" label="最大副本" rules={[{ required: true }]}>
              <InputNumber min={1} style={{ width: 100 }} />
            </Form.Item>
            <Form.Item name="cpu_utilization" label="CPU 目标(%)">
              <InputNumber min={1} max={100} style={{ width: 100 }} />
            </Form.Item>
            <Form.Item name="mem_utilization" label="内存目标(%)">
              <InputNumber min={1} max={100} style={{ width: 100 }} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </div>
  )
}

export default HPAManagement
