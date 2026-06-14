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
} from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  EditOutlined,
  DeleteOutlined,
  SearchOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getSecrets, getSecret, createSecret, updateSecret, deleteSecret, Secret } from '../../api/resources'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'

const { Title } = Typography
const { TextArea } = Input

const SecretManagement: React.FC = () => {
  const [secrets, setSecrets] = useState<Secret[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedSecret, setSelectedSecret] = useState<any>(null)
  const [showValues, setShowValues] = useState(false)
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchSecrets()
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

  const fetchSecrets = async () => {
    setLoading(true)
    try {
      const res = await getSecrets(selectedCluster, selectedNamespace || undefined)
      setSecrets(res.data || [])
    } catch (error) {
      console.error('Failed to fetch secrets:', error)
    } finally {
      setLoading(false)
    }
  }

  const decodeBase64 = (str: string) => {
    try {
      return atob(str)
    } catch {
      return str
    }
  }

  const encodeBase64 = (str: string) => {
    return btoa(str)
  }

  const handleCreate = async (values: any) => {
    try {
      const data: Record<string, string> = {}
      if (values.data) {
        values.data.split('\n').forEach((line: string) => {
          const [key, ...valueParts] = line.split('=')
          if (key && valueParts.length > 0) {
            data[key.trim()] = encodeBase64(valueParts.join('=').trim())
          }
        })
      }
      await createSecret(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        type: values.type || 'Opaque',
        data,
      })
      message.success('Secret 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchSecrets()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const handleEdit = async (record: Secret) => {
    try {
      const res = await getSecret(selectedCluster, record.namespace, record.name)
      setSelectedSecret(res.data)
      const dataStr = Object.entries(res.data.data || {})
        .map(([k, v]) => `${k}=${decodeBase64(v as string)}`)
        .join('\n')
      editForm.setFieldsValue({ data: dataStr })
      setEditModalVisible(true)
    } catch (error) {
      console.error('Failed to fetch secret:', error)
    }
  }

  const handleUpdate = async (values: any) => {
    if (!selectedSecret) return
    try {
      const data: Record<string, string> = {}
      if (values.data) {
        values.data.split('\n').forEach((line: string) => {
          const [key, ...valueParts] = line.split('=')
          if (key && valueParts.length > 0) {
            data[key.trim()] = encodeBase64(valueParts.join('=').trim())
          }
        })
      }
      await updateSecret(selectedCluster, selectedSecret.namespace, selectedSecret.name, { data })
      message.success('Secret 更新成功')
      setEditModalVisible(false)
      editForm.resetFields()
      fetchSecrets()
    } catch (error) {
      console.error('Update failed:', error)
    }
  }

  const handleDelete = async (record: Secret) => {
    try {
      await deleteSecret(selectedCluster, record.namespace, record.name)
      message.success('Secret 已删除')
      fetchSecrets()
    } catch (error) {
      console.error('Delete failed:', error)
    }
  }

  const handleViewDetail = async (record: Secret) => {
    try {
      const res = await getSecret(selectedCluster, record.namespace, record.name)
      setSelectedSecret(res.data)
      setShowValues(false)
      setDetailModalVisible(true)
    } catch (error) {
      console.error('Failed to fetch secret:', error)
    }
  }

  const columns: ColumnsType<Secret> = [
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
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type) => <Tag>{type}</Tag>,
    },
    {
      title: 'Keys',
      dataIndex: 'keys',
      key: 'keys',
      render: (keys: string[]) => (
        <Space size={[0, 4]} wrap>
          {keys.slice(0, 3).map(k => <Tag key={k}>{k}</Tag>)}
          {keys.length > 3 && <Tag>+{keys.length - 3}</Tag>}
        </Space>
      ),
    },
    {
      title: '数据条数',
      dataIndex: 'data_count',
      key: 'data_count',
    },
    {
      title: '年龄',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="查看详情">
            <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
          </Tooltip>
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
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
        <Title level={4}>Secret</Title>
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
          <Button icon={<SyncOutlined />} onClick={fetchSecrets}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={secrets} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal
        title="创建 Secret"
        open={createModalVisible}
        onCancel={() => { setCreateModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
            <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="secret 名称" />
          </Form.Item>
          <Form.Item name="type" label="类型" initialValue="Opaque">
            <Select
              options={[
                { label: 'Opaque', value: 'Opaque' },
                { label: 'kubernetes.io/tls', value: 'kubernetes.io/tls' },
                { label: 'kubernetes.io/dockerconfigjson', value: 'kubernetes.io/dockerconfigjson' },
                { label: 'kubernetes.io/basic-auth', value: 'kubernetes.io/basic-auth' },
              ]}
            />
          </Form.Item>
          <Form.Item name="data" label="数据 (key=value 格式，每行一条)" help="值会自动进行 Base64 编码">
            <TextArea rows={8} placeholder="username=admin&#10;password=secret123" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 编辑 Modal */}
      <Modal
        title={`编辑 Secret: ${selectedSecret?.name}`}
        open={editModalVisible}
        onCancel={() => { setEditModalVisible(false); editForm.resetFields() }}
        onOk={() => editForm.submit()}
        width={600}
      >
        <Form form={editForm} layout="vertical" onFinish={handleUpdate}>
          <Form.Item name="data" label="数据 (key=value 格式，每行一条)" help="值会自动进行 Base64 编码">
            <TextArea rows={10} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 详情 Modal */}
      <Modal
        title={`Secret: ${selectedSecret?.name}`}
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={700}
      >
        {selectedSecret && (
          <div>
            <p><strong>命名空间:</strong> {selectedSecret.namespace}</p>
            <p><strong>类型:</strong> {selectedSecret.type}</p>
            <div style={{ marginBottom: 16 }}>
              <Button
                icon={showValues ? <EyeInvisibleOutlined /> : <EyeOutlined />}
                onClick={() => setShowValues(!showValues)}
              >
                {showValues ? '隐藏值' : '显示值'}
              </Button>
            </div>
            <p><strong>数据:</strong></p>
            <pre style={{ background: '#f5f5f5', padding: 16, borderRadius: 8, maxHeight: 400, overflow: 'auto' }}>
              {JSON.stringify(
                Object.fromEntries(
                  Object.entries(selectedSecret.data || {}).map(([k, v]) => [
                    k,
                    showValues ? decodeBase64(v as string) : '******',
                  ])
                ),
                null,
                2
              )}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default SecretManagement
