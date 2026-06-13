import { useEffect, useState } from 'react'
import {
  Modal,
  Form,
  Input,
  Select,
  Button,
  Space,
  Tabs,
  InputNumber,
  Switch,
  Divider,
  message,
  Row,
  Col,
  Spin,
  Card,
  Tag,
  Typography,
} from 'antd'

const { Text } = Typography
import { PlusOutlined, MinusCircleOutlined } from '@ant-design/icons'
import { getDeploymentDetail, updateDeployment, getDeploymentServices, DeploymentDetail } from '../api/workload'

interface EditDeploymentModalProps {
  visible: boolean
  onClose: () => void
  onSuccess: () => void
  clusterId: number
  namespace: string
  name: string
}

const EditDeploymentModal: React.FC<EditDeploymentModalProps> = ({
  visible,
  onClose,
  onSuccess,
  clusterId,
  namespace,
  name,
}) => {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [fetching, setFetching] = useState(false)
  const [associatedServices, setAssociatedServices] = useState<any[]>([])

  useEffect(() => {
    if (visible) {
      fetchDeploymentDetail()
      fetchAssociatedServices()
    }
  }, [visible])

  const fetchDeploymentDetail = async () => {
    setFetching(true)
    try {
      const res = await getDeploymentDetail(clusterId, namespace, name)
      if (res.code === 0) {
        populateForm(res.data)
      }
    } catch (error) {
      console.error('Failed to fetch deployment detail:', error)
      message.error('获取 Deployment 详情失败')
    } finally {
      setFetching(false)
    }
  }

  const fetchAssociatedServices = async () => {
    try {
      const res = await getDeploymentServices(clusterId, namespace, name)
      if (res.code === 0) {
        setAssociatedServices(res.data || [])
        // 如果有关联的Service，填充到表单
        if (res.data && res.data.length > 0) {
          const svc = res.data[0]
          form.setFieldsValue({
            svc_name: svc.name,
            svc_type: svc.type || 'ClusterIP',
            svc_ports: svc.ports || [],
          })
        }
      }
    } catch (error) {
      console.error('Failed to fetch associated services:', error)
    }
  }

  const populateForm = (data: DeploymentDetail) => {
    // 转换标签为字符串格式
    const labelsStr = data.labels ? Object.entries(data.labels)
      .filter(([k]) => k !== 'app')
      .map(([k, v]) => `${k}=${v}`)
      .join(',') : ''

    const nodeSelectorStr = data.node_selector ? Object.entries(data.node_selector)
      .map(([k, v]) => `${k}=${v}`)
      .join(',') : ''

    form.setFieldsValue({
      replicas: data.replicas,
      labels: labelsStr,
      max_surge: data.max_surge,
      max_unavailable: data.max_unavailable,
      containers: (data.containers || []).map(c => ({
        name: c.name,
        image: c.image,
        image_pull_policy: c.image_pull_policy || 'IfNotPresent',
        command: c.command?.join(' '),
        args: c.args?.join(' '),
        resources: c.resources || {},
        ports: c.ports || [],
        env: c.env || [],
        volume_mounts: c.volume_mounts || [],
        liveness_probe_enabled: !!c.liveness_probe,
        liveness_probe_type: c.liveness_probe?.probe_type,
        liveness_http_path: c.liveness_probe?.http_get?.path,
        liveness_http_port: c.liveness_probe?.http_get?.port,
        readiness_probe_enabled: !!c.readiness_probe,
        readiness_probe_type: c.readiness_probe?.probe_type,
        readiness_http_path: c.readiness_probe?.http_get?.path,
        readiness_http_port: c.readiness_probe?.http_get?.port,
      })),
      volumes: data.volumes || [],
      node_selector: nodeSelectorStr,
      tolerations: data.tolerations || [],
      service_account_name: data.service_account_name,
      dns_policy: data.dns_policy || 'ClusterFirst',
      host_network: data.host_network,
      restart_policy: data.restart_policy || 'Always',
      termination_grace_period: data.termination_grace_period,
    })
  }

  const handleSubmit = async (values: any) => {
    setLoading(true)
    try {
      const convertTagsToObject = (tags: string): Record<string, string> | undefined => {
        if (!tags) return undefined
        const result: Record<string, string> = {}
        tags.split(',').forEach((pair: string) => {
          const [key, value] = pair.split('=')
          if (key && value) {
            result[key.trim()] = value.trim()
          }
        })
        return Object.keys(result).length > 0 ? result : undefined
      }

      const request: any = {
        replicas: values.replicas,
        labels: convertTagsToObject(values.labels),
        max_surge: values.max_surge,
        max_unavailable: values.max_unavailable,
        containers: (values.containers || []).map((c: any) => ({
          name: c.name,
          image: c.image,
          image_pull_policy: c.image_pull_policy,
          command: c.command ? c.command.split(' ') : undefined,
          args: c.args ? c.args.split(' ') : undefined,
          resources: c.resources ? {
            cpu_request: c.resources.cpu_request,
            cpu_limit: c.resources.cpu_limit,
            memory_request: c.resources.memory_request,
            memory_limit: c.resources.memory_limit,
          } : undefined,
          ports: (c.ports || []).map((p: any) => ({
            container_port: p.container_port,
            protocol: p.protocol,
            name: p.name,
          })),
          env: (c.env || []).map((e: any) => ({
            name: e.name,
            value: e.value,
          })),
          volume_mounts: (c.volume_mounts || []).map((m: any) => ({
            name: m.name,
            mount_path: m.mount_path,
            read_only: m.read_only,
          })),
          liveness_probe: c.liveness_probe_enabled ? {
            probe_type: c.liveness_probe_type || 'http',
            http_get: c.liveness_probe_type === 'http' ? {
              path: c.liveness_http_path || '/',
              port: c.liveness_http_port || 8080,
            } : undefined,
            tcp_socket: c.liveness_probe_type === 'tcp' ? {
              port: c.liveness_tcp_port || 8080,
            } : undefined,
            initial_delay_seconds: c.liveness_initial_delay || 0,
            period_seconds: c.liveness_period || 10,
          } : undefined,
          readiness_probe: c.readiness_probe_enabled ? {
            probe_type: c.readiness_probe_type || 'http',
            http_get: c.readiness_probe_type === 'http' ? {
              path: c.readiness_http_path || '/',
              port: c.readiness_http_port || 8080,
            } : undefined,
            tcp_socket: c.readiness_probe_type === 'tcp' ? {
              port: c.readiness_tcp_port || 8080,
            } : undefined,
            initial_delay_seconds: c.readiness_initial_delay || 0,
            period_seconds: c.readiness_period || 10,
          } : undefined,
        })),
        volumes: (values.volumes || []).map((v: any) => ({
          name: v.name,
          type: v.type,
          host_path: v.type === 'hostpath' ? { path: v.host_path } : undefined,
          pvc_name: v.type === 'pvc' ? v.pvc_name : undefined,
          configmap: v.type === 'configmap' ? v.configmap_name : undefined,
          secret: v.type === 'secret' ? v.secret_name : undefined,
          empty_dir: v.type === 'emptydir' ? { size_limit: v.empty_dir_size } : undefined,
        })),
        scheduling: {
          node_selector: convertTagsToObject(values.node_selector),
          tolerations: (values.tolerations || []).map((t: any) => ({
            key: t.key,
            operator: t.operator,
            value: t.value,
            effect: t.effect,
          })),
        },
        advanced: {
          service_account_name: values.service_account_name,
          termination_grace_period_seconds: values.termination_grace_period,
          dns_policy: values.dns_policy,
          host_network: values.host_network,
          restart_policy: values.restart_policy,
        },
      }

      // 添加Service配置
      if (values.svc_enable) {
        request.service = {
          name: values.svc_name || '',
          create: !values.svc_name && values.svc_create,
          type: values.svc_type || 'ClusterIP',
          ports: (values.svc_ports || []).map((p: any) => ({
            name: p.name,
            port: p.port,
            target_port: p.target_port,
            node_port: p.node_port,
            protocol: p.protocol || 'TCP',
          })),
        }
      }

      await updateDeployment(clusterId, namespace, name, request)
      message.success('更新成功')
      onSuccess()
      onClose()
    } catch (error) {
      console.error('Update failed:', error)
      message.error('更新失败')
    } finally {
      setLoading(false)
    }
  }

  const items = [
    {
      key: 'basic',
      label: '基本信息',
      children: (
        <>
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item label="名称">
                <Input value={name} disabled />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="命名空间">
                <Input value={namespace} disabled />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="replicas" label="副本数">
                <InputNumber min={0} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="labels" label="标签">
            <Input placeholder="例如: env=prod,team=backend" />
          </Form.Item>
          <Divider orientation="left">滚动更新策略</Divider>
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item name="max_surge" label="最大超出数">
                <InputNumber min={0} style={{ width: '100%' }} placeholder="默认 1" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="max_unavailable" label="最大不可用数">
                <InputNumber min={0} style={{ width: '100%' }} placeholder="默认 0" />
              </Form.Item>
            </Col>
          </Row>
        </>
      ),
    },
    {
      key: 'container',
      label: '容器配置',
      children: (
        <Form.List name="containers">
          {(fields, { add, remove }) => (
            <>
              {fields.map(({ key, name, ...restField }) => (
                <Card
                  key={key}
                  size="small"
                  title={`容器 ${name + 1}`}
                  extra={
                    fields.length > 1 && (
                      <Button type="link" danger onClick={() => remove(name)}>
                        删除
                      </Button>
                    )
                  }
                  style={{ marginBottom: 16 }}
                >
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'name']}
                        label="容器名称"
                        rules={[{ required: true }]}
                      >
                        <Input placeholder="例如: nginx" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'image']}
                        label="镜像"
                        rules={[{ required: true }]}
                      >
                        <Input placeholder="例如: nginx:latest" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'image_pull_policy']}
                        label="拉取策略"
                      >
                        <Select
                          options={[
                            { label: 'IfNotPresent', value: 'IfNotPresent' },
                            { label: 'Always', value: 'Always' },
                            { label: 'Never', value: 'Never' },
                          ]}
                        />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Divider orientation="left" plain>资源配额</Divider>
                  <Row gutter={24}>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'resources', 'cpu_request']} label="CPU 请求">
                        <Input placeholder="100m" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'resources', 'cpu_limit']} label="CPU 限制">
                        <Input placeholder="500m" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'resources', 'memory_request']} label="内存 请求">
                        <Input placeholder="128Mi" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item {...restField} name={[name, 'resources', 'memory_limit']} label="内存 限制">
                        <Input placeholder="256Mi" />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Divider orientation="left" plain>端口配置</Divider>
                  <Form.List name={[name, 'ports']}>
                    {(portFields, { add: addPort, remove: removePort }) => (
                      <>
                        {portFields.map(({ key: portKey, name: portName, ...portRest }) => (
                          <Space key={portKey} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                            <Form.Item {...portRest} name={[portName, 'name']} style={{ marginBottom: 0 }}>
                              <Input placeholder="端口名称" />
                            </Form.Item>
                            <Form.Item {...portRest} name={[portName, 'container_port']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <InputNumber placeholder="端口号" min={1} max={65535} />
                            </Form.Item>
                            <Form.Item {...portRest} name={[portName, 'protocol']} initialValue="TCP" style={{ marginBottom: 0 }}>
                              <Select style={{ width: 80 }}>
                                <Select.Option value="TCP">TCP</Select.Option>
                                <Select.Option value="UDP">UDP</Select.Option>
                              </Select>
                            </Form.Item>
                            <MinusCircleOutlined onClick={() => removePort(portName)} />
                          </Space>
                        ))}
                        <Button type="dashed" onClick={() => addPort()} block icon={<PlusOutlined />}>
                          添加端口
                        </Button>
                      </>
                    )}
                  </Form.List>

                  <Divider orientation="left" plain>环境变量</Divider>
                  <Form.List name={[name, 'env']}>
                    {(envFields, { add: addEnv, remove: removeEnv }) => (
                      <>
                        {envFields.map(({ key: envKey, name: envName, ...envRest }) => (
                          <Space key={envKey} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                            <Form.Item {...envRest} name={[envName, 'name']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <Input placeholder="变量名" />
                            </Form.Item>
                            <Form.Item {...envRest} name={[envName, 'value']} style={{ marginBottom: 0 }}>
                              <Input placeholder="变量值" />
                            </Form.Item>
                            <MinusCircleOutlined onClick={() => removeEnv(envName)} />
                          </Space>
                        ))}
                        <Button type="dashed" onClick={() => addEnv()} block icon={<PlusOutlined />}>
                          添加环境变量
                        </Button>
                      </>
                    )}
                  </Form.List>

                  <Divider orientation="left" plain>健康检查</Divider>
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item {...restField} name={[name, 'liveness_probe_enabled']} label="存活探针" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item {...restField} name={[name, 'readiness_probe_enabled']} label="就绪探针" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                  </Row>
                </Card>
              ))}
              <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                添加容器
              </Button>
            </>
          )}
        </Form.List>
      ),
    },
    {
      key: 'storage',
      label: '数据存储',
      children: (
        <Form.List name="volumes">
          {(fields, { add, remove }) => (
            <>
              {fields.map(({ key, name, ...restField }) => (
                <Card
                  key={key}
                  size="small"
                  title={`存储卷 ${name + 1}`}
                  extra={<Button type="link" danger onClick={() => remove(name)}>删除</Button>}
                  style={{ marginBottom: 16 }}
                >
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item {...restField} name={[name, 'name']} label="卷名称" rules={[{ required: true }]}>
                        <Input placeholder="例如: data" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item {...restField} name={[name, 'type']} label="类型" rules={[{ required: true }]}>
                        <Select
                          options={[
                            { label: 'EmptyDir', value: 'emptydir' },
                            { label: 'HostPath', value: 'hostpath' },
                            { label: 'PVC', value: 'pvc' },
                            { label: 'ConfigMap', value: 'configmap' },
                            { label: 'Secret', value: 'secret' },
                          ]}
                        />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        noStyle
                        shouldUpdate={(prevValues, curValues) => {
                          return prevValues.volumes?.[name]?.type !== curValues.volumes?.[name]?.type
                        }}
                      >
                        {({ getFieldValue }) => {
                          const volumeType = getFieldValue(['volumes', name, 'type'])
                          if (volumeType === 'hostpath') {
                            return <Form.Item {...restField} name={[name, 'host_path']} label="HostPath" rules={[{ required: true }]}><Input placeholder="/data/path" /></Form.Item>
                          }
                          if (volumeType === 'pvc') {
                            return <Form.Item {...restField} name={[name, 'pvc_name']} label="PVC名称" rules={[{ required: true }]}><Input placeholder="PVC名称" /></Form.Item>
                          }
                          if (volumeType === 'configmap') {
                            return <Form.Item {...restField} name={[name, 'configmap_name']} label="ConfigMap名称" rules={[{ required: true }]}><Input placeholder="ConfigMap名称" /></Form.Item>
                          }
                          if (volumeType === 'secret') {
                            return <Form.Item {...restField} name={[name, 'secret_name']} label="Secret名称" rules={[{ required: true }]}><Input placeholder="Secret名称" /></Form.Item>
                          }
                          if (volumeType === 'emptydir') {
                            return <Form.Item {...restField} name={[name, 'empty_dir_size']} label="大小限制"><Input placeholder="例如: 1Gi" /></Form.Item>
                          }
                          return null
                        }}
                      </Form.Item>
                    </Col>
                  </Row>
                </Card>
              ))}
              <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                添加存储卷
              </Button>
            </>
          )}
        </Form.List>
      ),
    },
    {
      key: 'network',
      label: '网络配置',
      children: (
        <>
          {associatedServices.length > 0 && (
            <div style={{ marginBottom: 16 }}>
              <Tag color="blue">已关联 Service: {associatedServices.map(s => s.name).join(', ')}</Tag>
            </div>
          )}
          <Form.Item name="svc_enable" label="配置 Service" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prevValues, curValues) => prevValues.svc_enable !== curValues.svc_enable}
          >
            {({ getFieldValue }) => {
              if (!getFieldValue('svc_enable')) return null
              return (
                <Card size="small" style={{ marginTop: 16 }}>
                  {associatedServices.length === 0 && (
                    <Form.Item name="svc_create" label="创建新 Service" valuePropName="checked" initialValue={true}>
                      <Switch />
                    </Form.Item>
                  )}
                  {associatedServices.length > 0 && (
                    <Form.Item name="svc_name" label="Service 名称">
                      <Input disabled />
                    </Form.Item>
                  )}
                  <Form.Item name="svc_type" label="类型" initialValue="ClusterIP">
                    <Select
                      options={[
                        { label: 'ClusterIP', value: 'ClusterIP' },
                        { label: 'NodePort', value: 'NodePort' },
                        { label: 'LoadBalancer', value: 'LoadBalancer' },
                      ]}
                    />
                  </Form.Item>
                  <Form.List name="svc_ports">
                    {(fields, { add, remove }) => (
                      <>
                        <Text strong>端口映射</Text>
                        {fields.map(({ key, name, ...restField }) => (
                          <Space key={key} style={{ display: 'flex', marginTop: 8 }} align="baseline">
                            <Form.Item {...restField} name={[name, 'name']} style={{ marginBottom: 0 }}>
                              <Input placeholder="名称" />
                            </Form.Item>
                            <Form.Item {...restField} name={[name, 'port']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <InputNumber placeholder="服务端口" min={1} max={65535} />
                            </Form.Item>
                            <Form.Item {...restField} name={[name, 'target_port']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <InputNumber placeholder="容器端口" min={1} max={65535} />
                            </Form.Item>
                            <Form.Item
                              noStyle
                              shouldUpdate={(prevValues, curValues) => prevValues.svc_type !== curValues.svc_type}
                            >
                              {({ getFieldValue }) => {
                                if (getFieldValue('svc_type') === 'NodePort') {
                                  return (
                                    <Form.Item {...restField} name={[name, 'node_port']} style={{ marginBottom: 0 }}>
                                      <InputNumber placeholder="NodePort" min={30000} max={32767} />
                                    </Form.Item>
                                  )
                                }
                                return null
                              }}
                            </Form.Item>
                            <Form.Item {...restField} name={[name, 'protocol']} initialValue="TCP" style={{ marginBottom: 0 }}>
                              <Select style={{ width: 80 }}>
                                <Select.Option value="TCP">TCP</Select.Option>
                                <Select.Option value="UDP">UDP</Select.Option>
                              </Select>
                            </Form.Item>
                            <MinusCircleOutlined onClick={() => remove(name)} />
                          </Space>
                        ))}
                        <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />} style={{ marginTop: 8 }}>
                          添加端口映射
                        </Button>
                      </>
                    )}
                  </Form.List>
                </Card>
              )
            }}
          </Form.Item>
        </>
      ),
    },
    {
      key: 'scheduling',
      label: '调度配置',
      children: (
        <>
          <Form.Item name="node_selector" label="节点选择器">
            <Input placeholder="例如: kubernetes.io/os=linux" />
          </Form.Item>

          <Divider orientation="left">污点容忍</Divider>
          <Form.List name="tolerations">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                    <Form.Item {...restField} name={[name, 'key']} style={{ marginBottom: 0 }}>
                      <Input placeholder="Key" />
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'operator']} initialValue="Equal" style={{ marginBottom: 0 }}>
                      <Select style={{ width: 100 }}>
                        <Select.Option value="Equal">Equal</Select.Option>
                        <Select.Option value="Exists">Exists</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'value']} style={{ marginBottom: 0 }}>
                      <Input placeholder="Value" />
                    </Form.Item>
                    <Form.Item {...restField} name={[name, 'effect']} style={{ marginBottom: 0 }}>
                      <Select style={{ width: 150 }} placeholder="Effect">
                        <Select.Option value="NoSchedule">NoSchedule</Select.Option>
                        <Select.Option value="PreferNoSchedule">PreferNoSchedule</Select.Option>
                        <Select.Option value="NoExecute">NoExecute</Select.Option>
                      </Select>
                    </Form.Item>
                    <MinusCircleOutlined onClick={() => remove(name)} />
                  </Space>
                ))}
                <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                  添加容忍规则
                </Button>
              </>
            )}
          </Form.List>
        </>
      ),
    },
    {
      key: 'advanced',
      label: '高级配置',
      children: (
        <>
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item name="service_account_name" label="ServiceAccount">
                <Input placeholder="默认" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="restart_policy" label="重启策略">
                <Select
                  options={[
                    { label: 'Always', value: 'Always' },
                    { label: 'OnFailure', value: 'OnFailure' },
                    { label: 'Never', value: 'Never' },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="dns_policy" label="DNS 策略">
                <Select
                  options={[
                    { label: 'ClusterFirst', value: 'ClusterFirst' },
                    { label: 'Default', value: 'Default' },
                    { label: 'ClusterFirstWithHostNet', value: 'ClusterFirstWithHostNet' },
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item name="termination_grace_period" label="优雅终止时间(秒)">
                <InputNumber min={0} style={{ width: '100%' }} placeholder="默认 30" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="host_network" label="使用主机网络" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>
        </>
      ),
    },
  ]

  return (
    <Modal
      title={`编辑 Deployment: ${name}`}
      open={visible}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={loading}
      width={1000}
      styles={{ body: { maxHeight: '70vh', overflow: 'auto' } }}
    >
      {fetching ? (
        <div style={{ textAlign: 'center', padding: 50 }}>
          <Spin size="large" />
        </div>
      ) : (
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Tabs items={items} />
        </Form>
      )}
    </Modal>
  )
}

export default EditDeploymentModal
