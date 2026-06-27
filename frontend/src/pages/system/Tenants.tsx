import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, message, Tag, Modal, Form, Input,
  Popconfirm, InputNumber
} from 'antd'
import {
  PlusOutlined, SyncOutlined, DeleteOutlined, EditOutlined, TeamOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { get, post, put, del } from '../../api/request'

const { Title, Text } = Typography

interface Tenant {
  id: number
  name: string
  display_name: string
  description: string
  max_cpu: string
  max_memory: string
  max_gpu: number
  max_namespaces: number
  max_pods: number
  status: string
  member_count: number
  namespace_count: number
  created_at: string
}

const Tenants: React.FC = () => {
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingTenant, setEditingTenant] = useState<Tenant | null>(null)
  const [form] = Form.useForm()

  useEffect(() => { fetchTenants() }, [])

  const fetchTenants = async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: Tenant[] }>('/tenants')
      setTenants(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleCreate = () => {
    setEditingTenant(null)
    form.resetFields()
    form.setFieldsValue({ max_namespaces: 5, max_pods: 50 })
    setModalVisible(true)
  }

  const handleEdit = (tenant: Tenant) => {
    setEditingTenant(tenant)
    form.setFieldsValue(tenant)
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editingTenant) {
        await put(`/tenants/${editingTenant.id}`, values)
        message.success('租户已更新')
      } else {
        await post('/tenants', values)
        message.success('租户已创建')
      }
      setModalVisible(false)
      form.resetFields()
      fetchTenants()
    } catch (e) { message.error('操作失败') }
  }

  const handleDelete = async (id: number) => {
    try {
      await del(`/tenants/${id}`)
      message.success('租户已删除')
      fetchTenants()
    } catch (e) { message.error('删除失败') }
  }

  const columns: ColumnsType<Tenant> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '显示名称', dataIndex: 'display_name', key: 'display_name' },
    {
      title: '资源配额', key: 'quota',
      render: (_, r) => (
        <Space direction="vertical" size={0}>
          <Text type="secondary">CPU: {r.max_cpu || '无限制'}</Text>
          <Text type="secondary">内存: {r.max_memory || '无限制'}</Text>
          <Text type="secondary">GPU: {r.max_gpu}</Text>
        </Space>
      )
    },
    {
      title: '限制', key: 'limits',
      render: (_, r) => (
        <Space direction="vertical" size={0}>
          <Text>命名空间: {r.namespace_count}/{r.max_namespaces}</Text>
          <Text>最大 Pod: {r.max_pods}</Text>
        </Space>
      )
    },
    { title: '成员', dataIndex: 'member_count', key: 'members' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => <Tag color={s === 'active' ? 'success' : 'default'}>{s === 'active' ? '活跃' : s}</Tag>
    },
    {
      title: '操作', key: 'action', width: 150,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}><TeamOutlined /> 租户管理</Title>
        <Space>
          <Button icon={<SyncOutlined />} onClick={fetchTenants}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>创建租户</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={tenants} rowKey="id" loading={loading} />
      </Card>

      <Modal
        title={editingTenant ? '编辑租户' : '创建租户'}
        open={modalVisible}
        onCancel={() => { setModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="name" label="标识" rules={[{ required: true }]}>
            <Input placeholder="租户标识" disabled={!!editingTenant} />
          </Form.Item>
          <Form.Item name="display_name" label="显示名称">
            <Input placeholder="显示名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="描述" />
          </Form.Item>
          <Form.Item label="资源配额">
            <Space>
              <Form.Item name="max_cpu" noStyle>
                <Input placeholder="CPU (如: 8)" style={{ width: 120 }} addonAfter="核" />
              </Form.Item>
              <Form.Item name="max_memory" noStyle>
                <Input placeholder="内存 (如: 16Gi)" style={{ width: 120 }} />
              </Form.Item>
              <Form.Item name="max_gpu" noStyle initialValue={0}>
                <InputNumber min={0} style={{ width: 100 }} addonAfter="GPU" />
              </Form.Item>
            </Space>
          </Form.Item>
          <Form.Item label="限制">
            <Space>
              <Form.Item name="max_namespaces" noStyle initialValue={5}>
                <InputNumber min={1} style={{ width: 120 }} addonBefore="命名空间" addonAfter="个" />
              </Form.Item>
              <Form.Item name="max_pods" noStyle initialValue={50}>
                <InputNumber min={1} style={{ width: 120 }} addonBefore="最大Pod" addonAfter="个" />
              </Form.Item>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Tenants
