import { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Typography,
  Select,
  Input,
  Tooltip,
  Modal,
  Form,
  message,
  Popconfirm,
  InputNumber,
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  SearchOutlined,
  MinusCircleOutlined,
  LinkOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getIngresses, createIngress, deleteIngress, Ingress } from '../../api/resources'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getServices } from '../../api/workload'

const { Title, Text } = Typography

const IngressManagement: React.FC = () => {
  const [ingresses, setIngresses] = useState<Ingress[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [services, setServices] = useState<any[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchIngresses()
    }
  }, [selectedCluster, selectedNamespace])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data && res.data.length > 0) {
        setSelectedCluster(res.data[0].id)
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(selectedCluster)
      setNamespaces(res.data || [])
    } catch (error) {
      console.error('Failed to fetch namespaces:', error)
    }
  }

  const fetchServices = async (namespace: string) => {
    try {
      const res = await getServices(selectedCluster, namespace)
      setServices(res.data || [])
    } catch (error) {
      console.error('Failed to fetch services:', error)
    }
  }

  const fetchIngresses = async () => {
    setLoading(true)
    try {
      const res = await getIngresses(selectedCluster, selectedNamespace || undefined)
      setIngresses(res.data || [])
    } catch (error) {
      console.error('Failed to fetch ingresses:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (values: any) => {
    try {
      await createIngress(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        class_name: values.class_name,
        host: values.host,
        paths: values.paths || [],
        tls_secret: values.tls_secret,
      })
      message.success('Ingress 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchIngresses()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const handleDelete = async (record: Ingress) => {
    try {
      await deleteIngress(selectedCluster, record.namespace, record.name)
      message.success('Ingress 已删除')
      fetchIngresses()
    } catch (error) {
      console.error('Delete failed:', error)
    }
  }

  const columns: ColumnsType<Ingress> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      filteredValue: searchText ? [searchText] : null,
      onFilter: (value, record) => record.name.includes(value as string),
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
    },
    {
      title: 'Class',
      dataIndex: 'class_name',
      key: 'class_name',
      render: (v) => v || '-',
    },
    {
      title: 'Hosts',
      dataIndex: 'hosts',
      key: 'hosts',
      render: (hosts: string[]) => (
        <Space size={[0, 4]} wrap>
          {hosts.map(h => (
            <Tag key={h} icon={<LinkOutlined />}>{h}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: 'Address',
      dataIndex: 'address',
      key: 'address',
      render: (v) => v || '-',
    },
    {
      title: 'TLS',
      dataIndex: 'tls',
      key: 'tls',
      render: (tls) => tls ? <Tag color="green">是</Tag> : <Tag>否</Tag>,
    },
    {
      title: '年龄',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_, record) => (
        <Space size="small">
          <Popconfirm title="确定删除吗？" onConfirm={() => handleDelete(record)}>
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
        <Title level={4}>Ingress</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Select
            value={selectedNamespace}
            onChange={setSelectedNamespace}
            style={{ width: 150 }}
            placeholder="所有命名空间"
            allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))}
          />
          <Input
            placeholder="搜索..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
          />
          <Button icon={<SyncOutlined />} onClick={fetchIngresses}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={ingresses} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal
        title="创建 Ingress"
        open={createModalVisible}
        onCancel={() => { setCreateModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={700}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
            <Select
              options={namespaces.map(ns => ({ label: ns, value: ns }))}
              onChange={(ns) => fetchServices(ns)}
            />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="ingress 名称" />
          </Form.Item>
          <Form.Item name="class_name" label="Ingress Class">
            <Input placeholder="例如: nginx" />
          </Form.Item>
          <Form.Item name="host" label="域名" rules={[{ required: true }]}>
            <Input placeholder="例如: example.com" />
          </Form.Item>

          <Form.List name="paths">
            {(fields, { add, remove }) => (
              <>
                <div style={{ marginBottom: 8 }}>
                  <Text strong>路径规则</Text>
                  <Button type="link" onClick={() => add()} icon={<PlusOutlined />}>添加</Button>
                </div>
                {fields.map(({ key, name, ...restField }) => (
                  <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                    <Form.Item {...restField} name={[name, 'path']} style={{ marginBottom: 0 }}>
                      <Input placeholder="/path" style={{ width: 120 }} />
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'path_type']} initialValue="Prefix" style={{ marginBottom: 0 }}>
                      <Select style={{ width: 120 }}>
                        <Select.Option value="Prefix">Prefix</Select.Option>
                        <Select.Option value="Exact">Exact</Select.Option>
                        <Select.Option value="ImplementationSpecific">ImplementationSpecific</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'service']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                      <Select
                        placeholder="选择 Service"
                        style={{ width: 200 }}
                        options={services.map(s => ({ label: s.name, value: s.name }))}
                        showSearch
                      />
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'port']} rules={[{ required: true }]} initialValue={80} style={{ marginBottom: 0 }}>
                      <InputNumber placeholder="端口" min={1} max={65535} style={{ width: 80 }} />
                    </Form.Item>
                    <MinusCircleOutlined onClick={() => remove(name)} />
                  </Space>
                ))}
              </>
            )}
          </Form.List>

          <Form.Item name="tls_secret" label="TLS Secret (可选)" style={{ marginTop: 16 }}>
            <Input placeholder="TLS Secret 名称" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default IngressManagement
