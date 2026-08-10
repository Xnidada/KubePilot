import { useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Select, Space, Spin, Typography, message } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { getClusterList, Cluster } from '../api/cluster'
import {
  getUserClusters,
  replaceUserClusters,
  User,
  UserClusterAssignment,
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

const UserClusterAccessModal: React.FC<UserClusterAccessModalProps> = ({
  open,
  user,
  onClose,
}) => {
  const [form] = Form.useForm<ClusterAccessFormValues>()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !user) return

    const load = async () => {
      setLoading(true)
      try {
        const [clusterResponse, assignmentResponse] = await Promise.all([
          getClusterList(1, 100),
          getUserClusters(user.id),
        ])
        setClusters(clusterResponse.data || [])
        form.setFieldsValue({
          assignments: (assignmentResponse.data || []).map((assignment) => ({
            ...assignment,
            namespace: assignment.namespace || '*',
          })),
        })
      } finally {
        setLoading(false)
      }
    }

    form.resetFields()
    void load()
  }, [form, open, user])

  const handleSubmit = async (values: ClusterAccessFormValues) => {
    if (!user) return
    setSaving(true)
    try {
      await replaceUserClusters(user.id, values.assignments || [])
      message.success(`用户 ${user.username} 的集群权限已更新`)
      onClose()
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={`集群权限 - ${user?.username || ''}`}
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={saving}
      width={820}
      destroyOnHidden
    >
      <Typography.Paragraph type="secondary">
        命名空间填写 * 表示整个集群。节点终端仅接受“整个集群 + 管理员”授权。
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
                {fields.map((field) => (
                  <Space key={field.key} align="start" style={{ display: 'flex' }}>
                    <Form.Item
                      {...field}
                      name={[field.name, 'cluster_id']}
                      label="集群"
                      rules={[{ required: true, message: '请选择集群' }]}
                    >
                      <Select
                        style={{ width: 220 }}
                        options={clusters.map((cluster) => ({
                          label: cluster.display_name || cluster.name,
                          value: cluster.id,
                        }))}
                        placeholder="选择集群"
                      />
                    </Form.Item>
                    <Form.Item
                      {...field}
                      name={[field.name, 'namespace']}
                      label="命名空间"
                      rules={[{ required: true, message: '请输入命名空间' }]}
                    >
                      <Input style={{ width: 180 }} placeholder="* 或 namespace" />
                    </Form.Item>
                    <Form.Item
                      {...field}
                      name={[field.name, 'permission_level']}
                      label="权限级别"
                      rules={[{ required: true, message: '请选择权限' }]}
                    >
                      <Select
                        style={{ width: 220 }}
                        options={permissionOptions}
                        placeholder="选择权限"
                      />
                    </Form.Item>
                    <Button
                      danger
                      type="text"
                      icon={<DeleteOutlined />}
                      onClick={() => remove(field.name)}
                      style={{ marginTop: 30 }}
                    />
                  </Space>
                ))}
                <Button
                  type="dashed"
                  icon={<PlusOutlined />}
                  onClick={() => add({ namespace: '*', permission_level: 'read' })}
                  block
                >
                  添加集群授权
                </Button>
              </Space>
            )}
          </Form.List>
        </Form>
      </Spin>
    </Modal>
  )
}

export default UserClusterAccessModal
