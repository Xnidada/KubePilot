import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Tooltip, Modal, Form, Input, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getPVCs, createPVC, deletePVC, PVC, getStorageClasses, StorageClass } from '../../api/storage'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'

const { Title } = Typography

const PersistentVolumeClaims: React.FC = () => {
  const [pvcs, setPVCs] = useState<PVC[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [storageClasses, setStorageClasses] = useState<StorageClass[]>([])
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchStorageClasses()
      fetchPVCs()
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

  const fetchStorageClasses = async () => {
    try {
      const res = await getStorageClasses(selectedCluster)
      setStorageClasses(res.data || [])
    } catch (error) {
      console.error('Failed to fetch storage classes:', error)
    }
  }

  const fetchPVCs = async () => {
    setLoading(true)
    try {
      const res = await getPVCs(selectedCluster, selectedNamespace || undefined)
      setPVCs(res.data || [])
    } catch (error) {
      console.error('Failed to fetch PVCs:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = (record: PVC) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 PVC "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deletePVC(selectedCluster, record.namespace, record.name)
          message.success('删除成功')
          fetchPVCs()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleCreate = async (values: any) => {
    try {
      await createPVC(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        capacity: values.capacity,
        access_modes: values.access_modes.split(',').map((m: string) => m.trim()),
        storage_class: values.storage_class,
        volume_name: values.volume_name,
      })
      message.success('创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      fetchPVCs()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string }> = {
      Bound: { color: 'success' },
      Pending: { color: 'warning' },
      Lost: { color: 'error' },
    }
    const config = statusMap[status] || { color: 'default' }
    return <Tag color={config.color}>{status}</Tag>
  }

  const columns: ColumnsType<PVC> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => getStatusTag(status),
    },
    {
      title: '绑定 PV',
      dataIndex: 'volume',
      key: 'volume',
      render: (volume) => volume || '-',
    },
    {
      title: '容量',
      dataIndex: 'capacity',
      key: 'capacity',
      render: (capacity) => capacity || '-',
    },
    {
      title: '访问模式',
      dataIndex: 'access_modes',
      key: 'access_modes',
    },
    {
      title: 'StorageClass',
      dataIndex: 'storage_class',
      key: 'storage_class',
      render: (sc) => sc || '-',
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
          <Tooltip title="删除">
            <Button type="link" danger icon={<DeleteOutlined />} onClick={() => handleDelete(record)} />
          </Tooltip>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>PersistentVolumeClaim (PVC)</Title>
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
          <Button icon={<SyncOutlined />} onClick={fetchPVCs}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={pvcs} rowKey={(record) => `${record.namespace}/${record.name}`} loading={loading} />
      </Card>

      <Modal
        title="创建 PersistentVolumeClaim"
        open={createModalVisible}
        onCancel={() => {
          setCreateModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item
            name="namespace"
            label="命名空间"
            rules={[{ required: true, message: '请选择命名空间' }]}
          >
            <Select
              placeholder="选择命名空间"
              options={namespaces.map(ns => ({ label: ns, value: ns }))}
            />
          </Form.Item>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="请输入 PVC 名称" />
          </Form.Item>
          <Form.Item
            name="capacity"
            label="请求容量"
            rules={[{ required: true, message: '请输入容量' }]}
          >
            <Input placeholder="例如: 10Gi" />
          </Form.Item>
          <Form.Item
            name="access_modes"
            label="访问模式"
            rules={[{ required: true, message: '请输入访问模式' }]}
          >
            <Input placeholder="例如: ReadWriteOnce" />
          </Form.Item>
          <Form.Item name="storage_class" label="StorageClass">
            <Select
              placeholder="选择 StorageClass"
              allowClear
              options={storageClasses.map(sc => ({ label: sc.name, value: sc.name }))}
            />
          </Form.Item>
          <Form.Item name="volume_name" label="绑定 PV (可选)">
            <Input placeholder="指定绑定的 PV 名称" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default PersistentVolumeClaims
