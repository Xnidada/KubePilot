import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Tooltip, Modal, Form, Input, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getPVCs, createPVC, updatePVC, deletePVC, PVC } from '../../api/storage'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'

const { Title } = Typography

const PersistentVolumeClaims: React.FC = () => {
  const [pvcs, setPVCs] = useState<PVC[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [modalVisible, setModalVisible] = useState(false)
  const [editingPVC, setEditingPVC] = useState<PVC | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
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

  const fetchPVCs = async () => {
    setLoading(true)
    try {
      const res = await getPVCs(selectedCluster, selectedNamespace)
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

  const handleEdit = (record: PVC) => {
    setEditingPVC(record)
    form.setFieldsValue({
      namespace: record.namespace,
      name: record.name,
      capacity: record.capacity,
      access_modes: record.access_modes,
      storage_class: record.storage_class,
    })
    setModalVisible(true)
  }

  const handleCreate = () => {
    setEditingPVC(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingPVC) {
        await updatePVC(selectedCluster, editingPVC.namespace, editingPVC.name, {
          capacity: values.capacity,
          access_modes: values.access_modes ? values.access_modes.split(',').map((m: string) => m.trim()) : undefined,
          storage_class: values.storage_class,
        })
        message.success('更新成功')
      } else {
        await createPVC(selectedCluster, {
          namespace: values.namespace,
          name: values.name,
          capacity: values.capacity,
          access_modes: values.access_modes.split(',').map((m: string) => m.trim()),
          storage_class: values.storage_class,
          volume_name: values.volume_name,
        })
        message.success('创建成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchPVCs()
    } catch (error) {
      console.error('Operation failed:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string }> = {
      Bound: { color: 'success' },
      Pending: { color: 'processing' },
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
      render: (vol) => vol || '-',
    },
    {
      title: '容量',
      dataIndex: 'capacity',
      key: 'capacity',
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
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
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
            placeholder="选择命名空间"
            allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))}
          />
          <Button icon={<SyncOutlined />} onClick={fetchPVCs}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={pvcs} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      <Modal
        title={editingPVC ? '编辑 PersistentVolumeClaim' : '创建 PersistentVolumeClaim'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="namespace"
            label="命名空间"
            rules={[{ required: !editingPVC, message: '请选择命名空间' }]}
          >
            <Select
              placeholder="选择命名空间"
              disabled={!!editingPVC}
              options={namespaces.map(ns => ({ label: ns, value: ns }))}
            />
          </Form.Item>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: !editingPVC, message: '请输入名称' }]}
          >
            <Input placeholder="请输入 PVC 名称" disabled={!!editingPVC} />
          </Form.Item>
          <Form.Item
            name="capacity"
            label="容量"
            rules={[{ required: !editingPVC, message: '请输入容量' }]}
          >
            <Input placeholder="例如: 10Gi" />
          </Form.Item>
          <Form.Item
            name="access_modes"
            label="访问模式"
            rules={[{ required: !editingPVC, message: '请输入访问模式' }]}
          >
            <Input placeholder="例如: ReadWriteOnce,ReadOnlyMany" />
          </Form.Item>
          <Form.Item name="storage_class" label="StorageClass">
            <Input placeholder="请输入 StorageClass 名称" />
          </Form.Item>
          {!editingPVC && (
            <Form.Item name="volume_name" label="绑定 PV 名称">
              <Input placeholder="指定要绑定的 PV（可选）" />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  )
}

export default PersistentVolumeClaims
