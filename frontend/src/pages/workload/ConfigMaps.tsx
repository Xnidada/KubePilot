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
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getConfigMaps, getConfigMap, createConfigMap, updateConfigMap, deleteConfigMap, ConfigMap } from '../../api/resources'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'

const { Title } = Typography
const { TextArea } = Input

const ConfigMapManagement: React.FC = () => {
  const [configMaps, setConfigMaps] = useState<ConfigMap[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedCM, setSelectedCM] = useState<any>(null)
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchConfigMaps()
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

  const fetchConfigMaps = async () => {
    setLoading(true)
    try {
      const res = await getConfigMaps(selectedCluster, selectedNamespace || undefined)
      setConfigMaps(res.data || [])
    } catch (error) {
      console.error('Failed to fetch configmaps:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (values: any) => {
    try {
      const data: Record<string, string> = {}
      if (values.data) {
        values.data.split('\n').forEach((line: string) => {
          const [key, ...valueParts] = line.split('=')
          if (key && valueParts.length > 0) {
            data[key.trim()] = valueParts.join('=').trim()
          }
        })
      }
      await createConfigMap(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        data,
      })
      message.success('ConfigMap 创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchConfigMaps()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const handleEdit = async (record: ConfigMap) => {
    try {
      const res = await getConfigMap(selectedCluster, record.namespace, record.name)
      setSelectedCM(res.data)
      editForm.setFieldsValue({
        data: Object.entries(res.data.data || {}).map(([k, v]) => `${k}=${v}`).join('\n'),
      })
      setEditModalVisible(true)
    } catch (error) {
      console.error('Failed to fetch configmap:', error)
    }
  }

  const handleUpdate = async (values: any) => {
    if (!selectedCM) return
    try {
      const data: Record<string, string> = {}
      if (values.data) {
        values.data.split('\n').forEach((line: string) => {
          const [key, ...valueParts] = line.split('=')
          if (key && valueParts.length > 0) {
            data[key.trim()] = valueParts.join('=').trim()
          }
        })
      }
      await updateConfigMap(selectedCluster, selectedCM.namespace, selectedCM.name, { data })
      message.success('ConfigMap 更新成功')
      setEditModalVisible(false)
      editForm.resetFields()
      fetchConfigMaps()
    } catch (error) {
      console.error('Update failed:', error)
    }
  }

  const handleDelete = async (record: ConfigMap) => {
    try {
      await deleteConfigMap(selectedCluster, record.namespace, record.name)
      message.success('ConfigMap 已删除')
      fetchConfigMaps()
    } catch (error) {
      console.error('Delete failed:', error)
    }
  }

  const handleViewDetail = async (record: ConfigMap) => {
    try {
      const res = await getConfigMap(selectedCluster, record.namespace, record.name)
      setSelectedCM(res.data)
      setDetailModalVisible(true)
    } catch (error) {
      console.error('Failed to fetch configmap:', error)
    }
  }

  const columns: ColumnsType<ConfigMap> = [
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
        <Title level={4}>ConfigMap</Title>
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
          <Button icon={<SyncOutlined />} onClick={fetchConfigMaps}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={configMaps} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* 创建 Modal */}
      <Modal
        title="创建 ConfigMap"
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
            <Input placeholder="configmap 名称" />
          </Form.Item>
          <Form.Item name="data" label="数据 (key=value 格式，每行一条)" help="例如: database_url=postgres://localhost/mydb">
            <TextArea rows={8} placeholder="key1=value1&#10;key2=value2" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 编辑 Modal */}
      <Modal
        title={`编辑 ConfigMap: ${selectedCM?.name}`}
        open={editModalVisible}
        onCancel={() => { setEditModalVisible(false); editForm.resetFields() }}
        onOk={() => editForm.submit()}
        width={600}
      >
        <Form form={editForm} layout="vertical" onFinish={handleUpdate}>
          <Form.Item name="data" label="数据 (key=value 格式，每行一条)">
            <TextArea rows={10} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 详情 Modal */}
      <Modal
        title={`ConfigMap: ${selectedCM?.name}`}
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={700}
      >
        {selectedCM && (
          <div>
            <p><strong>命名空间:</strong> {selectedCM.namespace}</p>
            <p><strong>数据:</strong></p>
            <pre style={{ background: '#f5f5f5', padding: 16, borderRadius: 8, maxHeight: 400, overflow: 'auto' }}>
              {JSON.stringify(selectedCM.data, null, 2)}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default ConfigMapManagement
