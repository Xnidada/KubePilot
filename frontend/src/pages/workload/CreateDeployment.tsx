import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Card,
  Form,
  Input,
  Select,
  Button,
  Space,
  Typography,
  Tabs,
  InputNumber,
  Switch,
  Divider,
  message,
  Row,
  Col,
} from 'antd'
import {
  PlusOutlined,
  MinusCircleOutlined,
  SaveOutlined,
  ArrowLeftOutlined,
} from '@ant-design/icons'
import { createEnterpriseDeployment, getNamespaceNames, EnterpriseDeploymentRequest } from '../../api/workload'
import { getClusterList, Cluster } from '../../api/cluster'
import { getStorageClasses, StorageClass } from '../../api/storage'

const { Title, Text } = Typography
const { TextArea } = Input

const CreateDeployment: React.FC = () => {
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [, setStorageClasses] = useState<StorageClass[]>([])

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchStorageClasses()
    }
  }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data && res.data.length > 0) {
        setSelectedCluster(res.data[0].id)
      }
    } catch (error) {
      console.error('Failed to fetch clusters:', error)
    }
  }

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(selectedCluster)
      setNamespaces(res.data || [])
    } catch (error) {
      console.error('Failed to fetch namespaces:', error)
    }
  }

  const fetchStorageClasses = async () => {
    try {
      const res = await getStorageClasses(selectedCluster)
      setStorageClasses(res.data || [])
    } catch (error) {
      console.error('Failed to fetch storage classes:', error)
    }
  }

  const handleSubmit = async (values: any) => {
    setLoading(true)
    try {
      const request: EnterpriseDeploymentRequest = {
        name: values.name,
        namespace: values.namespace,
        description: values.description,
        labels: convertTagsToObject(values.labels),
        replicas: values.replicas || 1,
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
        network: values.create_service ? {
          create_service: true,
          service_config: {
            name: values.service_name,
            type: values.service_type || 'ClusterIP',
            ports: (values.service_ports || []).map((p: any) => ({
              port: p.port,
              target_port: p.target_port,
              protocol: p.protocol,
              name: p.name,
            })),
          },
        } : undefined,
        advanced: {
          service_account_name: values.service_account_name,
          termination_grace_period_seconds: values.termination_grace_period,
          dns_policy: values.dns_policy,
          host_network: values.host_network,
          restart_policy: values.restart_policy,
          image_pull_secrets: values.image_pull_secrets ? values.image_pull_secrets.split(',').map((s: string) => s.trim()) : undefined,
        },
      }

      await createEnterpriseDeployment(selectedCluster, request)
      message.success('Deployment 创建成功')
      navigate('/workloads/deployments')
    } catch (error) {
      console.error('Create failed:', error)
      message.error('创建失败')
    } finally {
      setLoading(false)
    }
  }

  const convertTagsToObject = (tags: string): Record<string, string> | undefined => {
    if (!tags) return undefined
    const result: Record<string, string> = {}
    tags.split(',').forEach(pair => {
      const [key, value] = pair.split('=')
      if (key && value) {
        result[key.trim()] = value.trim()
      }
    })
    return Object.keys(result).length > 0 ? result : undefined
  }

  const items = [
    {
      key: 'basic',
      label: '基本信息',
      children: (
        <>
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                name="cluster"
                label="集群"
              >
                <Select
                  value={selectedCluster}
                  onChange={setSelectedCluster}
                  options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="namespace"
                label="命名空间"
                rules={[{ required: true, message: '请选择命名空间' }]}
              >
                <Select
                  placeholder="选择命名空间"
                  options={namespaces.map(ns => ({ label: ns, value: ns }))}
                />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                name="name"
                label="名称"
                rules={[{ required: true, message: '请输入名称' }]}
              >
                <Input placeholder="请输入 Deployment 名称" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="replicas" label="副本数" initialValue={1}>
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="description" label="描述">
            <TextArea rows={2} placeholder="请输入描述" />
          </Form.Item>
          <Form.Item name="labels" label="标签">
            <Input placeholder="例如: app=nginx,env=prod" />
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
                        rules={[{ required: true, message: '请输入容器名称' }]}
                      >
                        <Input placeholder="例如: nginx" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'image']}
                        label="镜像"
                        rules={[{ required: true, message: '请输入镜像' }]}
                      >
                        <Input placeholder="例如: nginx:latest" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'image_pull_policy']}
                        label="拉取策略"
                        initialValue="IfNotPresent"
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
                      <Form.Item
                        {...restField}
                        name={[name, 'resources', 'cpu_request']}
                        label="CPU 请求"
                      >
                        <Input placeholder="例如: 100m" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'resources', 'cpu_limit']}
                        label="CPU 限制"
                      >
                        <Input placeholder="例如: 500m" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'resources', 'memory_request']}
                        label="内存 请求"
                      >
                        <Input placeholder="例如: 128Mi" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'resources', 'memory_limit']}
                        label="内存 限制"
                      >
                        <Input placeholder="例如: 256Mi" />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Divider orientation="left" plain>端口配置</Divider>
                  <Form.List name={[name, 'ports']}>
                    {(portFields, { add: addPort, remove: removePort }) => (
                      <>
                        {portFields.map(({ key: portKey, name: portName, ...portRest }) => (
                          <Space key={portKey} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                            <Form.Item
                              {...portRest}
                              name={[portName, 'name']}
                              style={{ marginBottom: 0 }}
                            >
                              <Input placeholder="端口名称" />
                            </Form.Item>
                            <Form.Item
                              {...portRest}
                              name={[portName, 'container_port']}
                              rules={[{ required: true }]}
                              style={{ marginBottom: 0 }}
                            >
                              <InputNumber placeholder="端口号" min={1} max={65535} />
                            </Form.Item>
                            <Form.Item
                              {...portRest}
                              name={[portName, 'protocol']}
                              initialValue="TCP"
                              style={{ marginBottom: 0 }}
                            >
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
                            <Form.Item
                              {...envRest}
                              name={[envName, 'name']}
                              rules={[{ required: true }]}
                              style={{ marginBottom: 0 }}
                            >
                              <Input placeholder="变量名" />
                            </Form.Item>
                            <Form.Item
                              {...envRest}
                              name={[envName, 'value']}
                              style={{ marginBottom: 0 }}
                            >
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
                      <Form.Item
                        {...restField}
                        name={[name, 'liveness_probe_enabled']}
                        label="存活探针"
                        valuePropName="checked"
                      >
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        {...restField}
                        name={[name, 'readiness_probe_enabled']}
                        label="就绪探针"
                        valuePropName="checked"
                      >
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
        <>
          <Form.List name="volumes">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <Card
                    key={key}
                    size="small"
                    title={`存储卷 ${name + 1}`}
                    extra={
                      <Button type="link" danger onClick={() => remove(name)}>
                        删除
                      </Button>
                    }
                    style={{ marginBottom: 16 }}
                  >
                    <Row gutter={24}>
                      <Col span={8}>
                        <Form.Item
                          {...restField}
                          name={[name, 'name']}
                          label="卷名称"
                          rules={[{ required: true }]}
                        >
                          <Input placeholder="例如: data" />
                        </Form.Item>
                      </Col>
                      <Col span={8}>
                        <Form.Item
                          {...restField}
                          name={[name, 'type']}
                          label="类型"
                          rules={[{ required: true }]}
                        >
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
                              return (
                                <Form.Item
                                  {...restField}
                                  name={[name, 'host_path']}
                                  label="HostPath"
                                  rules={[{ required: true }]}
                                >
                                  <Input placeholder="/data/path" />
                                </Form.Item>
                              )
                            }
                            if (volumeType === 'pvc') {
                              return (
                                <Form.Item
                                  {...restField}
                                  name={[name, 'pvc_name']}
                                  label="PVC名称"
                                  rules={[{ required: true }]}
                                >
                                  <Input placeholder="PVC名称" />
                                </Form.Item>
                              )
                            }
                            if (volumeType === 'configmap') {
                              return (
                                <Form.Item
                                  {...restField}
                                  name={[name, 'configmap_name']}
                                  label="ConfigMap名称"
                                  rules={[{ required: true }]}
                                >
                                  <Input placeholder="ConfigMap名称" />
                                </Form.Item>
                              )
                            }
                            if (volumeType === 'secret') {
                              return (
                                <Form.Item
                                  {...restField}
                                  name={[name, 'secret_name']}
                                  label="Secret名称"
                                  rules={[{ required: true }]}
                                >
                                  <Input placeholder="Secret名称" />
                                </Form.Item>
                              )
                            }
                            if (volumeType === 'emptydir') {
                              return (
                                <Form.Item
                                  {...restField}
                                  name={[name, 'empty_dir_size']}
                                  label="大小限制"
                                >
                                  <Input placeholder="例如: 1Gi" />
                                </Form.Item>
                              )
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

          <Divider orientation="left">容器挂载</Divider>
          <Form.Item name="container_volume_mounts" help="在容器配置中设置挂载路径">
            <Text type="secondary">请在"容器配置"标签页中为每个容器配置挂载路径</Text>
          </Form.Item>
        </>
      ),
    },
    {
      key: 'network',
      label: '网络配置',
      children: (
        <>
          <Form.Item name="create_service" label="创建 Service" valuePropName="checked">
            <Switch />
          </Form.Item>

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, curValues) => prevValues.create_service !== curValues.create_service}
          >
            {({ getFieldValue }) => {
              if (!getFieldValue('create_service')) return null

              return (
                <Card size="small" style={{ marginTop: 16 }}>
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item name="service_name" label="Service 名称">
                        <Input placeholder="默认与 Deployment 同名" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item name="service_type" label="类型" initialValue="ClusterIP">
                        <Select
                          options={[
                            { label: 'ClusterIP', value: 'ClusterIP' },
                            { label: 'NodePort', value: 'NodePort' },
                            { label: 'LoadBalancer', value: 'LoadBalancer' },
                          ]}
                        />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Form.List name="service_ports">
                    {(fields, { add, remove }) => (
                      <>
                        <Text strong>端口映射</Text>
                        {fields.map(({ key, name, ...restField }) => (
                          <Space key={key} style={{ display: 'flex', marginTop: 8 }} align="baseline">
                            <Form.Item
                              {...restField}
                              name={[name, 'name']}
                              style={{ marginBottom: 0 }}
                            >
                              <Input placeholder="名称" />
                            </Form.Item>
                            <Form.Item
                              {...restField}
                              name={[name, 'port']}
                              rules={[{ required: true }]}
                              style={{ marginBottom: 0 }}
                            >
                              <InputNumber placeholder="服务端口" min={1} max={65535} />
                            </Form.Item>
                            <Form.Item
                              {...restField}
                              name={[name, 'target_port']}
                              rules={[{ required: true }]}
                              style={{ marginBottom: 0 }}
                            >
                              <InputNumber placeholder="容器端口" min={1} max={65535} />
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
            <Input placeholder="例如: kubernetes.io/os=linux,node-role.kubernetes.io/worker=" />
          </Form.Item>

          <Divider orientation="left">污点容忍</Divider>
          <Form.List name="tolerations">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                    <Form.Item
                      {...restField}
                      name={[name, 'key']}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder="Key" />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'operator']}
                      initialValue="Equal"
                      style={{ marginBottom: 0 }}
                    >
                      <Select style={{ width: 100 }}>
                        <Select.Option value="Equal">Equal</Select.Option>
                        <Select.Option value="Exists">Exists</Select.Option>
                      </Select>
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'value']}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder="Value" />
                    </Form.Item>
                    <Form.Item
                      {...restField}
                      name={[name, 'effect']}
                      style={{ marginBottom: 0 }}
                    >
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
              <Form.Item name="restart_policy" label="重启策略" initialValue="Always">
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
              <Form.Item name="dns_policy" label="DNS 策略" initialValue="ClusterFirst">
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
          <Form.Item name="image_pull_secrets" label="镜像拉取密钥">
            <Input placeholder="多个密钥用逗号分隔" />
          </Form.Item>
        </>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/workloads/deployments')}>
            返回
          </Button>
          <Title level={4} style={{ margin: 0 }}>创建 Deployment</Title>
        </Space>
        <Space>
          <Button onClick={() => navigate('/workloads/deployments')}>取消</Button>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={loading}
            onClick={() => form.submit()}
          >
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{
            replicas: 1,
            containers: [{}],
            service_type: 'ClusterIP',
            restart_policy: 'Always',
            dns_policy: 'ClusterFirst',
          }}
        >
          <Tabs items={items} />
        </Form>
      </Card>
    </div>
  )
}

export default CreateDeployment
