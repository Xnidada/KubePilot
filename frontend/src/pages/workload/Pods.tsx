import { useEffect, useState, useCallback } from 'react'
import { Card, Table, Button, Space, Typography, Select, Input, Tooltip, Modal, Form, message } from 'antd'
import {
  SyncOutlined,
  DeleteOutlined,
  FileTextOutlined,
  CodeOutlined,
  SearchOutlined,
  PlusOutlined,
  FolderOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getPods, createPod, deletePod, Pod, getNamespaceNames } from '../../api/workload'
import { getClusterList, Cluster } from '../../api/cluster'
import StatusTag from '../../components/StatusTag'
import LogViewer from '../../components/LogViewer'
import PodFileManager from '../../components/PodFileManager'
import Terminal from '../../components/Terminal'
import { usePolling, hasTerminatingResource } from '../../hooks/usePolling'

const { Title, Text } = Typography


const WorkloadPods: React.FC = () => {
  const [pods, setPods] = useState<Pod[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [logVisible, setLogVisible] = useState(false)
  const [fileManagerVisible, setFileManagerVisible] = useState(false)
  const [terminalVisible, setTerminalVisible] = useState(false)
  const [selectedPod, setSelectedPod] = useState<Pod | null>(null)
  const [createForm] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchPods() } }, [selectedCluster, selectedNamespace])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchNamespaces = async () => {
    try {
      const res = await getNamespaceNames(selectedCluster)
      setNamespaces(res.data || [])
    } catch (e) { console.error(e) }
  }

  const fetchPods = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getPods(selectedCluster, selectedNamespace || undefined)
      setPods(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }, [selectedCluster, selectedNamespace])

  // Auto-refresh when there are terminating pods
  usePolling(fetchPods, hasTerminatingResource(pods), { interval: 3000 })

  const handleCreate = async (values: any) => {
    try {
      await createPod(selectedCluster, {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
      })
      message.success('Pod 创建成功')
      setCreateModalVisible(false)
      createForm.resetFields()
      fetchPods()
    } catch (e) { message.error('创建失败') }
  }

  const handleDelete = async (record: Pod) => {
    try {
      await deletePod(selectedCluster, record.namespace, record.name)
      message.success('删除成功')
      fetchPods()
    } catch (e) { console.error(e) }
  }

  const columns: ColumnsType<Pod> = [
    {
      title: '名称', dataIndex: 'name', key: 'name', ellipsis: true,
      filteredValue: searchText ? [searchText] : null,
      onFilter: (v, r) => r.name.includes(v as string),
    },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (status) => <StatusTag status={status} />,
    },
    { title: '就绪', dataIndex: 'ready', key: 'ready' },
    {
      title: '重启', dataIndex: 'restarts', key: 'restarts',
      render: (r) => <Text type={r > 0 ? 'danger' : undefined}>{r}</Text>,
    },
    { title: 'IP', dataIndex: 'ip', key: 'ip' },
    { title: '节点', dataIndex: 'node', key: 'node' },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 200,
      render: (_, record) => (
        <Space size="small">
          <Tooltip title="日志">
            <Button type="link" icon={<FileTextOutlined />} onClick={() => { setSelectedPod(record); setLogVisible(true) }} />
          </Tooltip>
          <Tooltip title="文件管理">
            <Button type="link" icon={<FolderOutlined />} onClick={() => { setSelectedPod(record); setFileManagerVisible(true) }} />
          </Tooltip>
          <Tooltip title="终端">
            <Button type="link" icon={<CodeOutlined />} onClick={() => { setSelectedPod(record); setTerminalVisible(true) }} />
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
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchPods}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>创建</Button>
        </Space>
      </div>

      <Card>
        <Table columns={columns} dataSource={pods} rowKey={(r) => `${r.namespace}/${r.name}`} loading={loading} />
      </Card>

      {/* Create Pod Modal */}
      <Modal title="创建 Pod" open={createModalVisible} onCancel={() => { setCreateModalVisible(false); createForm.resetFields() }} onOk={() => createForm.submit()} width={500}>
        <Form form={createForm} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="namespace" label="命名空间" rules={[{ required: true }]}>
            <Select options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="例如: my-pod" />
          </Form.Item>
          <Form.Item name="image" label="镜像" rules={[{ required: true }]}>
            <Input placeholder="例如: nginx:latest" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Log Viewer */}
      {selectedPod && (
        <LogViewer
          visible={logVisible}
          onClose={() => { setLogVisible(false); setSelectedPod(null) }}
          clusterId={selectedCluster}
          namespace={selectedPod.namespace}
          podName={selectedPod.name}
        />
      )}

      {/* File Manager */}
      {selectedPod && (
        <PodFileManager
          visible={fileManagerVisible}
          onClose={() => { setFileManagerVisible(false); setSelectedPod(null) }}
          clusterId={selectedCluster}
          namespace={selectedPod.namespace}
          podName={selectedPod.name}
        />
      )}

      {/* Terminal */}
      {selectedPod && (
        <Terminal
          visible={terminalVisible}
          onClose={() => { setTerminalVisible(false); setSelectedPod(null) }}
          clusterId={selectedCluster}
          namespace={selectedPod.namespace}
          podName={selectedPod.name}
        />
      )}
    </div>
  )
}

export default WorkloadPods
