import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Typography, Select, Input, Tooltip, Modal, Drawer, Form, message } from 'antd'
import {
  SyncOutlined,
  DeleteOutlined,
  FileTextOutlined,
  CodeOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getPods, createPod, getPodLogs, deletePod, Pod, getNamespaces } from '../../api/workload'
import { getClusterList, Cluster } from '../../api/cluster'
import PodTerminal from '../../components/PodTerminal'

const { Title, Text } = Typography

const WorkloadPods: React.FC = () => {
  const [pods, setPods] = useState<Pod[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [logDrawerVisible, setLogDrawerVisible] = useState(false)
  const [terminalVisible, setTerminalVisible] = useState(false)
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [selectedPod, setSelectedPod] = useState<Pod | null>(null)
  const [logs, setLogs] = useState<string>('')
  const [logsLoading, setLogsLoading] = useState(false)
  const [createForm] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchNamespaces()
      fetchPods()
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
      const res = await getNamespaces(selectedCluster)
      setNamespaces(res.data || [])
    } catch (error) {
      console.error('Failed to fetch namespaces:', error)
    }
  }

  const fetchPods = async () => {
    setLoading(true)
    try {
      const res = await getPods(selectedCluster, selectedNamespace || undefined)
      setPods(res.data || [])
    } catch (error) {
      console.error('Failed to fetch pods:', error)
    } finally {
      setLoading(false)
    }
  }

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string }> = {
      Running: { color: 'success' },
      Pending: { color: 'warning' },
      Succeeded: { color: 'default' },
      Failed: { color: 'error' },
      CrashLoopBackOff: { color: 'error' },
      Unknown: { color: 'default' },
    }
    const config = statusMap[status] || statusMap.Unknown
    return <Tag color={config.color}>{status}</Tag>
  }

  const handleViewLogs = async (record: Pod) => {
    setSelectedPod(record)
    setLogDrawerVisible(true)
    setLogsLoading(true)
    try {
      const res = await getPodLogs(selectedCluster, record.namespace, record.name, 200)
      setLogs(typeof res === 'string' ? res : JSON.stringify(res, null, 2))
    } catch (error) {
      setLogs('获取日志失败')
      console.error('Failed to fetch logs:', error)
    } finally {
      setLogsLoading(false)
    }
  }

  const handleDelete = (record: Pod) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 Pod "${record.name}" 吗？`,
      onOk: async () => {
        try {
          await deletePod(selectedCluster, record.namespace, record.name)
          message.success('删除成功')
          fetchPods()
        } catch (error) {
          console.error('Delete failed:', error)
        }
      },
    })
  }

  const handleCreate = async (values: any) => {
    try {
      await createPod(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
        ports: values.ports ? [{ containerPort: parseInt(values.ports) }] : undefined,
      })
      message.success('创建成功')
      setCreateModalVisible(false)
      createForm.resetFields()
      fetchPods()
    } catch (error) {
      console.error('Create failed:', error)
    }
  }

  const columns: ColumnsType<Pod> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
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
      render: (status) => getStatusTag(status),
    },
    {
      title: '就绪',
      dataIndex: 'ready',
      key: 'ready',
    },
    {
      title: '重启',
      dataIndex: 'restarts',
      key: 'restarts',
      render: (restarts) => (
        <Text type={restarts > 0 ? 'danger' : undefined}>{restarts}</Text>
      ),
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '节点',
      dataIndex: 'node',
      key: 'node',
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
          <Tooltip title="日志">
            <Button type="link" icon={<FileTextOutlined />} onClick={() => handleViewLogs(record)} />
          </Tooltip>
          <Tooltip title="终端">
            <Button type="link" icon={<CodeOutlined />} onClick={() => {
              setSelectedPod(record)
              setTerminalVisible(true)
            }} />
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
        <Title level={4}>Pod</Title>
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
          <Button icon={<SyncOutlined />} onClick={fetchPods}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
            创建
          </Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={pods} rowKey="name" loading={loading} />
      </Card>

      <Modal
        title="创建 Pod"
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
            <Input placeholder="请输入 Pod 名称" />
          </Form.Item>
          <Form.Item
            name="image"
            label="镜像"
            rules={[{ required: true, message: '请输入镜像' }]}
          >
            <Input placeholder="例如: nginx:latest" />
          </Form.Item>
          <Form.Item name="ports" label="容器端口">
            <Input placeholder="例如: 80" />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={`日志 - ${selectedPod?.name}`}
        placement="right"
        width={800}
        onClose={() => setLogDrawerVisible(false)}
        open={logDrawerVisible}
        loading={logsLoading}
        extra={
          <Space>
            <Button icon={<SyncOutlined />} onClick={() => selectedPod && handleViewLogs(selectedPod)}>
              刷新
            </Button>
          </Space>
        }
      >
        <pre
          style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 8,
            height: 'calc(100vh - 200px)',
            overflow: 'auto',
            fontSize: 13,
            fontFamily: 'Consolas, Monaco, monospace',
          }}
        >
          {logs}
        </pre>
      </Drawer>

      {selectedPod && (
        <PodTerminal
          visible={terminalVisible}
          onClose={() => {
            setTerminalVisible(false)
            setSelectedPod(null)
          }}
          clusterId={selectedCluster}
          namespace={selectedPod.namespace}
          podName={selectedPod.name}
        />
      )}
    </div>
  )
}

export default WorkloadPods
