import { useEffect, useState, useCallback } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Input, Tooltip, Modal, Form, message } from 'antd'
import {
  PlusOutlined,
  SyncOutlined,
  EditOutlined,
  DeleteOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getServices, createService, deleteService, Service, getNamespaceNames } from '../../api/workload'
import { getClusterList, Cluster } from '../../api/cluster'
import EditServiceModal from '../../components/EditServiceModal'
import StatusTag from '../../components/StatusTag'
import { usePolling, hasTerminatingResource } from '../../hooks/usePolling'

const { Title } = Typography

const WorkloadServices: React.FC = () => {
  const [services, setServices] = useState<Service[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [selectedService, setSelectedService] = useState<Service | null>(null)
  const [editInitialValues, setEditInitialValues] = useState<any>(null)
  const [createForm] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchServices()
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

  const fetchServices = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getServices(selectedCluster, selectedNamespace || undefined)
      setServices(res.data || [])
    } catch (error) {
      console.error('Failed to fetch services:', error)
    } finally {
      setLoading(false)
    }
  }, [selectedCluster, selectedNamespace])

  // 自动轮询：当有 Terminating 状态的资源时自动刷新
  usePolling(fetchServices, hasTerminatingResource(services), { interval: 3000 })

  const handleDelete = (record: Service) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 Service "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deleteService(selectedCluster, record.namespace, record.name)
          message.success('删除成功')
          fetchServices()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleCreate = async (values: any) => {
    try {
      const selector: Record<string, string> = {}
      if (values.selector) {
        values.selector.split(',').forEach((pair: string) => {
          const [key, value] = pair.split('=')
          if (key && value) {
            selector[key.trim()] = value.trim()
          }
        })
      }

      await createService(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        type: values.type || 'ClusterIP',
        selector,
        ports: [{
          port: parseInt(values.port),
          targetPort: parseInt(values.targetPort),
          protocol: values.protocol || 'TCP',
        }],
      })
      message.success('创建成功')
      setCreateModalVisible(false)
      createForm.resetFields()
      fetchServices()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const handleEdit = async (record: Service) => {
    setSelectedService(record)
    try {
      const token = localStorage.getItem('auth-storage')
      const authData = token ? JSON.parse(token) : null
      const authToken = authData?.state?.token

      const response = await fetch(
        `/api/v1/clusters/${selectedCluster}/workloads/services/${record.namespace}/${record.name}`,
        {
          headers: { 'Authorization': `Bearer ${authToken}` },
        }
      )
      const data = await response.json()
      if (data.code === 0) {
        setEditInitialValues(data.data)
        setEditModalVisible(true)
      }
    } catch (error) {
      console.error('Failed to fetch service details:', error)
    }
  }

  const columns: ColumnsType<Service> = [
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
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type) => <Tag>{type}</Tag>,
    },
    {
      title: 'Cluster IP',
      dataIndex: 'cluster_ip',
      key: 'cluster_ip',
    },
    {
      title: '端口',
      dataIndex: 'ports',
      key: 'ports',
    },
    {
      title: '年龄',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
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
        <Title level={4}>Service</Title>
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
          <Button icon={<SyncOutlined />} onClick={fetchServices}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={services} rowKey="name" loading={loading} />
      </Card>

      <Modal
        title="创建 Service"
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
            <Input placeholder="请输入 Service 名称" />
          </Form.Item>
          <Form.Item name="type" label="类型" initialValue="ClusterIP">
            <Select
              options={[
                { label: 'ClusterIP', value: 'ClusterIP' },
                { label: 'NodePort', value: 'NodePort' },
                { label: 'LoadBalancer', value: 'LoadBalancer' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="selector"
            label="选择器"
            rules={[{ required: true, message: '请输入选择器' }]}
          >
            <Input placeholder="例如: app=nginx" />
          </Form.Item>
          <Form.Item
            name="port"
            label="服务端口"
            rules={[{ required: true, message: '请输入服务端口' }]}
          >
            <Input type="number" placeholder="例如: 80" />
          </Form.Item>
          <Form.Item
            name="targetPort"
            label="目标端口"
            rules={[{ required: true, message: '请输入目标端口' }]}
          >
            <Input type="number" placeholder="例如: 80" />
          </Form.Item>
          <Form.Item name="protocol" label="协议" initialValue="TCP">
            <Select
              options={[
                { label: 'TCP', value: 'TCP' },
                { label: 'UDP', value: 'UDP' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      {selectedService && (
        <EditServiceModal
          visible={editModalVisible}
          onClose={() => {
            setEditModalVisible(false)
            setSelectedService(null)
            setEditInitialValues(null)
          }}
          onSuccess={fetchServices}
          clusterId={selectedCluster}
          namespace={selectedService.namespace}
          name={selectedService.name}
          initialValues={editInitialValues}
        />
      )}
    </div>
  )
}

export default WorkloadServices
