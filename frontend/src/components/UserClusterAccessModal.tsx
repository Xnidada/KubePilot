import { useEffect, useState } from 'react'
import {
  Button,
  Collapse,
  Divider,
  Empty,
  AutoComplete,
  Form,
  Modal,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { getClusterList, type Cluster } from '../api/cluster'
import {
  getUserClusters,
  getUserEffectiveClusterPermissions,
  replaceUserClusters,
  type EffectiveUserClusterPermission,
  type User,
  type UserClusterAssignment,
} from '../api/system'

interface UserClusterAccessModalProps {
  open: boolean
  user: User | null
  onClose: () => void
}

interface ClusterAccessFormValues {
  assignments: UserClusterAssignment[]
}

const permissionOptions = [
  { label: '只读', value: 'read' },
  { label: '读写（允许 Pod 终端）', value: 'write' },
  { label: '管理员（允许节点终端）', value: 'admin' },
]

const permissionColors = { read: 'blue', write: 'orange', admin: 'red' } as const

const UserClusterAccessModal: React.FC<UserClusterAccessModalProps> = ({
  open,
  user,
  onClose,
}) => {
  const [form] = Form.useForm<ClusterAccessFormValues>()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [effectivePermissions, setEffectivePermissions] = useState<EffectiveUserClusterPermission[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !user) return

    const load = async () => {
      setLoading(true)
      try {
        const [clusterResult, directResult, effectiveResult] = await Promise.allSettled([
          getClusterList(1, 100),
          getUserClusters(user.id),
          getUserEffectiveClusterPermissions(user.id),
        ])
        if (clusterResult.status === 'fulfilled') {
          setClusters(clusterResult.value.data || [])
        }
        if (directResult.status === 'fulfilled') {
          form.setFieldsValue({
            assignments: (directResult.value.data || []).map(assignment => ({
              ...assignment,
              namespace: assignment.namespace || '*',
            })),
          })
        }
        if (effectiveResult.status === 'fulfilled') {
          const preview = effectiveResult.value.data
          setEffectivePermissions(Array.isArray(preview) ? preview : preview.grants || [])
        } else {
          setEffectivePermissions([])
        }
      } finally {
        setLoading(false)
      }
    }

    form.resetFields()
    setEffectivePermissions([])
    void load()
  }, [form, open, user])

  const handleSubmit = async (values: ClusterAccessFormValues) => {
    if (!user) return
    setSaving(true)
    try {
      await replaceUserClusters(user.id, values.assignments || [])
      message.success(`用户 ${user.username} 的直接集群权限已更新`)
      onClose()
    } finally {
      setSaving(false)
    }
  }

  const getClusterName = (permission: EffectiveUserClusterPermission) => {
    if (permission.cluster_display_name || permission.cluster_name) {
      return permission.cluster_display_name || permission.cluster_name
    }
    const cluster = clusters.find(item => item.id === permission.cluster_id)
    return cluster?.display_name || cluster?.name || `集群 #${permission.cluster_id}`
  }

  return (
    <Modal
      title={`集群权限 - ${user?.username || ''}`}
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={saving}
      width={900}
      destroyOnHidden
    >
      <Typography.Title level={5}>直接授权</Typography.Title>
      <Typography.Paragraph type="secondary">
        这里的修改只影响用户直接授权。命名空间填写 * 表示整个集群；节点终端仅接受“整个集群 + 管理员”授权。
      </Typography.Paragraph>
      <Spin spinning={loading}>
        <Form<ClusterAccessFormValues>
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{ assignments: [] }}
        >
          <Form.List name="assignments">
            {(fields, { add, remove }) => (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                {fields.map(field => (
                  <Space key={field.key} align="start" style={{ display: 'flex' }}>
                    <Form.Item {...field} name={[field.name, 'cluster_id']} label="集群" rules={[{ required: true, message: '请选择集群' }]}>
                      <Select
                        style={{ width: 240 }}
                        showSearch
                        optionFilterProp="label"
                        options={clusters.map(cluster => ({ label: cluster.display_name || cluster.name, value: cluster.id }))}
                        placeholder="选择集群"
                      />
                    </Form.Item>
                    <Form.Item {...field} name={[field.name, 'namespace']} label="命名空间" rules={[{ required: true, message: '请选择或输入命名空间' }]}>
                      <AutoComplete
                        style={{ width: 190 }}
                        options={[{ value: '*' }, { value: 'default' }, { value: 'kube-system' }]}
                        placeholder="* 或命名空间"
                        filterOption={(inputValue, option) =>
                          String(option?.value || '').toLowerCase().includes(inputValue.toLowerCase())
                        }
                      />
                    </Form.Item>
                    <Form.Item {...field} name={[field.name, 'permission_level']} label="权限级别" rules={[{ required: true, message: '请选择权限' }]}>
                      <Select style={{ width: 220 }} options={permissionOptions} placeholder="选择权限" />
                    </Form.Item>
                    <Button danger type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} style={{ marginTop: 30 }} />
                  </Space>
                ))}
                <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ namespace: '*', permission_level: 'read' })} block>
                  添加直接授权
                </Button>
              </Space>
            )}
          </Form.List>
        </Form>

        <Divider />
        <Typography.Title level={5}>有效授权</Typography.Title>
        <Typography.Paragraph type="secondary">
          有效权限合并用户直接授权和用户组授权。展开条目可查看每项权限的来源。
        </Typography.Paragraph>
        {effectivePermissions.length ? (
          <Collapse
            size="small"
            items={effectivePermissions.map((permission, index) => ({
              key: `${permission.cluster_id}:${permission.namespace}:${index}`,
              label: (
                <Space>
                  <span>{getClusterName(permission)} / {permission.namespace || '*'}</span>
                  <Tag color={permissionColors[permission.permission_level]}>{permission.permission_level}</Tag>
                </Space>
              ),
              children: permission.sources?.length ? (
                <Space direction="vertical">
                  {permission.sources.map((source, sourceIndex) => (
                    <Space key={`${source.source_type}:${source.source_id || sourceIndex}`}>
                      <Tag color={source.source_type === 'direct' ? 'blue' : 'purple'}>
                        {source.source_type === 'direct' ? '直接授权' : '用户组'}
                      </Tag>
                      <span>{source.source_name || (source.source_type === 'direct' ? user?.username : `用户组 #${source.source_id}`)}</span>
                      <Tag color={permissionColors[source.permission_level]}>{source.permission_level}</Tag>
                    </Space>
                  ))}
                </Space>
              ) : (
                <Typography.Text type="secondary">后端未返回授权来源明细</Typography.Text>
              ),
            }))}
          />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无有效授权数据" />
        )}
      </Spin>
    </Modal>
  )
}

export default UserClusterAccessModal
