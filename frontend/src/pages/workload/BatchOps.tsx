import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, message, Table, Tag, Input, Form, Modal
} from 'antd'
import {
  DeleteOutlined, ReloadOutlined, TagOutlined
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames, getPods, getDeployments, getServices, batchOperation, ResourceRef, BatchResult } from '../../api/workload'

const { Title, Text } = Typography

const BatchOps: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [resourceType, setResourceType] = useState<string>('pod')
  const [resources, setResources] = useState<ResourceRef[]>([])
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<BatchResult | null>(null)
  const [labelModalVisible, setLabelModalVisible] = useState(false)
  const [labelForm] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchResources() } }, [selectedCluster, selectedNamespace, resourceType])

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
      let res: any
      const ns = selectedNamespace || undefined
      switch (resourceType) {
        case 'pod':
          res = await getPods(selectedCluster, ns)
          setResources((res.data || []).map((r: any) => ({ kind: 'Pod', name: r.name, namespace: r.namespace })))
          break
        case 'deployment':
          res = await getDeployments(selectedCluster, ns)
          setResources((res.data || []).map((r: any) => ({ kind: 'Deployment', name: r.name, namespace: r.namespace })))
          break
        case 'service':
          res = await getServices(selectedCluster, ns)
          setResources((res.data || []).map((r: any) => ({ kind: 'Service', name: r.name, namespace: r.namespace })))
          break
      }
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleBatchAction = async (action: 'delete' | 'restart' | 'label') => {
    if (selectedKeys.length === 0) {
      message.warning('请先选择资源')
      return
    }

    if (action === 'label') {
      setLabelModalVisible(true)
      return
    }

    const confirmText = action === 'delete'
      ? `确定要删除选中的 ${selectedKeys.length} 个资源吗？`
      : `确定要重启选中的 ${selectedKeys.length} 个资源吗？`

    Modal.confirm({
      title: '确认操作',
      content: confirmText,
      onOk: async () => {
        await executeBatch(action)
      }
    })
  }

  const executeBatch = async (action: string, labels?: Record<string, string>) => {
    const selectedResources = resources.filter(r => selectedKeys.includes(`${r.kind}/${r.namespace}/${r.name}`))
    try {
      const res = await batchOperation({
        cluster_id: selectedCluster,
        resources: selectedResources,
        action: action as any,
        labels,
      })
      setResult(res.data)
      message.success(`操作完成: 成功 ${res.data.success}, 失败 ${res.data.failed}`)
      fetchResources()
    } catch (e) { message.error('批量操作失败') }
  }

  const handleLabelSubmit = async (values: any) => {
    const labels: Record<string, string> = {}
    values.labels?.forEach((item: any) => {
      if (item.key && item.value) labels[item.key] = item.value
    })
    await executeBatch('label', labels)
    setLabelModalVisible(false)
    labelForm.resetFields()
  }

  const rowSelection = {
    selectedRowKeys: selectedKeys,
    onChange: (keys: any[]) => setSelectedKeys(keys),
  }

  const columns = [
    { title: '类型', dataIndex: 'kind', key: 'kind', render: (k: string) => <Tag>{k}</Tag> },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>批量操作</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="所有命名空间" allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Select value={resourceType} onChange={setResourceType} style={{ width: 120 }}
            options={[
              { label: 'Pod', value: 'pod' },
              { label: 'Deployment', value: 'deployment' },
              { label: 'Service', value: 'service' },
            ]} />
          <Button icon={<ReloadOutlined />} onClick={fetchResources}>刷新</Button>
        </Space>
      </div>

      <Card
        title={`资源列表 (已选 ${selectedKeys.length} 项)`}
        extra={
          <Space>
            <Button danger icon={<DeleteOutlined />} onClick={() => handleBatchAction('delete')}
              disabled={selectedKeys.length === 0}>
              批量删除
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => handleBatchAction('restart')}
              disabled={selectedKeys.length === 0}>
              批量重启
            </Button>
            <Button icon={<TagOutlined />} onClick={() => handleBatchAction('label')}
              disabled={selectedKeys.length === 0}>
              批量标签
            </Button>
          </Space>
        }
      >
        <Table
          rowSelection={rowSelection}
          columns={columns}
          dataSource={resources.map((r) => ({ ...r, key: `${r.kind}/${r.namespace}/${r.name}` }))}
          loading={loading}
          size="small"
        />
      </Card>

      {result && (
        <Card title="操作结果" style={{ marginTop: 16 }}>
          <Space size="large">
            <Text>总计: <Tag>{result.total}</Tag></Text>
            <Text>成功: <Tag color="success">{result.success}</Tag></Text>
            <Text>失败: <Tag color="error">{result.failed}</Tag></Text>
          </Space>
          {result.results.filter(r => r.status === 'failed').length > 0 && (
            <Table
              size="small"
              style={{ marginTop: 16 }}
              dataSource={result.results.filter(r => r.status === 'failed')}
              columns={[
                { title: '类型', dataIndex: 'kind', render: (k: string) => <Tag>{k}</Tag> },
                { title: '名称', dataIndex: 'name' },
                { title: '错误', dataIndex: 'error', render: (e: string) => <Text type="danger">{e}</Text> },
              ]}
            />
          )}
        </Card>
      )}

      <Modal title="批量设置标签" open={labelModalVisible}
        onCancel={() => setLabelModalVisible(false)}
        onOk={() => labelForm.submit()} width={500}>
        <Form form={labelForm} layout="vertical" onFinish={handleLabelSubmit}>
          <Form.List name="labels">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name }) => (
                  <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                    <Form.Item name={[name, 'key']} noStyle rules={[{ required: true }]}>
                      <Input placeholder="标签名" />
                    </Form.Item>
                    <Form.Item name={[name, 'value']} noStyle rules={[{ required: true }]}>
                      <Input placeholder="标签值" />
                    </Form.Item>
                    <Button onClick={() => remove(name)} danger>删除</Button>
                  </Space>
                ))}
                <Button onClick={() => add()} block>添加标签</Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>
    </div>
  )
}

export default BatchOps
