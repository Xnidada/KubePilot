import { useEffect, useState } from 'react'
import {
  Card, Table, Tag, Button, Space, Typography, Select, Input, Spin, Collapse, Badge, Row, Col, Statistic, Modal, message, Popconfirm
} from 'antd'
import {
  SyncOutlined, SearchOutlined, ApiOutlined, FolderOutlined, CloudServerOutlined,
  EyeOutlined, DeleteOutlined, UnorderedListOutlined, PlusOutlined, EditOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { get, post, put, del } from '../../api/request'

const { Title, Text } = Typography

interface CRDResource {
  group: string
  version: string
  kind: string
  name: string
  namespaced: boolean
  verbs: string[]
}

interface CRDGroup {
  group: string
  resources: CRDResource[]
}

const CRDManagement: React.FC = () => {
  const [crds, setCrds] = useState<CRDResource[]>([])
  const [loading, setLoading] = useState(false)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [searchText, setSearchText] = useState('')
  const [expandedGroups, setExpandedGroups] = useState<string[]>([])

  // Custom resource list state
  const [resourceListVisible, setResourceListVisible] = useState(false)
  const [selectedCRD, setSelectedCRD] = useState<CRDResource | null>(null)
  const [customResources, setCustomResources] = useState<any[]>([])
  const [resourceLoading, setResourceLoading] = useState(false)

  // Resource detail state
  const [detailVisible, setDetailVisible] = useState(false)
  const [selectedResource, setSelectedResource] = useState<any>(null)

  // Create/Edit state
  const [createVisible, setCreateVisible] = useState(false)
  const [editVisible, setEditVisible] = useState(false)
  const [editResource, setEditResource] = useState<any>(null)
  const [jsonInput, setJsonInput] = useState('')

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchCRDs() } }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchCRDs = async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: CRDResource[] }>(`/clusters/${selectedCluster}/workloads/crds`)
      setCrds(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const fetchCustomResources = async (crd: CRDResource) => {
    setResourceLoading(true)
    setSelectedCRD(crd)
    setResourceListVisible(true)
    try {
      const res = await get<{ code: number; data: any }>(
        `/clusters/${selectedCluster}/workloads/crds/${crd.group}/${crd.version}/${crd.name}`
      )
      setCustomResources(res.data?.items || [])
    } catch (e) {
      console.error(e)
      message.error('获取资源列表失败')
    } finally {
      setResourceLoading(false)
    }
  }

  const handleCreate = async () => {
    if (!selectedCRD || !jsonInput) return
    try {
      const data = JSON.parse(jsonInput)
      await post(`/clusters/${selectedCluster}/workloads/crds/${selectedCRD.group}/${selectedCRD.version}/${selectedCRD.name}`, data)
      message.success('创建成功')
      setCreateVisible(false)
      setJsonInput('')
      fetchCustomResources(selectedCRD)
    } catch (e: any) {
      message.error(e.message || '创建失败')
    }
  }

  const handleEdit = async () => {
    if (!selectedCRD || !editResource || !jsonInput) return
    try {
      const data = JSON.parse(jsonInput)
      const name = editResource.metadata?.name
      const namespace = editResource.metadata?.namespace
      let url = `/clusters/${selectedCluster}/workloads/crds/${selectedCRD.group}/${selectedCRD.version}/${selectedCRD.name}/${name}`
      if (namespace) {
        url += `?ns=${namespace}`
      }
      await put(url, data)
      message.success('更新成功')
      setEditVisible(false)
      setJsonInput('')
      setEditResource(null)
      fetchCustomResources(selectedCRD)
    } catch (e: any) {
      message.error(e.message || '更新失败')
    }
  }

  const handleDeleteResource = async (resource: any) => {
    if (!selectedCRD) return
    try {
      const name = resource.metadata?.name
      const namespace = resource.metadata?.namespace
      let url = `/clusters/${selectedCluster}/workloads/crds/${selectedCRD.group}/${selectedCRD.version}/${selectedCRD.name}/${name}`
      if (namespace) {
        url += `?ns=${namespace}`
      }
      await del(url)
      message.success('删除成功')
      fetchCustomResources(selectedCRD)
    } catch (e) {
      console.error(e)
      message.error('删除失败')
    }
  }

  // 按 group 分组
  const groupedCRDs: CRDGroup[] = crds.reduce((acc, crd) => {
    const group = crd.group || 'core'
    let existing = acc.find(g => g.group === group)
    if (!existing) {
      existing = { group, resources: [] }
      acc.push(existing)
    }
    existing.resources.push(crd)
    return acc
  }, [] as CRDGroup[])

  const filteredGroups = groupedCRDs.filter(group => {
    if (!searchText) return true
    if (group.group.toLowerCase().includes(searchText.toLowerCase())) return true
    return group.resources.some(r =>
      r.kind.toLowerCase().includes(searchText.toLowerCase()) ||
      r.name.toLowerCase().includes(searchText.toLowerCase())
    )
  })

  const crdColumns: ColumnsType<CRDResource> = [
    { title: 'Kind', dataIndex: 'kind', key: 'kind', render: (v) => <Tag color="blue">{v}</Tag> },
    { title: 'Name', dataIndex: 'name', key: 'name' },
    { title: 'Version', dataIndex: 'version', key: 'version' },
    { title: '命名空间', dataIndex: 'namespaced', key: 'namespaced', render: (v) => v ? <Tag color="green">是</Tag> : <Tag>否</Tag> },
    {
      title: '操作', key: 'action', width: 200,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" icon={<UnorderedListOutlined />} onClick={() => fetchCustomResources(record)}>
            查看资源
          </Button>
        </Space>
      ),
    },
  ]

  const resourceColumns: ColumnsType<any> = [
    {
      title: '名称', dataIndex: ['metadata', 'name'], key: 'name',
      render: (name: string) => <Text strong>{name}</Text>,
    },
    {
      title: '命名空间', dataIndex: ['metadata', 'namespace'], key: 'namespace',
      render: (ns: string) => ns || '-',
    },
    {
      title: '年龄', dataIndex: ['metadata', 'creationTimestamp'], key: 'age',
      render: (time: string) => {
        if (!time) return '-'
        const date = new Date(time)
        const now = new Date()
        const diff = now.getTime() - date.getTime()
        const days = Math.floor(diff / 86400000)
        if (days > 0) return `${days}天`
        const hours = Math.floor(diff / 3600000)
        if (hours > 0) return `${hours}小时`
        const minutes = Math.floor(diff / 60000)
        return `${minutes}分钟`
      },
    },
    {
      title: '操作', key: 'action', width: 200,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" icon={<EyeOutlined />} onClick={() => { setSelectedResource(record); setDetailVisible(true) }}>
            详情
          </Button>
          <Button type="link" icon={<EditOutlined />} onClick={() => {
            setEditResource(record)
            setJsonInput(JSON.stringify(record, null, 2))
            setEditVisible(true)
          }}>
            编辑
          </Button>
          <Popconfirm title="确定删除？" onConfirm={() => handleDeleteResource(record)}>
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>CRD 管理</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Input
            placeholder="搜索 CRD..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 300 }}
          />
          <Button icon={<SyncOutlined />} onClick={fetchCRDs}>刷新</Button>
        </Space>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col span={8}>
          <Card>
            <Statistic title="CRD 总数" value={crds.length} prefix={<ApiOutlined />} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="API 组数" value={groupedCRDs.length} prefix={<FolderOutlined />} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="集群范围资源" value={crds.filter(c => !c.namespaced).length} prefix={<CloudServerOutlined />} />
          </Card>
        </Col>
      </Row>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 50 }}>
          <Spin size="large" />
        </div>
      ) : (
        <Collapse
          activeKey={expandedGroups}
          onChange={(keys) => setExpandedGroups(keys as string[])}
          items={filteredGroups.map(group => ({
            key: group.group,
            label: (
              <Space>
                <FolderOutlined />
                <Text strong>{group.group || 'core'}</Text>
                <Badge count={group.resources.length} style={{ backgroundColor: '#1890ff' }} />
              </Space>
            ),
            children: (
              <Table
                columns={crdColumns}
                dataSource={group.resources}
                rowKey={(r) => `${r.group}/${r.kind}`}
                pagination={false}
                size="small"
              />
            ),
          }))}
        />
      )}

      {/* Custom Resources List Modal */}
      <Modal
        title={`${selectedCRD?.kind || 'Custom Resources'} 列表`}
        open={resourceListVisible}
        onCancel={() => setResourceListVisible(false)}
        footer={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => {
            setJsonInput(JSON.stringify({
              apiVersion: `${selectedCRD?.group}/${selectedCRD?.version}`,
              kind: selectedCRD?.kind,
              metadata: {
                name: '',
                namespace: selectedCRD?.namespaced ? 'default' : undefined,
              },
              spec: {},
            }, null, 2))
            setCreateVisible(true)
          }}>
            创建资源
          </Button>
        }
        width={1000}
      >
        <Table
          columns={resourceColumns}
          dataSource={customResources}
          rowKey={(r) => r.metadata?.name || Math.random().toString()}
          loading={resourceLoading}
          pagination={{ pageSize: 10 }}
          size="small"
        />
      </Modal>

      {/* Resource Detail Modal */}
      <Modal
        title={`资源详情: ${selectedResource?.metadata?.name}`}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={800}
      >
        <pre style={{ background: '#f5f5f5', padding: 16, borderRadius: 8, maxHeight: 500, overflow: 'auto', fontSize: 12 }}>
          {JSON.stringify(selectedResource, null, 2)}
        </pre>
      </Modal>

      {/* Create Resource Modal */}
      <Modal
        title={`创建 ${selectedCRD?.kind || 'Resource'}`}
        open={createVisible}
        onCancel={() => { setCreateVisible(false); setJsonInput('') }}
        onOk={handleCreate}
        width={800}
      >
        <div style={{ marginBottom: 8 }}>
          <Text type="secondary">请输入 JSON 格式的资源定义：</Text>
        </div>
        <textarea
          value={jsonInput}
          onChange={(e) => setJsonInput(e.target.value)}
          style={{
            width: '100%',
            height: 400,
            fontFamily: 'Consolas, Monaco, monospace',
            fontSize: 13,
            padding: 16,
            border: '1px solid #d9d9d9',
            borderRadius: 8,
            background: '#f5f5f5',
          }}
          spellCheck={false}
        />
      </Modal>

      {/* Edit Resource Modal */}
      <Modal
        title={`编辑 ${selectedCRD?.kind || 'Resource'}: ${editResource?.metadata?.name}`}
        open={editVisible}
        onCancel={() => { setEditVisible(false); setJsonInput(''); setEditResource(null) }}
        onOk={handleEdit}
        width={800}
      >
        <div style={{ marginBottom: 8 }}>
          <Text type="secondary">编辑 JSON 格式的资源定义：</Text>
        </div>
        <textarea
          value={jsonInput}
          onChange={(e) => setJsonInput(e.target.value)}
          style={{
            width: '100%',
            height: 400,
            fontFamily: 'Consolas, Monaco, monospace',
            fontSize: 13,
            padding: 16,
            border: '1px solid #d9d9d9',
            borderRadius: 8,
            background: '#f5f5f5',
          }}
          spellCheck={false}
        />
      </Modal>
    </div>
  )
}

export default CRDManagement
