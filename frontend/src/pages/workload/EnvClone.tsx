import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Checkbox, Alert, Table, Row, Col, Tag
} from 'antd'
import { CopyOutlined, ReloadOutlined, CheckSquareOutlined } from '@ant-design/icons'
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
  useEffect(() => { if (selectedCluster && sourceNamespace) fetchResources() }, [selectedCluster, sourceNamespace])

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

      // 获取 Deployments
      try {
        const deploys = await getDeployments(selectedCluster, sourceNamespace)
        for (const d of (deploys.data || [])) {
          items.push({
            key: `Deployment/${d.namespace}/${d.name}`,
            kind: 'Deployment',
            name: d.name,
            namespace: d.namespace,
            selected: true,
          })
        }
      } catch (e) {}

      // 获取 Services
      try {
        const svcs = await getServices(selectedCluster, sourceNamespace)
        for (const s of (svcs.data || [])) {
          items.push({
            key: `Service/${s.namespace}/${s.name}`,
            kind: 'Service',
            name: s.name,
            namespace: s.namespace,
            selected: true,
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
    const allSelected = resources.every(r => r.selected)
    setResources(prev => prev.map(r => ({ ...r, selected: !allSelected })))
  }

  const selectedCount = resources.filter(r => r.selected).length

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
          checked={resources.length > 0 && resources.every(r => r.selected)}
          indeterminate={selectedCount > 0 && selectedCount < resources.length}
          onChange={handleSelectAll}
        />
      ),
      key: 'select',
      width: 50,
      render: (_, record) => (
        <Checkbox
          checked={record.selected}
          onChange={() => handleToggleResource(record.key)}
        />
      ),
    },
    {
      title: '类型',
      dataIndex: 'kind',
      key: 'kind',
      filters: [
        { text: 'Deployment', value: 'Deployment' },
        { text: 'Service', value: 'Service' },
      ],
      onFilter: (value, record) => record.kind === value,
      render: (kind) => (
        <Tag color={kind === 'Deployment' ? 'blue' : 'green'}>{kind}</Tag>
      ),
    },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
  ]

  return (
    <div>
      <Title level={4}>📋 环境克隆</Title>

      <Alert
        message="环境克隆说明"
        description="从源命名空间克隆资源配置到目标命名空间。支持选择具体资源进行克隆。"
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
            title="资源列表"
            extra={
              <Space>
                <Button size="small" icon={<CheckSquareOutlined />} onClick={handleSelectAll}>
                  {resources.every(r => r.selected) ? '取消全选' : '全选'}
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
