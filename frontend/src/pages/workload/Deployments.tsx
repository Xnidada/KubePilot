import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Table, Tag, Button, Space, Typography, Select, Input, Tooltip, Modal, Form, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  EditOutlined,
  DeleteOutlined,
  ColumnHeightOutlined,
  HistoryOutlined,
  SearchOutlined,
  CodeOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getDeployments, createDeployment, scaleDeployment, deleteDeployment, Deployment, getNamespaceNames } from '../../api/workload'
import { getClusterList, Cluster } from '../../api/cluster'
import EditDeploymentModal from '../../components/EditDeploymentModal'
import DeploymentHistoryModal from '../../components/DeploymentHistoryModal'
import YAMLEditor from '../../components/YAMLEditor'
import StatusTag from '../../components/StatusTag'
import { usePolling, hasTerminatingResource } from '../../hooks/usePolling'

const { Title } = Typography

const WorkloadDeployments: React.FC = () => {
  const navigate = useNavigate()
  const [deployments, setDeployments] = useState<Deployment[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [scaleModalVisible, setScaleModalVisible] = useState(false)
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [historyModalVisible, setHistoryModalVisible] = useState(false)
  const [yamlModalVisible, setYamlModalVisible] = useState(false)
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [selectedDeployment, setSelectedDeployment] = useState<Deployment | null>(null)
  const [form] = Form.useForm()
  const [createForm] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchDeployments()
    }
  }, [selectedCluster, selectedNamespace])

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

  const fetchDeployments = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getDeployments(selectedCluster, selectedNamespace || undefined)
      setDeployments(res.data || [])
    } catch (error) {
      console.error('Failed to fetch deployments:', error)
    } finally {
      setLoading(false)
    }
  }, [selectedCluster, selectedNamespace])

  // 自动轮询：当有 Terminating 状态的资源时自动刷新
  usePolling(fetchDeployments, hasTerminatingResource(deployments), { interval: 3000 })

  const handleScale = (record: Deployment) => {
    setSelectedDeployment(record)
    setScaleModalVisible(true)
    form.setFieldsValue({ replicas: parseInt(record.ready.split('/')[0]) })
  }

  const handleScaleSubmit = async (values: any) => {
    if (!selectedDeployment) return
    try {
      await scaleDeployment(selectedCluster, selectedDeployment.namespace, selectedDeployment.name, values.replicas)
      message.success(`伸缩 ${selectedDeployment.name} 到 ${values.replicas} 个副本`)
      setScaleModalVisible(false)
      form.resetFields()
      fetchDeployments()
    } catch (error) {
      console.error('Scale failed:', error)
    }
  }

  const handleDelete = (record: Deployment) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 Deployment "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deleteDeployment(selectedCluster, record.namespace, record.name)
          message.success('删除成功')
          fetchDeployments()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleEdit = (record: Deployment) => {
    setSelectedDeployment(record)
    setEditModalVisible(true)
  }

  const handleHistory = (record: Deployment) => {
    setSelectedDeployment(record)
    setHistoryModalVisible(true)
  }

  const handleCreate = async (values: any) => {
    try {
      await createDeployment(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
        replicas: values.replicas || 1,
        ports: values.ports ? [{ containerPort: parseInt(values.ports) }] : undefined,
      })
      message.success('创建成功')
      setCreateModalVisible(false)
      createForm.resetFields()
      fetchDeployments()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const columns: ColumnsType<Deployment> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      filteredValue: searchText ? [searchText] : null,
      onFilter: (value, record) => record.name.includes(value as string),
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => <StatusTag status={status || 'Active'} />,
    },
    {
      title: '就绪',
      dataIndex: 'ready',
      key: 'ready',
      render: (ready) => {
        const [current, desired] = ready.split('/')
        const isReady = current === desired
        return (
          <Tag color={isReady ? 'success' : 'warning'}>
            {ready}
          </Tag>
        )
      },
    },
    {
      title: '最新',
      dataIndex: 'up_to_date',
      key: 'up_to_date',
    },
    {
      title: '可用',
      dataIndex: 'available',
      key: 'available',
    },
    {
      title: '镜像',
      dataIndex: 'images',
      key: 'images',
      render: (images: string[]) => (
        <>
          {images.map((img: string) => (
            <Tag key={img}>{img}</Tag>
          ))}
        </>
      ),
    },
    {
      title: '年龄',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="伸缩">
            <Button type="link" icon={<ColumnHeightOutlined />} onClick={() => handleScale(record)} />
          </Tooltip>
          <Tooltip title="历史">
            <Button type="link" icon={<HistoryOutlined />} onClick={() => handleHistory(record)} />
          </Tooltip>
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          </Tooltip>
          <Tooltip title="YAML">
            <Button type="link" icon={<CodeOutlined />} onClick={() => { setSelectedDeployment(record); setYamlModalVisible(true) }} />
          </Tooltip>
          <Tooltip title="删除">
            <Button type="link" danger icon={<DeleteOutlined />} onClick={() => handleDelete(record)} />
          </Tooltip>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>Deployment</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Select
            value={selectedNamespace}
            onChange={setSelectedNamespace}
            style={{ width: 150 }}
            placeholder="所有命名空间"
            allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))}
          />
          <Input
            placeholder="搜索..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
          />
          <Button icon={<SyncOutlined />} onClick={fetchDeployments}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/workloads/deployments/create')}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={deployments} rowKey="name" loading={loading} />
      </Card>

      <Modal
        title="伸缩 Deployment"
        open={scaleModalVisible}
        onCancel={() => {
          setScaleModalVisible(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={handleScaleSubmit}>
          <Form.Item label="Deployment">
            <Input value={selectedDeployment?.name} disabled />
          </Form.Item>
          <Form.Item
            name="replicas"
            label="副本数"
            rules={[{ required: true, message: '请输入副本数' }]}
          >
            <Input type="number" min={0} max={100} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="创建 Deployment"
        open={createModalVisible}
        onCancel={() => {
          setCreateModalVisible(false)
          createForm.resetFields()
        }}
        onOk={() => createForm.submit()}
        width={600}
      >
        <Form form={createForm} layout="vertical" onFinish={handleCreate}>
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
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="请输入 Deployment 名称" />
          </Form.Item>
          <Form.Item
            name="image"
            label="镜像"
            rules={[{ required: true, message: '请输入镜像' }]}
          >
            <Input placeholder="例如: nginx:latest" />
          </Form.Item>
          <Form.Item name="replicas" label="副本数" initialValue={1}>
            <Input type="number" min={1} max={100} />
          </Form.Item>
          <Form.Item name="ports" label="容器端口">
            <Input placeholder="例如: 80" />
          </Form.Item>
        </Form>
      </Modal>

      {selectedDeployment && (
        <EditDeploymentModal
          visible={editModalVisible}
          onClose={() => {
            setEditModalVisible(false)
            setSelectedDeployment(null)
          }}
          onSuccess={fetchDeployments}
          clusterId={selectedCluster}
          namespace={selectedDeployment.namespace}
          name={selectedDeployment.name}
        />
      )}

      {selectedDeployment && (
        <DeploymentHistoryModal
          visible={historyModalVisible}
          onClose={() => {
            setHistoryModalVisible(false)
            setSelectedDeployment(null)
          }}
          onSuccess={fetchDeployments}
          clusterId={selectedCluster}
          namespace={selectedDeployment.namespace}
          name={selectedDeployment.name}
        />
      )}

      {selectedDeployment && (
        <YAMLEditor
          visible={yamlModalVisible}
          onClose={() => {
            setYamlModalVisible(false)
            setSelectedDeployment(null)
          }}
          onSuccess={fetchDeployments}
          clusterId={selectedCluster}
          resourceType="deployments"
          namespace={selectedDeployment.namespace}
          resourceName={selectedDeployment.name}
        />
      )}
    </div>
  )
}

export default WorkloadDeployments
