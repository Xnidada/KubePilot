import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, message, Popconfirm, Modal, Form, Input } from 'antd'
import { PlusOutlined, SyncOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { get, post, del } from '../../api/request'

const { Title } = Typography

interface NetworkPolicy {
  name: string
  namespace: string
  pods: string[]
  policy_types: string[]
  age: string
}

const NetworkPolicies: React.FC = () => {
  const [policies, setPolicies] = useState<NetworkPolicy[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchPolicies() } }, [selectedCluster, selectedNamespace])

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

  const fetchPolicies = async () => {
    setLoading(true)
    try {
      const params = selectedNamespace ? `?ns=${selectedNamespace}` : ''
      const res = await get<{ code: number; data: NetworkPolicy[] }>(`/clusters/${selectedCluster}/workloads/networkpolicies${params}`)
      setPolicies(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleCreate = async (values: any) => {
    try {
      await post(`/clusters/${selectedCluster}/workloads/networkpolicies`, {
        name: values.name,
        namespace: values.namespace,
        pod_selector: { app: values.app_label },
        policy_types: values.policy_types,
      })
      message.success('NetworkPolicy 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchPolicies()
    } catch (e) { message.error('创建失败') }
  }

  const handleDelete = async (record: NetworkPolicy) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/networkpolicies/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchPolicies()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<NetworkPolicy> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: 'Pod 选择器', key: 'pods',
      render: (_, r) => r.pods?.map(p => <Tag key={p}>{p}</Tag>) || '-'
    },
    {
      title: '策略类型', key: 'types',
      render: (_, r) => r.policy_types?.map(t => <Tag color="blue" key={t}>{t}</Tag>)
    },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 80,
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
        <Title level={4}>NetworkPolicy</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="所有命名空间" allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Button icon={<SyncOutlined />} onClick={fetchPolicies}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); setCreateModalVisible(true) }}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={policies} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      <Modal title="创建 NetworkPolicy" open={createModalVisible}
        onCancel={() => { setCreateModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()} width={600}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="NetworkPolicy 名称" />
          </Form.Item>
          <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
            <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          </Form.Item>
          <Form.Item name="app_label" label="Pod 标签 (app)" rules={[{ required: true }]}>
            <Input placeholder="例如: nginx" />
          </Form.Item>
          <Form.Item name="policy_types" label="策略类型" rules={[{ required: true }]}>
            <Select mode="multiple" options={[
              { label: 'Ingress', value: 'Ingress' },
              { label: 'Egress', value: 'Egress' },
            ]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default NetworkPolicies
