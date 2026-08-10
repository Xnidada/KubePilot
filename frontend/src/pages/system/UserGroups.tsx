import { useCallback, useEffect, useState } from 'react'
import {
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, type Cluster } from '../../api/cluster'
import {
  createUserGroup,
  deleteUserGroup,
  getUserGroupClusters,
  getUserGroupMembers,
  getUserGroups,
  getUsers,
  replaceUserGroupClusters,
  replaceUserGroupMembers,
  updateUserGroup,
  type User,
  type UserGroup,
  type UserGroupClusterAssignment,
} from '../../api/system'

const { Title, Text } = Typography

interface GroupFormValues {
  name: string
  description?: string
  status: number
}

interface MembersFormValues {
  user_ids: number[]
}

interface AccessFormValues {
  assignments: UserGroupClusterAssignment[]
}

const permissionOptions = [
  { label: '只读', value: 'read' },
  { label: '读写', value: 'write' },
  { label: '管理员', value: 'admin' },
]

const UserGroups: React.FC = () => {
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [editingGroup, setEditingGroup] = useState<UserGroup | null>(null)
  const [groupModalOpen, setGroupModalOpen] = useState(false)
  const [membersGroup, setMembersGroup] = useState<UserGroup | null>(null)
  const [accessGroup, setAccessGroup] = useState<UserGroup | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [groupForm] = Form.useForm<GroupFormValues>()
  const [membersForm] = Form.useForm<MembersFormValues>()
  const [accessForm] = Form.useForm<AccessFormValues>()

  const fetchGroups = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getUserGroups(page, pageSize)
      setGroups(response.data || [])
      setTotal(response.total || 0)
    } finally {
      setLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => {
    void fetchGroups()
  }, [fetchGroups])

  useEffect(() => {
    const loadOptions = async () => {
      const [usersResponse, clustersResponse] = await Promise.all([
        getUsers(1, 100),
        getClusterList(1, 100),
      ])
      setUsers(usersResponse.data || [])
      setClusters(clustersResponse.data || [])
    }
    void loadOptions()
  }, [])

  const openCreate = () => {
    setEditingGroup(null)
    groupForm.resetFields()
    setGroupModalOpen(true)
  }

  const openEdit = (group: UserGroup) => {
    setEditingGroup(group)
    groupForm.setFieldsValue({
      name: group.name,
      description: group.description,
      status: group.status,
    })
    setGroupModalOpen(true)
  }

  const saveGroup = async (values: GroupFormValues) => {
    setSaving(true)
    try {
      if (editingGroup) {
        await updateUserGroup(editingGroup.id, values)
        message.success('用户组已更新')
      } else {
        await createUserGroup(values)
        message.success('用户组已创建')
      }
      setGroupModalOpen(false)
      await fetchGroups()
    } finally {
      setSaving(false)
    }
  }

  const removeGroup = async (id: number) => {
    await deleteUserGroup(id)
    message.success('用户组已删除')
    await fetchGroups()
  }

  const openMembers = async (group: UserGroup) => {
    setMembersGroup(group)
    membersForm.resetFields()
    setDetailLoading(true)
    try {
      const response = await getUserGroupMembers(group.id)
      membersForm.setFieldsValue({ user_ids: (response.data || []).map(member => member.user_id) })
    } finally {
      setDetailLoading(false)
    }
  }

  const saveMembers = async (values: MembersFormValues) => {
    if (!membersGroup) return
    setSaving(true)
    try {
      await replaceUserGroupMembers(membersGroup.id, values.user_ids || [])
      message.success('用户组成员已更新')
      setMembersGroup(null)
      await fetchGroups()
    } finally {
      setSaving(false)
    }
  }

  const openAccess = async (group: UserGroup) => {
    setAccessGroup(group)
    accessForm.resetFields()
    setDetailLoading(true)
    try {
      const response = await getUserGroupClusters(group.id)
      accessForm.setFieldsValue({
        assignments: (response.data || []).map(assignment => ({
          ...assignment,
          namespace: assignment.namespace || '*',
        })),
      })
    } finally {
      setDetailLoading(false)
    }
  }

  const saveAccess = async (values: AccessFormValues) => {
    if (!accessGroup) return
    setSaving(true)
    try {
      await replaceUserGroupClusters(accessGroup.id, values.assignments || [])
      message.success('用户组集群授权已更新')
      setAccessGroup(null)
    } finally {
      setSaving(false)
    }
  }

  const columns: ColumnsType<UserGroup> = [
    {
      title: '用户组',
      dataIndex: 'name',
      render: name => (
        <Space>
          <TeamOutlined />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    { title: '描述', dataIndex: 'description', render: value => value || '-' },
    { title: '成员数', dataIndex: 'member_count', width: 100, render: value => value ?? 0 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: value => <Tag color={value === 1 ? 'green' : 'default'}>{value === 1 ? '启用' : '停用'}</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 180, render: value => value || '-' },
    {
      title: '操作',
      width: 360,
      render: (_, group) => (
        <Space size="small" wrap>
          <Button type="link" icon={<EditOutlined />} onClick={() => openEdit(group)}>编辑</Button>
          <Button type="link" icon={<TeamOutlined />} onClick={() => void openMembers(group)}>成员</Button>
          <Button type="link" icon={<SafetyCertificateOutlined />} onClick={() => void openAccess(group)}>授权</Button>
          <Popconfirm title="确定删除该用户组吗？" onConfirm={() => void removeGroup(group.id)}>
            <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>用户组管理</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>创建用户组</Button>
      </div>
      <Card>
        <Table<UserGroup>
          rowKey="id"
          columns={columns}
          dataSource={groups}
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            onChange: (nextPage, nextSize) => {
              setPage(nextPage)
              setPageSize(nextSize)
            },
          }}
        />
      </Card>

      <Modal
        title={editingGroup ? '编辑用户组' : '创建用户组'}
        open={groupModalOpen}
        onCancel={() => setGroupModalOpen(false)}
        onOk={() => groupForm.submit()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={groupForm} layout="vertical" onFinish={saveGroup} initialValues={{ status: 1 }}>
          <Form.Item name="name" label="用户组名称" rules={[{ required: true, message: '请输入用户组名称' }]}>
            <Input maxLength={64} placeholder="例如：开发团队" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea maxLength={256} rows={3} placeholder="用户组用途" />
          </Form.Item>
          <Form.Item name="status" label="状态" rules={[{ required: true }]}>
            <Select options={[{ value: 1, label: '启用' }, { value: 0, label: '停用' }]} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`成员管理 - ${membersGroup?.name || ''}`}
        open={Boolean(membersGroup)}
        onCancel={() => setMembersGroup(null)}
        onOk={() => membersForm.submit()}
        confirmLoading={saving || detailLoading}
        destroyOnHidden
      >
        <Form form={membersForm} layout="vertical" onFinish={saveMembers} initialValues={{ user_ids: [] }}>
          <Form.Item name="user_ids" label="组成员">
            <Select
              mode="multiple"
              allowClear
              showSearch
              optionFilterProp="label"
              loading={detailLoading}
              placeholder="选择用户"
              options={users.map(user => ({
                value: user.id,
                label: `${user.real_name || user.username} (${user.username})`,
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`集群授权 - ${accessGroup?.name || ''}`}
        open={Boolean(accessGroup)}
        onCancel={() => setAccessGroup(null)}
        onOk={() => accessForm.submit()}
        confirmLoading={saving}
        width={860}
        destroyOnHidden
      >
        <Text type="secondary">命名空间填写 * 表示整个集群；权限级别按 read、write、admin 逐级增强。</Text>
        <Form form={accessForm} layout="vertical" onFinish={saveAccess} initialValues={{ assignments: [] }}>
          <Form.List name="assignments">
            {(fields, { add, remove }) => (
              <Space direction="vertical" style={{ width: '100%', marginTop: 16 }}>
                {fields.map(field => (
                  <Space key={field.key} align="start" style={{ display: 'flex' }}>
                    <Form.Item {...field} name={[field.name, 'cluster_id']} label="集群" rules={[{ required: true, message: '请选择集群' }]}>
                      <Select
                        style={{ width: 240 }}
                        showSearch
                        optionFilterProp="label"
                        loading={detailLoading}
                        options={clusters.map(cluster => ({ value: cluster.id, label: cluster.display_name || cluster.name }))}
                      />
                    </Form.Item>
                    <Form.Item {...field} name={[field.name, 'namespace']} label="命名空间" rules={[{ required: true, message: '请输入命名空间' }]}>
                      <AutoComplete
                        style={{ width: 180 }}
                        options={[{ value: '*' }, { value: 'default' }, { value: 'kube-system' }]}
                        placeholder="* 或 namespace"
                        filterOption={(inputValue, option) =>
                          String(option?.value || '').toLowerCase().includes(inputValue.toLowerCase())
                        }
                      />
                    </Form.Item>
                    <Form.Item {...field} name={[field.name, 'permission_level']} label="权限" rules={[{ required: true, message: '请选择权限' }]}>
                      <Select style={{ width: 150 }} options={permissionOptions} />
                    </Form.Item>
                    <Button danger type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} style={{ marginTop: 30 }} />
                  </Space>
                ))}
                <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ namespace: '*', permission_level: 'read' })} block>
                  添加授权
                </Button>
              </Space>
            )}
          </Form.List>
        </Form>
      </Modal>
    </div>
  )
}

export default UserGroups
