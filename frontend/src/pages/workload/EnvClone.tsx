import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Checkbox, Alert, Table, Row, Col, Tag, Tooltip
} from 'antd'
import { CopyOutlined, ReloadOutlined, CheckSquareOutlined, WarningOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getDeployments, getServices } from '../../api/workload'
import { post } from '../../api/request'

const { Title, Text } = Typography

interface ResourceItem {
  key: string
  kind: string
  name: string
  namespace: string
  selected: boolean
  existsInTarget: boolean
}

const EnvClone: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [sourceNamespace, setSourceNamespace] = useState('')
  const [targetNamespace, setTargetNamespace] = useState('')
  const [resources, setResources] = useState<ResourceItem[]>([])
  const [loading, setLoading] = useState(false)
  const [cloning, setCloning] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) fetchNamespaces() }, [selectedCluster])
  useEffect(() => { if (selectedCluster && sourceNamespace) fetchResources() }, [selectedCluster, sourceNamespace, targetNamespace])

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

  const fetchResources = async () => {
    setLoading(true)
    try {
      const items: ResourceItem[] = []
      const targetNames = new Set<string>()

      // 先获取目标命名空间的资源
      if (targetNamespace) {
        try {
          const targetDeploys = await getDeployments(selectedCluster, targetNamespace)
          for (const d of (targetDeploys.data || [])) {
            targetNames.add(`Deployment/${d.name}`)
          }
        } catch (e) {}

        try {
          const targetSvcs = await getServices(selectedCluster, targetNamespace)
          for (const s of (targetSvcs.data || [])) {
            targetNames.add(`Service/${s.name}`)
          }
        } catch (e) {}
      }

      // 获取源命名空间的资源
      try {
        const deploys = await getDeployments(selectedCluster, sourceNamespace)
        for (const d of (deploys.data || [])) {
          const exists = targetNames.has(`Deployment/${d.name}`)
          items.push({
            key: `Deployment/${d.namespace}/${d.name}`,
            kind: 'Deployment',
            name: d.name,
            namespace: d.namespace,
            selected: !exists,  // 已存在的默认不选
            existsInTarget: exists,
          })
        }
      } catch (e) {}

      try {
        const svcs = await getServices(selectedCluster, sourceNamespace)
        for (const s of (svcs.data || [])) {
          const exists = targetNames.has(`Service/${s.name}`)
          items.push({
            key: `Service/${s.namespace}/${s.name}`,
            kind: 'Service',
            name: s.name,
            namespace: s.namespace,
            selected: !exists,
            existsInTarget: exists,
          })
        }
      } catch (e) {}

      setResources(items)
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleToggleResource = (key: string) => {
    setResources(prev => prev.map(r =>
      r.key === key ? { ...r, selected: !r.selected } : r
    ))
  }

  const handleSelectAll = () => {
    const selectable = resources.filter(r => !r.existsInTarget)
    const allSelected = selectable.every(r => r.selected)
    setResources(prev => prev.map(r =>
      r.existsInTarget ? r : { ...r, selected: !allSelected }
    ))
  }

  const selectedCount = resources.filter(r => r.selected).length
  const duplicateCount = resources.filter(r => r.existsInTarget).length

  const handleClone = async () => {
    if (!sourceNamespace || !targetNamespace) {
      message.warning('请选择源和目标命名空间')
      return
    }
    if (sourceNamespace === targetNamespace) {
      message.warning('源和目标命名空间不能相同')
      return
    }
    if (selectedCount === 0) {
      message.warning('请至少选择一个资源')
      return
    }

    setCloning(true)
    try {
      const selectedResources = resources.filter(r => r.selected).map(r => ({
        kind: r.kind,
        name: r.name,
      }))

      await post(`/clusters/${selectedCluster}/workloads/namespaces/clone`, {
        source: sourceNamespace,
        target: targetNamespace,
        resources: selectedResources,
      })

      message.success(`成功克隆 ${selectedCount} 个资源到 ${targetNamespace}`)
      fetchResources() // 刷新列表
    } catch (e) {
      message.error('克隆失败')
    } finally {
      setCloning(false)
    }
  }

  const columns: ColumnsType<ResourceItem> = [
    {
      title: (
        <Checkbox
          checked={resources.filter(r => !r.existsInTarget).length > 0 && resources.filter(r => !r.existsInTarget).every(r => r.selected)}
          indeterminate={selectedCount > 0 && selectedCount < resources.filter(r => !r.existsInTarget).length}
          onChange={handleSelectAll}
        />
      ),
      key: 'select',
      width: 50,
      render: (_, record) => (
        <Checkbox
          checked={record.selected}
          disabled={record.existsInTarget}
          onChange={() => handleToggleResource(record.key)}
        />
      ),
    },
    {
      title: '类型',
      dataIndex: 'kind',
      key: 'kind',
      width: 120,
      render: (kind) => (
        <Tag color={kind === 'Deployment' ? 'blue' : 'green'}>{kind}</Tag>
      ),
    },
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '目标状态',
      key: 'status',
      width: 120,
      render: (_, record) => (
        record.existsInTarget ? (
          <Tooltip title="目标命名空间已存在同名资源">
            <Tag color="warning" icon={<WarningOutlined />}>已存在</Tag>
          </Tooltip>
        ) : (
          <Tag color="success">可克隆</Tag>
        )
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>📋 环境克隆</Title>

      <Alert
        message="环境克隆说明"
        description="从源命名空间克隆资源配置到目标命名空间。已存在于目标命名空间的资源会自动标记，避免重复克隆。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Row gutter={16}>
        <Col span={8}>
          <Card title="克隆配置">
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>集群</Text>
                <Select
                  value={selectedCluster}
                  onChange={setSelectedCluster}
                  style={{ width: '100%', marginTop: 8 }}
                  options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
                />
              </div>
              <div>
                <Text strong>源命名空间</Text>
                <Select
                  value={sourceNamespace}
                  onChange={setSourceNamespace}
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择源命名空间"
                  options={namespaces.map(ns => ({ label: ns, value: ns }))}
                />
              </div>
              <div>
                <Text strong>目标命名空间</Text>
                <Select
                  value={targetNamespace}
                  onChange={setTargetNamespace}
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择目标命名空间"
                  options={namespaces.map(ns => ({ label: ns, value: ns }))}
                />
              </div>

              {duplicateCount > 0 && (
                <Alert
                  message={`${duplicateCount} 个资源在目标命名空间已存在`}
                  type="warning"
                  showIcon
                />
              )}

              <Button
                type="primary"
                icon={<CopyOutlined />}
                onClick={handleClone}
                loading={cloning}
                block
                disabled={selectedCount === 0}
              >
                克隆选中资源 ({selectedCount})
              </Button>
            </Space>
          </Card>
        </Col>

        <Col span={16}>
          <Card
            title={`源命名空间资源 (${resources.length})`}
            extra={
              <Space>
                <Button size="small" icon={<CheckSquareOutlined />} onClick={handleSelectAll}>
                  {resources.filter(r => !r.existsInTarget).every(r => r.selected) ? '取消全选' : '全选可克隆'}
                </Button>
                <Button size="small" icon={<ReloadOutlined />} onClick={fetchResources}>刷新</Button>
              </Space>
            }
          >
            <Table
              columns={columns}
              dataSource={resources}
              rowKey="key"
              loading={loading}
              size="small"
              pagination={false}
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default EnvClone
