import { useState, useEffect } from 'react'
import {
  Card,
  Form,
  Input,
  Select,
  Button,
  Space,
  Typography,
  message,
  InputNumber,
  Alert,
  Tag,
  Divider,
  AutoComplete,
  Table,
  Modal,
  Popconfirm,
} from 'antd'
import {
  PlusOutlined,
  ApiOutlined,
  ReloadOutlined,
  DeleteOutlined,
  EditOutlined,
  StarOutlined,
  StarFilled,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listLLMConfigs,
  saveLLMConfig,
  updateLLMConfig,
  deleteLLMConfig,
  setDefaultLLMConfig,
  testLLMConfig,
  LLMConfig,
} from '../../api/aiops'

const { Title, Paragraph } = Typography

const modelOptions = [
  { label: 'GPT-4o', value: 'gpt-4o' },
  { label: 'GPT-4o Mini', value: 'gpt-4o-mini' },
  { label: 'GPT-4 Turbo', value: 'gpt-4-turbo' },
  { label: 'GPT-4', value: 'gpt-4' },
  { label: 'GPT-3.5 Turbo', value: 'gpt-3.5-turbo' },
  { label: 'Claude 3.5 Sonnet', value: 'claude-3-5-sonnet-20241022' },
  { label: 'Claude 3.5 Haiku', value: 'claude-3-5-haiku-20241022' },
  { label: 'Claude 3 Opus', value: 'claude-3-opus-20240229' },
  { label: 'Claude 3 Sonnet', value: 'claude-3-sonnet-20240229' },
  { label: 'Claude 3 Haiku', value: 'claude-3-haiku-20240307' },
  { label: 'Qwen 2.5', value: 'qwen2.5' },
  { label: 'Qwen 2', value: 'qwen2' },
  { label: 'Llama 3.1', value: 'llama3.1' },
  { label: 'Llama 3', value: 'llama3' },
  { label: 'Mistral', value: 'mistral' },
  { label: 'DeepSeek V2', value: 'deepseek-coder-v2' },
  { label: 'DeepSeek Chat', value: 'deepseek-chat' },
  { label: 'Gemini Pro', value: 'gemini-pro' },
]

const AISettings: React.FC = () => {
  const [configs, setConfigs] = useState<LLMConfig[]>([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editMode, setEditMode] = useState(false)
  const [selectedConfig, setSelectedConfig] = useState<LLMConfig | null>(null)
  const [form] = Form.useForm()
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)

  useEffect(() => {
    fetchConfigs()
  }, [])

  const fetchConfigs = async () => {
    setLoading(true)
    try {
      const res = await listLLMConfigs()
      if (res.code === 0) {
        setConfigs(res.data || [])
      }
    } catch (error) {
      console.error('Failed to fetch configs:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setEditMode(false)
    setSelectedConfig(null)
    form.resetFields()
    form.setFieldsValue({
      provider: 'openai',
      model: 'gpt-3.5-turbo',
      temperature: 0.7,
      max_tokens: 2048,
      timeout: 120,
    })
    setModalVisible(true)
  }

  const handleEdit = (record: LLMConfig) => {
    setEditMode(true)
    setSelectedConfig(record)
    form.setFieldsValue({
      provider: record.provider,
      api_key: '', // 不回显
      base_url: record.base_url,
      model: record.model,
      temperature: record.temperature,
      max_tokens: record.max_tokens,
      timeout: record.timeout,
    })
    setModalVisible(true)
  }

  const handleSubmit = async (values: any) => {
    try {
      if (editMode && selectedConfig) {
        const updateData: any = {}
        if (values.api_key) updateData.api_key = values.api_key
        if (values.base_url) updateData.base_url = values.base_url
        if (values.model) updateData.model = values.model
        if (values.temperature) updateData.temperature = values.temperature
        if (values.max_tokens) updateData.max_tokens = values.max_tokens
        if (values.timeout) updateData.timeout = values.timeout

        await updateLLMConfig(selectedConfig.id, updateData)
        message.success('配置更新成功')
      } else {
        await saveLLMConfig({
          provider: values.provider,
          api_key: values.api_key,
          base_url: values.base_url,
          model: values.model,
          temperature: values.temperature,
          max_tokens: values.max_tokens,
          timeout: values.timeout,
        })
        message.success('配置创建成功')
      }
      setModalVisible(false)
      form.resetFields()
      fetchConfigs()
    } catch (error: any) {
      message.error(error?.message || '操作失败')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteLLMConfig(id)
      message.success('配置已删除')
      fetchConfigs()
    } catch (error: any) {
      message.error(error?.message || '删除失败')
    }
  }

  const handleSetDefault = async (id: number) => {
    try {
      await setDefaultLLMConfig(id)
      message.success('默认配置已更新')
      fetchConfigs()
    } catch (error: any) {
      message.error(error?.message || '设置失败')
    }
  }

  const handleTest = async () => {
    const values = form.getFieldsValue()
    if (!values.provider || !values.api_key || !values.model) {
      message.warning('请填写 Provider、API Key 和 Model')
      return
    }

    setTesting(true)
    setTestResult(null)

    try {
      const res = await testLLMConfig({
        provider: values.provider,
        api_key: values.api_key,
        base_url: values.base_url,
        model: values.model,
      })

      if (res.code === 0) {
        setTestResult({ success: true, message: '连接成功！' })
      } else {
        setTestResult({ success: false, message: '连接失败' })
      }
    } catch (error: any) {
      setTestResult({ success: false, message: error?.message || '连接失败' })
    } finally {
      setTesting(false)
    }
  }

  const columns: ColumnsType<LLMConfig> = [
    {
      title: 'Provider',
      dataIndex: 'provider',
      key: 'provider',
      render: (provider) => (
        <Tag color={provider === 'openai' ? 'green' : 'blue'}>{provider}</Tag>
      ),
    },
    {
      title: 'Model',
      dataIndex: 'model',
      key: 'model',
    },
    {
      title: 'API Key',
      dataIndex: 'api_key',
      key: 'api_key',
    },
    {
      title: 'Base URL',
      dataIndex: 'base_url',
      key: 'base_url',
      render: (url) => url || '-',
    },
    {
      title: 'Temperature',
      dataIndex: 'temperature',
      key: 'temperature',
    },
    {
      title: '状态',
      key: 'status',
      render: (_, record) => (
        record.is_active ? (
          <Tag icon={<StarFilled />} color="warning">默认</Tag>
        ) : (
          <Tag>普通</Tag>
        )
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 300,
      render: (_, record) => (
        <Space size="small">
          {!record.is_active && (
            <Button
              type="link"
              icon={<StarOutlined />}
              onClick={() => handleSetDefault(record.id)}
            >
              设为默认
            </Button>
          )}
          <Button
            type="link"
            icon={<EditOutlined />}
            onClick={() => handleEdit(record)}
          >
            编辑
          </Button>
          {!record.is_active && (
            <Popconfirm
              title="确定删除此配置？"
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
      <Title level={4}>🤖 AI 设置</Title>

      <Card>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
          <Paragraph>
            管理 LLM 配置，支持 OpenAI、Anthropic 和 Ollama 格式。可以配置多个 LLM，设置默认使用的模型。
          </Paragraph>
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchConfigs}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>添加配置</Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={configs}
          rowKey="id"
          loading={loading}
          pagination={false}
        />
      </Card>

      {/* 创建/编辑 Modal */}
      <Modal
        title={editMode ? '编辑 LLM 配置' : '添加 LLM 配置'}
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false)
          form.resetFields()
          setTestResult(null)
        }}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="provider"
            label="Provider"
            rules={[{ required: true, message: '请选择 Provider' }]}
          >
            <Select
              options={[
                { label: 'OpenAI (兼容 Ollama)', value: 'openai' },
                { label: 'Anthropic (Claude)', value: 'anthropic' },
              ]}
              disabled={editMode}
            />
          </Form.Item>

          <Form.Item
            name="api_key"
            label="API Key"
            rules={editMode ? [] : [{ required: true, message: '请输入 API Key' }]}
            extra={editMode ? '留空则不修改' : ''}
          >
            <Input.Password placeholder="sk-..." autoComplete="off" />
          </Form.Item>

          <Form.Item
            name="base_url"
            label="Base URL (可选)"
            extra="留空使用默认地址。Ollama 示例: http://localhost:11434/v1"
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Form.Item
            name="model"
            label="Model"
            rules={[{ required: true, message: '请输入模型名称' }]}
            extra="可直接输入自定义模型名称"
          >
            <AutoComplete
              placeholder="输入或选择模型名称"
              options={modelOptions}
              filterOption={(inputValue, option) =>
                option?.value?.toString().toLowerCase().includes(inputValue.toLowerCase()) ||
                option?.label?.toString().toLowerCase().includes(inputValue.toLowerCase()) ||
                false
              }
            />
          </Form.Item>

          <Divider>高级配置</Divider>

          <Form.Item name="temperature" label="Temperature">
            <InputNumber min={0} max={2} step={0.1} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item name="max_tokens" label="Max Tokens">
            <InputNumber min={100} max={100000} step={100} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item name="timeout" label="Timeout (秒)">
            <InputNumber min={10} max={600} step={10} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item>
            <Button
              icon={<ApiOutlined />}
              onClick={handleTest}
              loading={testing}
            >
              测试连接
            </Button>
          </Form.Item>
        </Form>

        {testResult && (
          <Alert
            message={testResult.success ? '连接测试成功' : '连接测试失败'}
            description={testResult.message}
            type={testResult.success ? 'success' : 'error'}
            showIcon
            style={{ marginTop: 16 }}
          />
        )}
      </Modal>
    </div>
  )
}

export default AISettings
