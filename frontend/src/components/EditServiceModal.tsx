import { useEffect, useState } from 'react'
import { Modal, Form, Input, InputNumber, Select, Space, Button, message } from 'antd'
import { PlusOutlined, MinusCircleOutlined } from '@ant-design/icons'

interface EditServiceModalProps {
  visible: boolean
  onClose: () => void
  onSuccess: () => void
  clusterId: number
  namespace: string
  name: string
  initialValues?: any
}

const EditServiceModal: React.FC<EditServiceModalProps> = ({
  visible,
  onClose,
  onSuccess,
  clusterId,
  namespace,
  name,
  initialValues,
}) => {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (visible && initialValues) {
      form.setFieldsValue({
        type: initialValues.type,
        ports: initialValues.ports || [],
      })
    }
  }, [visible, initialValues])

  const handleSubmit = async (values: any) => {
    setLoading(true)
    try {
      const token = localStorage.getItem('auth-storage')
      const authData = token ? JSON.parse(token) : null
      const authToken = authData?.state?.token

      const response = await fetch(`/api/v1/clusters/${clusterId}/workloads/services/${namespace}/${name}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          type: values.type,
          ports: values.ports,
        }),
      })

      const data = await response.json()
      if (data.code === 0) {
        message.success('更新成功')
        onSuccess()
        onClose()
      } else {
        message.error(data.message || '更新失败')
      }
    } catch (error) {
      message.error('更新失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title={`编辑 Service: ${name}`}
      open={visible}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={loading}
      width={700}
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Form.Item name="type" label="类型">
          <Select
            options={[
              { label: 'ClusterIP', value: 'ClusterIP' },
              { label: 'NodePort', value: 'NodePort' },
              { label: 'LoadBalancer', value: 'LoadBalancer' },
            ]}
          />
        </Form.Item>

        <Form.Item label="端口配置">
          <Form.List name="ports">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline" wrap>
                    <Form.Item
                      {...restField}
                      name={[name, 'name']}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder="名称" style={{ width: 80 }} />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'port']}
                      rules={[{ required: true }]}
                      style={{ marginBottom: 0 }}
                    >
                      <InputNumber placeholder="服务端口" min={1} max={65535} style={{ width: 100 }} />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'target_port']}
                      rules={[{ required: true }]}
                      style={{ marginBottom: 0 }}
                    >
                      <InputNumber placeholder="容器端口" min={1} max={65535} style={{ width: 100 }} />
                    </Form.Item>
                    <Form.Item
                      noStyle
                      shouldUpdate={(prevValues, curValues) => prevValues.type !== curValues.type}
                    >
                      {({ getFieldValue }) => {
                        if (getFieldValue('type') === 'NodePort') {
                          return (
                            <Form.Item
                              {...restField}
                              name={[name, 'node_port']}
                              style={{ marginBottom: 0 }}
                            >
                              <InputNumber placeholder="NodePort" min={30000} max={32767} style={{ width: 110 }} />
                            </Form.Item>
                          )
                        }
                        return null
                      }}
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'protocol']}
                      initialValue="TCP"
                      style={{ marginBottom: 0 }}
                    >
                      <Select style={{ width: 80 }}>
                        <Select.Option value="TCP">TCP</Select.Option>
                        <Select.Option value="UDP">UDP</Select.Option>
                      </Select>
                    </Form.Item>
                    <MinusCircleOutlined onClick={() => remove(name)} />
                  </Space>
                ))}
                <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                  添加端口
                </Button>
              </>
            )}
          </Form.List>
        </Form.Item>
      </Form>
    </Modal>
  )
}

export default EditServiceModal
