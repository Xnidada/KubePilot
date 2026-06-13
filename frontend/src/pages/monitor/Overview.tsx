import { useEffect, useState } from 'react'
import { Card, Row, Col, Statistic, Typography, Select, Table, Tag, Badge } from 'antd'
import {
  AlertOutlined,
  CheckCircleOutlined,
  ArrowUpOutlined,
  CloudServerOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster, getClusterInfo, ClusterInfo } from '../../api/cluster'
import { getEvents, Event, getNodes, Node } from '../../api/workload'

const { Title } = Typography

const MonitorOverview: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [clusterInfo, setClusterInfo] = useState<ClusterInfo | null>(null)
  const [events, setEvents] = useState<Event[]>([])
  const [nodes, setNodes] = useState<Node[]>([])

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchClusterInfo()
      fetchEvents()
      fetchNodes()
    }
  }, [selectedCluster])

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

  const fetchClusterInfo = async () => {
    try {
      const res = await getClusterInfo(selectedCluster)
      setClusterInfo(res.data)
    } catch (error) {
      console.error('Failed to fetch cluster info:', error)
    }
  }

  const fetchEvents = async () => {
    try {
      const res = await getEvents(selectedCluster)
      setEvents(res.data || [])
    } catch (error) {
      console.error('Failed to fetch events:', error)
    }
  }

  const fetchNodes = async () => {
    try {
      const res = await getNodes(selectedCluster)
      setNodes(res.data || [])
    } catch (error) {
      console.error('Failed to fetch nodes:', error)
    }
  }

  const getEventTypeTag = (type: string) => {
    return type === 'Warning' ? (
      <Tag color="warning">Warning</Tag>
    ) : (
      <Tag color="processing">Normal</Tag>
    )
  }

  const nodeColumns: ColumnsType<Node> = [
    {
      title: '节点名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Badge
          status={status === 'Ready' ? 'success' : 'error'}
          text={status}
        />
      ),
    },
    {
      title: '角色',
      dataIndex: 'roles',
      key: 'roles',
    },
    {
      title: 'CPU',
      dataIndex: 'cpu_capacity',
      key: 'cpu_capacity',
    },
    {
      title: '内存',
      dataIndex: 'mem_capacity',
      key: 'mem_capacity',
    },
    {
      title: '容器运行时',
      dataIndex: 'container_rt',
      key: 'container_rt',
      ellipsis: true,
    },
    {
      title: 'Kubelet',
      dataIndex: 'kubelet_ver',
      key: 'kubelet_ver',
    },
  ]

  const eventColumns: ColumnsType<Event> = [
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type) => getEventTypeTag(type),
    },
    {
      title: '原因',
      dataIndex: 'reason',
      key: 'reason',
    },
    {
      title: '对象',
      dataIndex: 'object',
      key: 'object',
      ellipsis: true,
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
    },
    {
      title: '消息',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
    },
    {
      title: '时间',
      dataIndex: 'age',
      key: 'age',
    },
  ]

  const readyNodes = nodes.filter((n) => n.status === 'Ready').length
  const warningEvents = events.filter((e) => e.type === 'Warning').length

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>监控告警</Title>
        <Select
          value={selectedCluster}
          onChange={setSelectedCluster}
          style={{ width: 200 }}
          placeholder="选择集群"
          options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
        />
      </div>

      <Row gutter={[24, 24]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="节点总数"
              value={clusterInfo?.node_count || 0}
              prefix={<CloudServerOutlined style={{ color: '#1890ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="健康节点"
              value={readyNodes}
              prefix={<CheckCircleOutlined style={{ color: '#52c41a' }} />}
              suffix={`/ ${nodes.length}`}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="CPU 容量"
              value={clusterInfo?.cpu_capacity || '-'}
              prefix={<ArrowUpOutlined style={{ color: '#1890ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="警告事件"
              value={warningEvents}
              prefix={<AlertOutlined style={{ color: warningEvents > 0 ? '#faad14' : '#52c41a' }} />}
              valueStyle={{ color: warningEvents > 0 ? '#faad14' : '#52c41a' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[24, 24]} style={{ marginTop: 24 }}>
        <Col xs={24} lg={12}>
          <Card title={`节点列表 (${nodes.length})`}>
            <Table
              columns={nodeColumns}
              dataSource={nodes}
              rowKey="name"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title={`最近事件 (${events.length})`}>
            <Table
              columns={eventColumns}
              dataSource={events.slice(0, 20)}
              rowKey={(record, index) => `${record.object}-${index}`}
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default MonitorOverview
