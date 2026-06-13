import { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Button,
  Space,
  Tag,
  Modal,
  Form,
  Input,
  Select,
  message,
  Popconfirm,
  Typography,
  Switch,
} from 'antd'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  UserOutlined,
  KeyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getUsers, createUser, updateUser, deleteUser, resetPassword, getRoles, User, Role } from '../../api/system'

const { Title } = Typography

const SystemUsers: React.FC = () => {
  const [users, setUsers] = useState<User[]>([])
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [modalVisible, setModalVisible] = useState(false)
  const [modalType, setModalType] = useState<'create' | 'edit'>('create')
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchUsers()
    fetchRoles()
  }, [page, pageSize])

  const fetchUsers = async () => {
    setLoading(true)
    try {
      const res = await getUsers(page, pageSize)
      setUsers(res.data || [])
      setTotal(res.total || 0)
    } catch (error) {
      console.error('Failed to fetch users:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchRoles = async () => {
    try {
      const res = await getRoles()
      setRoles(res.data || [])
    } catch (error) {
      console.error('Failed to fetch roles:', error)
    }
  }

  const handleCreate = () => {
    setModalType('create')
    setSelectedUser(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEdit = (record: User) => {
    setModalType('edit')
    setSelectedUser(record)
    form.setFieldsValue({
      username: record.username,
      email: record.email,
      real_name: record.real_name,
      phone: record.phone,
      role_id: record.role_id,
    })
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (modalType === 'create') {
        await createUser(values)
        message.success('用户创建成功')
      } else if (selectedUser) {
        await updateUser(selectedUser.id, values)
        message.success('用户更新成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchUsers()
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteUser(id)
      message.success('用户已删除')
      fetchUsers()
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const handleResetPassword = async (_username: string, id: number) => {
    try {
      await resetPassword(id)
      message.success(`密码已重置为默认密码: kubepilot123`)
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const handleToggleStatus = async (record: User) => {
    try {
      await updateUser(record.id, { status: record.status === 1 ? 0 : 1 })
      message.success(`用户已${record.status === 1 ? '禁用' : '启用'}`)
      fetchUsers()
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const getRoleTag = (roleName: string) => {
    const map: Record<string, { color: string }> = {
      admin: { color: 'red' },
      operator: { color: 'blue' },
      user: { color: 'green' },
      viewer: { color: 'default' },
    }
    const config = map[roleName] || map.viewer
    return <Tag color={config.color}>{roleName}</Tag>
  }

  const columns: ColumnsType<User> = [
    {
      title: '用户名',
      dataIndex: 'username',
      key: 'username',
    },
    {
      title: '姓名',
      dataIndex: 'real_name',
      key: 'real_name',
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      key: 'email',
    },
    {
      title: '角色',
      dataIndex: 'role_name',
      key: 'role_name',
      render: (roleName) => getRoleTag(roleName),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => (
        <Switch
          checked={status === 1}
          onChange={() => handleToggleStatus(record)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
        />
      ),
    },
    {
      title: '最后登录',
      dataIndex: 'last_login',
      key: 'last_login',
      render: (time) => time || '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
    },
    {
      title: '操作',
      key: 'action',
      width: 250,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)}>
            编辑
          </Button>
          <Button
            type="link"
            icon={<KeyOutlined />}
            onClick={() => handleResetPassword(record.username, record.id)}
          >
            重置密码
          </Button>
          {record.username !== 'admin' && (
            <Popconfirm
              title="确定要删除吗？"
              onConfirm={() => handleDelete(record.id)}
            >
              <Button type="link" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>用户管理</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
          创建用户
        </Button>
      </div>

      <Card>
        <Table
          columns={columns}
          dataSource={users}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPage(page)
              setPageSize(pageSize)
            },
          }}
        />
      </Card>

      <Modal
        title={modalType === 'create' ? '创建用户' : '编辑用户'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="请输入用户名" disabled={modalType === 'edit'} />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效的邮箱' },
            ]}
          >
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          <Form.Item name="real_name" label="姓名">
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="phone" label="手机号">
            <Input placeholder="请输入手机号" />
          </Form.Item>
          {modalType === 'create' && (
            <Form.Item
              name="password"
              label="密码"
              rules={[{ required: true, message: '请输入密码' }]}
            >
              <Input.Password placeholder="请输入密码" />
            </Form.Item>
          )}
          <Form.Item
            name="role_id"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
          >
            <Select
              placeholder="请选择角色"
              options={roles.map(r => ({ label: r.description || r.name, value: r.id }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default SystemUsers
