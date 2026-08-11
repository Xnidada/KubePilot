import { useCallback, useEffect, useState } from 'react'
import {
  Alert, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography, message,
} from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SafetyOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  OAuthConfigItem,
  createOAuthConfig,
  deleteOAuthConfig,
  listOAuthConfigs,
  updateOAuthConfig,
} from '../../api/system'

const { Title, Text, Paragraph } = Typography

const PROVIDER_OPTIONS = [
  { value: 'github', label: 'GitHub' },
  { value: 'gitlab', label: 'GitLab' },
  { value: 'google', label: 'Google' },
]

const SSOSettings: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<OAuthConfigItem[]>([])
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<OAuthConfigItem | null>(null)
  const [form] = Form.useForm()

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listOAuthConfigs()
      setItems(res.data || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchList() }, [fetchList])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      provider: 'github',
      enabled: true,
      default_role: 2,
      redirect_url: `${window.location.origin}/api/v1/oauth/github/callback`,
    })
    setOpen(true)
  }

  const openEdit = (row: OAuthConfigItem) => {
    setEditing(row)
    form.setFieldsValue({
      provider: row.provider,
      name: row.name,
      client_id: row.client_id,
      client_secret: '',
      redirect_url: row.redirect_url,
      auth_url: row.auth_url,
      token_url: row.token_url,
      userinfo_url: row.userinfo_url,
      scopes: row.scopes,
      enabled: row.enabled,
      default_role: row.default_role || 2,
    })
    setOpen(true)
  }

  const onProviderChange = (provider: string) => {
    form.setFieldsValue({
      redirect_url: `${window.location.origin}/api/v1/oauth/${provider}/callback`,
      name: PROVIDER_OPTIONS.find((p) => p.value === provider)?.label || provider,
    })
  }

  const submit = async () => {
    const values = await form.validateFields()
    try {
      if (editing) {
        const payload: Record<string, unknown> = { ...values }
        delete payload.provider
        if (!values.client_secret) delete payload.client_secret
        await updateOAuthConfig(editing.id, payload)
        message.success('已更新')
      } else {
        await createOAuthConfig(values)
        message.success('已创建')
      }
      setOpen(false)
      fetchList()
    } catch (e: any) {
      message.error(e?.message || '保存失败')
    }
  }

  const columns: ColumnsType<OAuthConfigItem> = [
    { title: '名称', dataIndex: 'name' },
    { title: 'Provider', dataIndex: 'provider', render: (v) => <Tag>{v}</Tag> },
    { title: 'Client ID', dataIndex: 'client_id', ellipsis: true },
    {
      title: '启用',
      dataIndex: 'enabled',
      render: (v, row) => (
        <Switch
          checked={v}
          onChange={async (checked) => {
            try {
              await updateOAuthConfig(row.id, { enabled: checked })
              fetchList()
            } catch {
              message.error('更新失败')
            }
          }}
        />
      ),
    },
    {
      title: '操作',
      width: 140,
      render: (_, row) => (
        <Space>
          <Button type="link" icon={<EditOutlined />} onClick={() => openEdit(row)} />
          <Popconfirm title="确认删除？" onConfirm={async () => {
            await deleteOAuthConfig(row.id)
            message.success('已删除')
            fetchList()
          }}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}><SafetyOutlined /> SSO / OAuth</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>添加提供商</Button>
      </div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="配置后将在登录页显示对应 SSO 按钮"
        description={
          <Paragraph style={{ marginBottom: 0 }}>
            Redirect URL 需在第三方应用中登记，格式一般为{' '}
            <Text code>{`${window.location.origin}/api/v1/oauth/<provider>/callback`}</Text>
          </Paragraph>
        }
      />
      <Card>
        <Table rowKey="id" loading={loading} columns={columns} dataSource={items} pagination={false} />
      </Card>

      <Modal
        title={editing ? '编辑 OAuth 提供商' : '添加 OAuth 提供商'}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={submit}
        width={640}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="provider" label="Provider" rules={[{ required: true }]}>
            <Select options={PROVIDER_OPTIONS} disabled={!!editing} onChange={onProviderChange} />
          </Form.Item>
          <Form.Item name="name" label="显示名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="client_id" label="Client ID" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="client_secret"
            label="Client Secret"
            rules={editing ? [] : [{ required: true, message: '请输入 Client Secret' }]}
            extra={editing ? '留空表示不修改密钥' : undefined}
          >
            <Input.Password placeholder={editing ? '留空不修改' : ''} />
          </Form.Item>
          <Form.Item name="redirect_url" label="Redirect URL" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="scopes" label="Scopes">
            <Input placeholder="可选，留空使用默认" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default SSOSettings
