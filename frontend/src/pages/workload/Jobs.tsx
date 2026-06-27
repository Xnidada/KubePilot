import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, message, Popconfirm, Tooltip, Alert, Modal
} from 'antd'
import {
  SyncOutlined, DeleteOutlined, SearchOutlined, ScheduleOutlined,
  RocketOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import StatusTag from '../../components/StatusTag'
import { get, del } from '../../api/request'

const { Title, Text } = Typography

interface Job {
  name: string
  namespace: string
  status: string
  completions: number
  succeeded: number
  age: string
  images: string[]
  labels?: Record<string, string>
}

const JobManagement: React.FC = () => {
  const navigate = useNavigate()
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [searchText, setSearchText] = useState('')
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([])

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchData() } }, [selectedCluster, selectedNamespace])

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

  const fetchData = async () => {
    setLoading(true)
    try {
      const params = selectedNamespace ? `?ns=${selectedNamespace}` : ''
      const res = await get<{ code: number; data: Job[] }>(`/clusters/${selectedCluster}/workloads/jobs${params}`)
      setJobs(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleDelete = async (record: Job) => {
    try {
      await del(`/clusters/${selectedCluster}/workloads/jobs/${record.namespace}/${record.name}`)
      message.success('删除成功')
      fetchData()
    } catch (e) { console.error(e) }
  }

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的 Job')
      return
    }
    Modal.confirm({
      title: '批量删除',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个 Job 吗？`,
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        let success = 0
        for (const key of selectedRowKeys) {
          const [namespace, name] = key.split('/')
          try {
            await del(`/clusters/${selectedCluster}/workloads/jobs/${namespace}/${name}`)
            success++
          } catch (e) { /* ignore */ }
        }
        message.success(`成功删除 ${success} 个 Job`)
        setSelectedRowKeys([])
        fetchData()
      },
    })
  }

  // 判断是否为调度任务创建的 Job
  const isSchedulerJob = (record: Job) => {
    return record.labels?.['kubepilot/task-id'] !== undefined
  }

  const columns: ColumnsType<Job> = [
    { title: '名称', dataIndex: 'name', key: 'name', filteredValue: searchText ? [searchText] : null, onFilter: (v, r) => r.name.includes(v as string) },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: '来源', key: 'source', width: 120,
      render: (_, record) => (
        isSchedulerJob(record) ? (
          <Tag color="blue" icon={<ScheduleOutlined />}>调度任务</Tag>
        ) : (
          <Tag color="default">手动创建</Tag>
        )
      )
    },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s) => <StatusTag status={s} /> },
    { title: 'Completions', dataIndex: 'completions', key: 'completions' },
    { title: 'Succeeded', dataIndex: 'succeeded', key: 'succeeded' },
    { title: '镜像', dataIndex: 'images', key: 'images', render: (imgs: string[]) => imgs?.map(i => <Tag key={i}>{i}</Tag>) },
    { title: '年龄', dataIndex: 'age', key: 'age' },
    {
      title: '操作', key: 'action', width: 100,
      render: (_, record) => (
        <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record)}>
          <Button type="link" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>Job</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }} options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }} placeholder="所有命名空间" allowClear options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Input placeholder="搜索..." prefix={<SearchOutlined />} value={searchText} onChange={(e) => setSearchText(e.target.value)} style={{ width: 200 }} />
          <Button icon={<SyncOutlined />} onClick={fetchData}>刷新</Button>
          <Tooltip title="通过任务调度创建 Job，支持优先级、队列、资源预留">
            <Button type="primary" icon={<RocketOutlined />} onClick={() => navigate('/scheduler/tasks')}>
              任务调度创建
            </Button>
          </Tooltip>
        </Space>
      </div>

      {/* 批量操作栏 */}
      {selectedRowKeys.length > 0 && (
        <Card style={{ marginBottom: 16 }}>
          <Space>
            <Text strong>已选择 {selectedRowKeys.length} 项</Text>
            <Button danger icon={<DeleteOutlined />} onClick={handleBatchDelete}>批量删除</Button>
            <Button type="link" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
          </Space>
        </Card>
      )}

      <Card>
        <Alert
          message="Job 管理说明"
          description="此页面为 K8S Job 资源的只读视图。如需创建新任务，请使用「任务调度」功能，支持优先级调度、队列管理和资源预留。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" type="primary" icon={<ScheduleOutlined />} onClick={() => navigate('/scheduler/tasks')}>
              前往任务调度
            </Button>
          }
        />
        <Table
          columns={columns}
          dataSource={jobs}
          rowKey={(r) => `${r.namespace}/${r.name}`}
          loading={loading}
          rowSelection={{
            selectedRowKeys,
            onChange: (keys: any[]) => setSelectedRowKeys(keys),
          }}
        />
      </Card>
    </div>
  )
}

export default JobManagement
