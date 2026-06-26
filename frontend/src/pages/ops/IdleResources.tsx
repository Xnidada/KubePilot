import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Tag, Table, Alert, Empty
} from 'antd'
import {
  ReloadOutlined, DeleteOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { findIdleResources, cleanIdleResources, IdleResource } from '../../api/ops'

const { Title } = Typography

const IdleResourcesPage: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [resources, setResources] = useState<IdleResource[]>([])
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [cleaning, setCleaning] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchResources() }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchResources = async () => {
    setLoading(true)
    try {
      const res = await findIdleResources(selectedCluster)
      setResources(res.data.resources || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleClean = async () => {
    if (selectedKeys.length === 0) {
      message.warning('请先选择要清理的资源')
      return
    }
    setCleaning(true)
    try {
      const toClean = resources
        .filter(r => selectedKeys.includes(`${r.kind}/${r.namespace}/${r.name}`))
        .map(r => ({ kind: r.kind, name: r.name, namespace: r.namespace }))
      await cleanIdleResources(selectedCluster, toClean)
      message.success(`已清理 ${toClean.length} 个资源`)
      setSelectedKeys([])
      fetchResources()
    } catch (e) { message.error('清理失败') }
    finally { setCleaning(false) }
  }

  const getKindTag = (kind: string) => {
    const colors: Record<string, string> = {
      Job: 'orange',
      ConfigMap: 'blue',
      PVC: 'purple',
      Service: 'green',
      Pod: 'cyan',
    }
    return <Tag color={colors[kind] || 'default'}>{kind}</Tag>
  }

  const columns: ColumnsType<IdleResource> = [
    {
      title: '类型', dataIndex: 'kind', key: 'kind', width: 100,
      render: (k) => getKindTag(k)
    },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    { title: '年龄', dataIndex: 'age', key: 'age', width: 80 },
    { title: '原因', dataIndex: 'reason', key: 'reason' },
  ]

  const rowSelection = {
    selectedRowKeys: selectedKeys,
    onChange: (keys: any[]) => setSelectedKeys(keys),
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>🧹 闲置资源清理</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchResources}>扫描</Button>
          <Button danger icon={<DeleteOutlined />} onClick={handleClean}
            disabled={selectedKeys.length === 0} loading={cleaning}>
            清理选中 ({selectedKeys.length})
          </Button>
        </Space>
      </div>

      <Alert
        message="闲置资源说明"
        description="自动扫描已完成的 Job、未引用的 ConfigMap、未绑定的 PVC、无后端的 Service、已完成的 Pod 等闲置资源。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Card
        title={`扫描结果 (共 ${resources.length} 个闲置资源)`}
      >
        {resources.length === 0 ? (
          <Empty description="未发现闲置资源，集群状态良好！" />
        ) : (
          <Table
            rowSelection={rowSelection}
            columns={columns}
            dataSource={resources.map(r => ({ ...r, key: `${r.kind}/${r.namespace}/${r.name}` }))}
            loading={loading}
            size="small"
          />
        )}
      </Card>
    </div>
  )
}

export default IdleResourcesPage
