import { useEffect, useState, useCallback } from 'react'
import {
  Card,
  Table,
  Button,
  Space,
  Tag,
  Modal,
  Form,
  Input,
  message,
  Popconfirm,
  Typography,
  Tree,
  Row,
  Col,
  Spin,
} from 'antd'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  SafetyOutlined,
  UserOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DataNode } from 'antd/es/tree'
import { getRoles, createRole, updateRole, deleteRole, getResourceTypes, getActionTypes } from '../../api/system'

const { Title, Text } = Typography

const resourceLabels: Record<string, string> = {
  clusters: '集群管理',
  deployments: 'Deployment',
  pods: 'Pod',
  services: 'Service',
  configmaps: 'ConfigMap',
  secrets: 'Secret',
  pvcs: 'PVC',
  pvs: 'PV',
  namespaces: '命名空间',
  nodes: '节点',
  events: '事件',
  alerts: '告警',
  users: '用户管理',
  roles: '角色管理',
  audit_logs: '审计日志',
  appstore: '应用商店',
}

const actionLabels: Record<string, string> = {
  view: '查看',
  create: '创建',
  edit: '编辑',
  delete: '删除',
  exec: '执行',
  admin: '管理',
}

const SystemRoles: React.FC = () => {
  const [roles, setRoles] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [modalType, setModalType] = useState<'create' | 'edit'>('create')
  const [selectedRole, setSelectedRole] = useState<any | null>(null)
  const [form] = Form.useForm()
  const [checkedKeys, setCheckedKeys] = useState<string[]>([])
  const [treeData, setTreeData] = useState<DataNode[]>([])
  const [initLoading, setInitLoading] = useState(true)

  const buildTreeData = useCallback((resources: string[], actions: string[]) => {
    if (!resources.length || !actions.length) return []
    return resources.map(resource => ({
      title: resourceLabels[resource] || resource,
      key: resource,
      children: actions.map(action => ({
        title: actionLabels[action] || action,
        key: `${resource}:${action}`,
      })),
    }))
  }, [])

  useEffect(() => {
    const init = async () => {
      setInitLoading(true)
      try {
        const [rolesRes, resourcesRes, actionsRes] = await Promise.all([
          getRoles(),
          getResourceTypes(),
          getActionTypes(),
        ])
        setRoles(rolesRes.data || [])
        const resources = resourcesRes.data || []
        const actions = actionsRes.data || []
        setTreeData(buildTreeData(resources, actions))
      } catch (error) {
        console.error('Failed to initialize:', error)
      } finally {
        setInitLoading(false)
      }
    }
    init()
  }, [buildTreeData])

  const fetchRoles = async () => {
    setLoading(true)
    try {
      const res = await getRoles()
      setRoles(res.data || [])
    } catch (error) {
      console.error('Failed to fetch roles:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setModalType('create')
    setSelectedRole(null)
    form.resetFields()
    setCheckedKeys([])
    setModalVisible(true)
  }

  const handleEdit = (record: any) => {
    setModalType('edit')
    setSelectedRole(record)
    form.setFieldsValue({
      name: record.name,
      description: record.description,
    })

    // 转换权限为checkedKeys
    const keys: string[] = []
    if (record.permissions) {
      record.permissions.forEach((p: any) => {
        if (p.resource === '*') {
          treeData.forEach((node: any) => {
            if (node.children) {
              node.children.forEach((child: any) => {
                keys.push(child.key as string)
              })
            }
          })
        } else {
          p.actions?.forEach((a: string) => {
            if (a === '*') {
              const resourceNode = treeData.find((n: any) => n.key === p.resource)
              if (resourceNode?.children) {
                resourceNode.children.forEach((child: any) => {
                  keys.push(child.key as string)
                })
              }
            } else {
              keys.push(`${p.resource}:${a}`)
            }
          })
        }
      })
    }
    setCheckedKeys(keys)
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      // 转换checkedKeys为permissions
      const permissionMap: Record<string, string[]> = {}
      checkedKeys.forEach(key => {
        const [resource, action] = key.split(':')
        if (resource && action) {
          if (!permissionMap[resource]) {
            permissionMap[resource] = []
          }
          permissionMap[resource].push(action)
        }
      })

      const permissions = Object.entries(permissionMap).map(([resource, actions]) => ({
        resource,
        actions,
      }))

      if (modalType === 'create') {
        await createRole({
          name: values.name,
          description: values.description,
          permissions,
        })
        message.success('角色创建成功')
      } else if (selectedRole) {
        await updateRole(selectedRole.id, {
          name: values.name,
          description: values.description,
          permissions,
        })
        message.success('角色更新成功')
      }
      setModalVisible(false)
      form.resetFields()
      setCheckedKeys([])
      fetchRoles()
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteRole(id)
      message.success('角色已删除')
      fetchRoles()
    } catch (error) {
      console.error('Failed:', error)
    }
  }

  const handleSelectAll = () => {
    const allKeys: string[] = []
    treeData.forEach((node: any) => {
      if (node.children) {
        node.children.forEach((child: any) => {
          allKeys.push(child.key as string)
        })
      }
    })
    setCheckedKeys(allKeys)
  }

  const handleDeselectAll = () => {
    setCheckedKeys([])
  }

  const columns: ColumnsType<any> = [
    {
      title: '角色名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: any) => (
        <Space>
          <SafetyOutlined style={{ color: record.is_system ? '#1890ff' : '#52c41a' }} />
          <Text strong>{name}</Text>
          {record.is_system && <Tag color="blue">系统</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
    },
    {
      title: '权限数',
      key: 'permissions',
      render: (_: any, record: any) => {
        let count = 0
        if (record.permissions) {
          record.permissions.forEach((p: any) => {
            if (p.resource === '*') {
              count = 96 // 16 resources * 6 actions
            } else {
              count += (p.actions?.length || 0)
            }
          })
        }
        return <Tag>{count} 个权限</Tag>
      },
    },
    {
      title: '用户数',
      dataIndex: 'user_count',
      key: 'user_count',
      render: (count: number) => (
        <Space>
          <UserOutlined />
          <Text>{count || 0}</Text>
        </Space>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: any, record: any) => (
        <Space size="small">
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)}>
            编辑
          </Button>
          {!record.is_system && (
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

  if (initLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>角色管理</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
          创建角色
        </Button>
      </div>

      <Card>
        <Table
          columns={columns}
          dataSource={roles}
          rowKey="id"
          loading={loading}
          pagination={false}
        />
      </Card>

      <Modal
        title={modalType === 'create' ? '创建角色' : '编辑角色'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
          setCheckedKeys([])
        }}
        onOk={() => form.submit()}
        width={800}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                name="name"
                label="角色名称"
                rules={[{ required: true, message: '请输入角色名称' }]}
              >
                <Input placeholder="请输入角色名称" disabled={modalType === 'edit' && selectedRole?.is_system} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="description" label="描述">
                <Input placeholder="请输入角色描述" />
              </Form.Item>
            </Col>
          </Row>

          <div style={{ marginBottom: 16 }}>
            <Space>
              <Text strong>权限配置：</Text>
              <Button size="small" onClick={handleSelectAll}>全选</Button>
              <Button size="small" onClick={handleDeselectAll}>取消全选</Button>
            </Space>
          </div>

          <Form.Item>
            <Card size="small" style={{ maxHeight: 400, overflow: 'auto' }}>
              {treeData.length > 0 ? (
                <Tree
                  checkable
                  checkedKeys={checkedKeys}
                  onCheck={(keys: any) => setCheckedKeys(Array.isArray(keys) ? keys : keys.checked)}
                  treeData={treeData}
                  defaultExpandAll={false}
                  defaultExpandedKeys={treeData.map((n: any) => n.key as string)}
                />
              ) : (
                <Text type="secondary">加载中...</Text>
              )}
            </Card>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default SystemRoles
