import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Tooltip, Modal, Form, Input, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getPVs, createPV, updatePV, deletePV, PV } from '../../api/storage'
import { getClusterList, Cluster } from '../../api/cluster'

const { Title } = Typography

const PersistentVolumes: React.FC = () => {
  const [pvs, setPVs] = useState<PV[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingPV, setEditingPV] = useState<PV | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchPVs()
    }
  }, [selectedCluster])

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

  const fetchPVs = async () => {
    setLoading(true)
    try {
      const res = await getPVs(selectedCluster)
      setPVs(res.data || [])
    } catch (error) {
      console.error('Failed to fetch PVs:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = (record: PV) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 PV "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deletePV(selectedCluster, record.name)
          message.success('删除成功')
          fetchPVs()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleEdit = (record: PV) => {
    setEditingPV(record)
    form.setFieldsValue({
      name: record.name,
      capacity: record.capacity,
      access_modes: record.access_modes,
      reclaim_policy: record.reclaim_policy,
      storage_class: record.storage_class,
    })
    setModalVisible(true)
  }

  const handleCreate = () => {
    setEditingPV(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingPV) {
        await updatePV(selectedCluster, editingPV.name, {
          capacity: values.capacity,
          access_modes: values.access_modes ? values.access_modes.split(',').map((m: string) => m.trim()) : undefined,
          reclaim_policy: values.reclaim_policy,
          storage_class: values.storage_class,
        })
        message.success('更新成功')
      } else {
        await createPV(selectedCluster, {
          name: values.name,
          capacity: values.capacity,
          access_modes: values.access_modes.split(',').map((m: string) => m.trim()),
          reclaim_policy: values.reclaim_policy || 'Retain',
          storage_class: values.storage_class,
          host_path: values.host_path,
        })
        message.success('创建成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchPVs()
    } catch (error) {
      console.error('Operation failed:', error)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string }> = {
      Available: { color: 'success' },
      Bound: { color: 'processing' },
      Released: { color: 'warning' },
      Failed: { color: 'error' },
    }
    const config = statusMap[status] || { color: 'default' }
    return <Tag color={config.color}>{status}</Tag>
  }

  const columns: ColumnsType<PV> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
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
      title: '回收策略',
      dataIndex: 'reclaim_policy',
      key: 'reclaim_policy',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => getStatusTag(status),
    },
    {
      title: '绑定 PVC',
      dataIndex: 'claim',
      key: 'claim',
      render: (claim) => claim || '-',
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
        <Title level={4}>PersistentVolume (PV)</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Button icon={<SyncOutlined />} onClick={fetchPVs}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={pvs} rowKey="name" loading={loading} />
      </Card>

      <Modal
        title={editingPV ? '编辑 PersistentVolume' : '创建 PersistentVolume'}
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
            name="name"
            label="名称"
            rules={[{ required: !editingPV, message: '请输入名称' }]}
          >
            <Input placeholder="请输入 PV 名称" disabled={!!editingPV} />
          </Form.Item>
          <Form.Item
            name="capacity"
            label="容量"
            rules={[{ required: !editingPV, message: '请输入容量' }]}
          >
            <Input placeholder="例如: 10Gi" />
          </Form.Item>
          <Form.Item
            name="access_modes"
            label="访问模式"
            rules={[{ required: !editingPV, message: '请输入访问模式' }]}
          >
            <Input placeholder="例如: ReadWriteOnce,ReadOnlyMany" />
          </Form.Item>
          <Form.Item name="reclaim_policy" label="回收策略" initialValue="Retain">
            <Select
              options={[
                { label: 'Retain', value: 'Retain' },
                { label: 'Delete', value: 'Delete' },
                { label: 'Recycle', value: 'Recycle' },
              ]}
            />
          </Form.Item>
          <Form.Item name="storage_class" label="StorageClass">
            <Input placeholder="请输入 StorageClass 名称" />
          </Form.Item>
          {!editingPV && (
            <Form.Item name="host_path" label="HostPath">
              <Input placeholder="例如: /data/pv" />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  )
}

export default PersistentVolumes
