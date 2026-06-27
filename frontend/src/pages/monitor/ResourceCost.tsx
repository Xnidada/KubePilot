import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Typography, Select, Row, Col, Statistic, Progress,
  Modal, Form, InputNumber, message
} from 'antd'
import { ReloadOutlined, SettingOutlined, DollarOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { get, post } from '../../api/request'

const { Title, Text } = Typography

interface NamespaceCost {
  namespace: string
  cpu_request: number
  memory_request: number
  pod_count: number
  cpu_cost: number
  memory_cost: number
  total_cost: number
}

interface CostConfig {
  cpu_per_unit: number
  mem_per_unit: number
  gpu_per_unit: number
  currency: string
}

const ResourceCost: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [costs, setCosts] = useState<NamespaceCost[]>([])
  const [totalCost, setTotalCost] = useState(0)
  const [currency, setCurrency] = useState('USD')
  const [loading, setLoading] = useState(false)
  const [configModalVisible, setConfigModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchCosts(); fetchConfig() } }, [selectedCluster])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
      if (res.data?.length > 0) setSelectedCluster(res.data[0].id)
    } catch (e) { console.error(e) }
  }

  const fetchConfig = async () => {
    try {
      const res = await get<{ code: number; data: CostConfig }>(`/clusters/${selectedCluster}/cost/config`)
      if (res.data) {
        form.setFieldsValue(res.data)
      }
    } catch (e) { console.error(e) }
  }

  const fetchCosts = async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: { namespaces: NamespaceCost[]; total_cost: number; currency: string } }>(
        `/clusters/${selectedCluster}/cost/analysis`
      )
      if (res.data) {
        setCosts(res.data.namespaces || [])
        setTotalCost(res.data.total_cost || 0)
        setCurrency(res.data.currency || 'USD')
      }
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const handleSaveConfig = async (values: any) => {
    try {
      await post(`/clusters/${selectedCluster}/cost/config`, values)
      message.success('成本配置已保存')
      setConfigModalVisible(false)
      fetchCosts()
    } catch (e) {
      message.error('保存失败')
    }
  }

  const columns: ColumnsType<NamespaceCost> = [
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: 'CPU 请求', dataIndex: 'cpu_request', key: 'cpu',
      render: (v) => `${v}m`
    },
    {
      title: '内存请求', dataIndex: 'memory_request', key: 'memory',
      render: (v) => `${(v / 1024).toFixed(1)}Gi`
    },
    { title: 'Pod 数', dataIndex: 'pod_count', key: 'pods' },
    {
      title: 'CPU 成本', dataIndex: 'cpu_cost', key: 'cpu_cost',
      render: (v) => <Text>{currency} {v.toFixed(2)}</Text>
    },
    {
      title: '内存成本', dataIndex: 'memory_cost', key: 'mem_cost',
      render: (v) => <Text>{currency} {v.toFixed(2)}</Text>
    },
    {
      title: '总成本', dataIndex: 'total_cost', key: 'total',
      render: (v) => <Text strong>{currency} {v.toFixed(2)}</Text>,
      sorter: (a, b) => a.total_cost - b.total_cost,
    },
    {
      title: '占比', key: 'ratio',
      render: (_, r) => (
        <Progress
          percent={totalCost > 0 ? Math.round((r.total_cost / totalCost) * 100) : 0}
          size="small"
          style={{ width: 100 }}
        />
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>💰 资源成本分析</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Button icon={<SettingOutlined />} onClick={() => setConfigModalVisible(true)}>
            成本配置
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchCosts}>刷新</Button>
        </Space>
      </div>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={8}>
          <Card>
            <Statistic
              title="总成本（月估算）"
              value={totalCost}
              prefix={currency === 'USD' ? '$' : '¥'}
              precision={2}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="命名空间数"
              value={costs.length}
              prefix={<DollarOutlined />}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="平均成本"
              value={costs.length > 0 ? totalCost / costs.length : 0}
              prefix={currency === 'USD' ? '$' : '¥'}
              precision={2}
            />
          </Card>
        </Col>
      </Row>

      <Card>
        <Table columns={columns} dataSource={costs} rowKey="namespace" loading={loading} />
      </Card>

      {/* 成本配置弹窗 */}
      <Modal
        title="成本配置"
        open={configModalVisible}
        onCancel={() => setConfigModalVisible(false)}
        onOk={() => form.submit()}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleSaveConfig}>
          <Form.Item
            name="cpu_per_unit"
            label="CPU 单价（每核每小时）"
            rules={[{ required: true }]}
          >
            <InputNumber
              min={0}
              step={0.001}
              style={{ width: '100%' }}
              addonBefore="$"
              addonAfter="/核/小时"
            />
          </Form.Item>
          <Form.Item
            name="mem_per_unit"
            label="内存单价（每 GB 每小时）"
            rules={[{ required: true }]}
          >
            <InputNumber
              min={0}
              step={0.001}
              style={{ width: '100%' }}
              addonBefore="$"
              addonAfter="/GB/小时"
            />
          </Form.Item>
          <Form.Item
            name="gpu_per_unit"
            label="GPU 单价（每卡每小时）"
            rules={[{ required: true }]}
          >
            <InputNumber
              min={0}
              step={0.1}
              style={{ width: '100%' }}
              addonBefore="$"
              addonAfter="/卡/小时"
            />
          </Form.Item>
          <Form.Item
            name="currency"
            label="货币单位"
          >
            <Select
              options={[
                { label: 'USD ($)', value: 'USD' },
                { label: 'CNY (¥)', value: 'CNY' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ResourceCost
