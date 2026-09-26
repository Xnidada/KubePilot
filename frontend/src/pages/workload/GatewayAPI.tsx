import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Drawer, Input, Select, Space, Table, Tabs, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, type Cluster } from '../../api/cluster'
import { get } from '../../api/request'

const { Title, Text } = Typography

interface Condition {
  type: string
  status: string
  reason?: string
  message?: string
  observedGeneration?: number
}

interface ResourceRef { name: string; namespace?: string; kind?: string; group?: string; port?: number }

interface GatewayItem {
  kind: string
  name: string
  namespace?: string
  generation: number
  spec: {
    controllerName?: string
    gatewayClassName?: string
    listeners?: Array<{ name: string; protocol: string; port: number; hostname?: string }>
    parentRefs?: ResourceRef[]
    hostnames?: string[]
    rules?: Array<{ backendRefs?: ResourceRef[] }>
    from?: Array<{ group?: string; kind: string; namespace: string }>
    to?: Array<{ group?: string; kind: string; name?: string }>
  }
  status: {
    addresses?: Array<{ type?: string; value: string }>
    conditions?: Condition[]
    listeners?: Array<{ name: string; conditions?: Condition[] }>
    parents?: Array<{ parentRef: ResourceRef; conditions?: Condition[] }>
  }
}

interface GatewayOverview {
  installed: boolean
  versions: Record<string, string>
  classes_visible: boolean
  items: GatewayItem[]
}

const refText = (ref: ResourceRef, ownNamespace?: string) => `${ref.namespace || ownNamespace || '集群'}/${ref.name}${ref.port ? `:${ref.port}` : ''}`

const conditionsFor = (item: GatewayItem): Condition[] => [
  ...(item.status?.conditions || []),
  ...(item.status?.listeners || []).flatMap(listener => listener.conditions || []),
  ...(item.status?.parents || []).flatMap(parent => parent.conditions || []),
]

const conditionTag = (item: GatewayItem) => {
  if (item.kind === 'ReferenceGrant') return <Tag>授权策略</Tag>
  const route = item.kind === 'HTTPRoute' || item.kind === 'GRPCRoute'
  const groups = route ? (item.status?.parents || []).map(parent => parent.conditions || [])
    : item.kind === 'Gateway' ? [item.status?.conditions || [], ...(item.status?.listeners || []).map(listener => listener.conditions || [])]
      : [item.status?.conditions || []]
  if (!groups.length || groups.some(group => !group.length)) return <Tag>状态未上报</Tag>
  if (route && groups.length < (item.spec?.parentRefs || []).length) return <Tag color="warning">待确认</Tag>
  if (item.kind === 'Gateway' && groups.length - 1 < (item.spec?.listeners || []).length) return <Tag color="warning">待确认</Tag>
  const conditions = groups.flat()
  if (conditions.some(condition => condition.observedGeneration !== undefined && condition.observedGeneration < item.generation)) {
    return <Tag color="warning">状态过期</Tag>
  }
  if (conditions.some(condition => condition.status === 'False')) return <Tag color="error">控制器拒绝</Tag>
  if (conditions.some(condition => condition.status !== 'True')) return <Tag color="warning">状态未知</Tag>
  const required = item.kind === 'GatewayClass' ? ['Accepted']
    : item.kind === 'Gateway' ? ['Accepted', 'Programmed'] : ['Accepted', 'ResolvedRefs']
  const requiredGroups = route ? groups : groups.slice(0, 1)
  if (requiredGroups.some(group => required.some(type => !group.some(condition => condition.type === type && condition.status === 'True')))) {
    return <Tag color="warning">待确认</Tag>
  }
  return <Tag color="success">控制器已确认</Tag>
}

const GatewayAPI: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [clusterID, setClusterID] = useState(0)
  const [namespaceDraft, setNamespaceDraft] = useState('')
  const [namespace, setNamespace] = useState('')
  const [overview, setOverview] = useState<GatewayOverview | null>(null)
  const [selected, setSelected] = useState<GatewayItem | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    getClusterList(1, 100).then(res => {
      setClusters(res.data || [])
      if (res.data?.length) setClusterID(res.data[0].id)
    }).catch(() => setError('获取集群列表失败'))
  }, [])

  const refresh = useCallback(async () => {
    if (!clusterID) return
    setLoading(true)
    setError('')
    try {
      const res = await get<{ code: number; data: GatewayOverview }>(`/clusters/${clusterID}/workloads/gateway-api`, {
        params: namespace ? { ns: namespace } : {},
      })
      setOverview(res.data)
    } catch (failure) {
      setOverview(null)
      setError(failure instanceof Error ? failure.message : '获取 Gateway API 资源失败')
    } finally {
      setLoading(false)
    }
  }, [clusterID, namespace])

  useEffect(() => { refresh() }, [refresh])

  const nameColumn = {
    title: '名称', dataIndex: 'name', key: 'name',
    render: (_: string, item: GatewayItem) => <Button type="link" onClick={() => setSelected(item)}>{item.name}</Button>,
  }
  const namespaceColumn = { title: '命名空间', dataIndex: 'namespace', key: 'namespace', render: (value?: string) => value || '—' }
  const statusColumn = { title: '控制器状态', key: 'status', render: (_: unknown, item: GatewayItem) => conditionTag(item) }
  const columns: Record<string, ColumnsType<GatewayItem>> = {
    GatewayClass: [nameColumn, { title: '控制器', key: 'controller', render: (_, item) => item.spec?.controllerName || '—' }, statusColumn],
    Gateway: [nameColumn, namespaceColumn,
      { title: 'Class', key: 'class', render: (_, item) => item.spec?.gatewayClassName || '—' },
      { title: '监听器', key: 'listeners', render: (_, item) => (item.spec?.listeners || []).map(listener => `${listener.name} ${listener.protocol}:${listener.port}`).join(', ') || '—' },
      { title: '地址', key: 'addresses', render: (_, item) => (item.status?.addresses || []).map(address => address.value).join(', ') || '未分配' }, statusColumn],
    Route: [
      { title: '类型', dataIndex: 'kind', key: 'kind' }, nameColumn, namespaceColumn,
      { title: '域名', key: 'hosts', render: (_, item) => item.spec?.hostnames?.join(', ') || '全部' },
      { title: '关联 Gateway', key: 'parents', render: (_, item) => item.spec?.parentRefs?.map(ref => refText(ref, item.namespace)).join(', ') || '—' },
      { title: '后端', key: 'backends', render: (_, item) => item.spec?.rules?.flatMap(rule => rule.backendRefs || []).map(ref => refText(ref, item.namespace)).join(', ') || '—' },
      statusColumn,
    ],
    ReferenceGrant: [nameColumn, namespaceColumn,
      { title: '允许来源', key: 'from', render: (_, item) => item.spec?.from?.map(from => `${from.kind} @ ${from.namespace}`).join(', ') || '—' },
      { title: '目标类型', key: 'to', render: (_, item) => item.spec?.to?.map(to => `${to.kind}${to.name ? `/${to.name}` : ''}`).join(', ') || '—' },
    ],
  }

  const table = (kinds: string[], key: keyof typeof columns) => {
    const available = kinds.filter(kind => overview?.versions[kind])
    if (!available.length) return <Alert type="info" showIcon message={`${kinds.join(' / ')} CRD 未提供`} />
    const items = (overview?.items || []).filter(item => kinds.includes(item.kind))
    return <Table<GatewayItem> rowKey={item => `${item.kind}/${item.namespace || ''}/${item.name}`}
      columns={columns[key]} dataSource={items} loading={loading} scroll={{ x: 1000 }} />
  }

  return <div>
    <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 16 }} wrap>
      <Title level={4} style={{ margin: 0 }}>Gateway API</Title>
      <Space wrap>
        <Select value={clusterID || undefined} style={{ width: 200 }} placeholder="选择集群"
          options={clusters.map(cluster => ({ value: cluster.id, label: cluster.display_name || cluster.name }))}
          onChange={value => { setClusterID(value); setNamespace(''); setNamespaceDraft(''); setOverview(null) }} />
        <Input value={namespaceDraft} onChange={event => setNamespaceDraft(event.target.value)}
          onPressEnter={() => { if (namespaceDraft.trim() === namespace) refresh(); else setNamespace(namespaceDraft.trim()) }}
          placeholder="命名空间（留空为有权范围）" style={{ width: 230 }} allowClear />
        <Button icon={<ReloadOutlined />} loading={loading}
          onClick={() => { if (namespaceDraft.trim() === namespace) refresh(); else setNamespace(namespaceDraft.trim()) }}>查询 / 刷新</Button>
      </Space>
    </Space>
    {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />}
    {overview && !overview.installed ? <Alert type="info" showIcon message="当前集群未提供 Gateway API v1 资源" description="本页面只读取资源，不会自动安装 CRD 或控制器。" /> : null}
    {overview?.installed && <Card>
      {!overview.classes_visible && <Alert type="info" showIcon message="当前账号没有集群级授权，GatewayClass 列表已隐藏。" style={{ marginBottom: 16 }} />}
      <Tabs items={[
        { key: 'gateways', label: `Gateway (${overview.items.filter(item => item.kind === 'Gateway').length})`, children: table(['Gateway'], 'Gateway') },
        { key: 'routes', label: `路由 (${overview.items.filter(item => item.kind === 'HTTPRoute' || item.kind === 'GRPCRoute').length})`, children: table(['HTTPRoute', 'GRPCRoute'], 'Route') },
        { key: 'classes', label: `GatewayClass (${overview.items.filter(item => item.kind === 'GatewayClass').length})`, children: overview.classes_visible ? table(['GatewayClass'], 'GatewayClass') : <Text type="secondary">需要集群级查看权限</Text> },
        { key: 'grants', label: `ReferenceGrant (${overview.items.filter(item => item.kind === 'ReferenceGrant').length})`, children: table(['ReferenceGrant'], 'ReferenceGrant') },
      ]} />
    </Card>}
    <Drawer title={`${selected?.kind || ''} ${selected?.namespace ? `${selected.namespace}/` : ''}${selected?.name || ''}`}
      open={!!selected} onClose={() => setSelected(null)} width={720}>
      {selected && <>
        <Space style={{ marginBottom: 16 }}>{conditionTag(selected)}<Text type="secondary">Generation {selected.generation}</Text></Space>
        {conditionsFor(selected).map((condition, index) => <Alert key={`${condition.type}-${index}`}
          type={condition.status === 'False' ? 'error' : condition.status === 'True' ? 'success' : 'warning'}
          showIcon message={`${condition.type}: ${condition.status}${condition.reason ? ` (${condition.reason})` : ''}`}
          description={condition.message || undefined} style={{ marginBottom: 8 }} />)}
        <Title level={5}>配置与状态</Title>
        <pre style={{ overflowX: 'auto', maxHeight: 500, fontSize: 12 }}>{JSON.stringify({ spec: selected.spec, status: selected.status }, null, 2)}</pre>
      </>}
    </Drawer>
  </div>
}

export default GatewayAPI
