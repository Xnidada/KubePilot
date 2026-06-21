import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Tooltip, Modal, Form, Input, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listStorageClasses,
  createStorageClass,
  updateStorageClass,
  deleteStorageClass,
  StorageClass,
} from '../../api/storage'
import { getClusterList, Cluster } from '../../api/cluster'

const { Title } = Typography

const StorageClasses: React.FC = () => {
  const [storageClasses, setStorageClasses] = useState<StorageClass[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingSC, setEditingSC] = useState<StorageClass | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchStorageClasses()
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

  const fetchStorageClasses = async () => {
    setLoading(true)
    try {
      const res = await listStorageClasses(selectedCluster)
      setStorageClasses(res.data || [])
    } catch (error) {
      console.error('Failed to fetch StorageClasses:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = (record: StorageClass) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 StorageClass "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deleteStorageClass(selectedCluster, record.name)
          message.success('删除成功')
          fetchStorageClasses()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleEdit = (record: StorageClass) => {
    setEditingSC(record)
    form.setFieldsValue({
      name: record.name,
      provisioner: record.provisioner,
      reclaim_policy: record.reclaim_policy,
      volume_binding_mode: record.volume_binding_mode,
    })
    setModalVisible(true)
  }

  const handleCreate = () => {
    setEditingSC(null)
    form.resetFields()
    form.setFieldsValue({ reclaim_policy: 'Delete', volume_binding_mode: 'Immediate' })
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingSC) {
        await updateStorageClass(selectedCluster, editingSC.name, {
          provisioner: values.provisioner,
          reclaim_policy: values.reclaim_policy,
          volume_binding_mode: values.volume_binding_mode,
        })
        message.success('更新成功')
      } else {
        await createStorageClass(selectedCluster, {
          name: values.name,
          provisioner: values.provisioner,
          reclaim_policy: values.reclaim_policy,
          volume_binding_mode: values.volume_binding_mode,
        })
        message.success('创建成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchStorageClasses()
    } catch (error) {
      console.error('Operation failed:', error)
    }
  }

  const columns: ColumnsType<StorageClass> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'Provisioner',
      dataIndex: 'provisioner',
      key: 'provisioner',
    },
    {
      title: '回收策略',
      dataIndex: 'reclaim_policy',
      key: 'reclaim_policy',
      render: (policy) => (
        <Tag color={policy === 'Delete' ? 'error' : 'processing'}>{policy}</Tag>
      ),
    },
    {
      title: '绑定模式',
      dataIndex: 'volume_binding_mode',
      key: 'volume_binding_mode',
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
        <Title level={4}>StorageClass</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Button icon={<SyncOutlined />} onClick={fetchStorageClasses}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={storageClasses} rowKey="name" loading={loading} />
      </Card>

      <Modal
        title={editingSC ? '编辑 StorageClass' : '创建 StorageClass'}
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
            rules={[{ required: !editingSC, message: '请输入名称' }]}
          >
            <Input placeholder="请输入 StorageClass 名称" disabled={!!editingSC} />
          </Form.Item>
          <Form.Item
            name="provisioner"
            label="Provisioner"
            rules={[{ required: true, message: '请输入 Provisioner' }]}
          >
            <Input placeholder="例如: kubernetes.io/aws-ebs, kubernetes.io/no-provisioner" />
          </Form.Item>
          <Form.Item name="reclaim_policy" label="回收策略" initialValue="Delete">
            <Select
              options={[
                { label: 'Delete', value: 'Delete' },
                { label: 'Retain', value: 'Retain' },
              ]}
            />
          </Form.Item>
          <Form.Item name="volume_binding_mode" label="绑定模式" initialValue="Immediate">
            <Select
              options={[
                { label: 'Immediate', value: 'Immediate' },
                { label: 'WaitForFirstConsumer', value: 'WaitForFirstConsumer' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default StorageClasses
