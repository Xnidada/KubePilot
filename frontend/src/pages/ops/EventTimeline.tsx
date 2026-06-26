import { useState, useEffect } from 'react'
import {
  Card, Button, Space, Typography, Select, Tag, Timeline, Row, Col, Statistic, Empty
} from 'antd'
import {
  ReloadOutlined, WarningOutlined, CheckCircleOutlined, ClockCircleOutlined
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import { getNamespaceNames } from '../../api/workload'
import { getEventTimeline, TimelineEvent } from '../../api/ops'

const { Title, Text } = Typography

const EventTimelinePage: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [namespaces, setNamespaces] = useState<string[]>([])
  const [selectedNamespace, setSelectedNamespace] = useState<string>('')
  const [hours, setHours] = useState<number>(24)
  const [events, setEvents] = useState<TimelineEvent[]>([])
  const [warningCount, setWarningCount] = useState(0)
  const [loading, setLoading] = useState(false)

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (selectedCluster) { fetchNamespaces(); fetchEvents() } }, [selectedCluster, selectedNamespace, hours])

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

  const fetchEvents = async () => {
    setLoading(true)
    try {
      const res = await getEventTimeline(selectedCluster, selectedNamespace || undefined, hours)
      setEvents(res.data.events || [])
      setWarningCount(res.data.warning_count || 0)
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const getTimelineDot = (type: string) => {
    if (type === 'Warning') {
      return <WarningOutlined style={{ color: '#ff4d4f', fontSize: 16 }} />
    }
    return <CheckCircleOutlined style={{ color: '#52c41a', fontSize: 16 }} />
  }

  const getTimelineColor = (type: string) => {
    return type === 'Warning' ? 'red' : 'green'
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>📅 事件时间线</Title>
        <Space>
          <Select value={selectedCluster} onChange={setSelectedCluster} style={{ width: 200 }}
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          <Select value={selectedNamespace} onChange={setSelectedNamespace} style={{ width: 150 }}
            placeholder="所有命名空间" allowClear
            options={namespaces.map(ns => ({ label: ns, value: ns }))} />
          <Select value={hours} onChange={setHours} style={{ width: 120 }}
            options={[
              { label: '最近 1 小时', value: 1 },
              { label: '最近 6 小时', value: 6 },
              { label: '最近 12 小时', value: 12 },
              { label: '最近 24 小时', value: 24 },
              { label: '最近 3 天', value: 72 },
              { label: '最近 7 天', value: 168 },
            ]} />
          <Button icon={<ReloadOutlined />} onClick={fetchEvents}>刷新</Button>
        </Space>
      </div>

      {/* 统计卡片 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={8}>
          <Card>
            <Statistic title="事件总数" value={events.length} prefix={<ClockCircleOutlined />} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="警告事件" value={warningCount} prefix={<WarningOutlined />}
              valueStyle={{ color: warningCount > 0 ? '#ff4d4f' : '#52c41a' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="正常事件" value={events.length - warningCount} prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
      </Row>

      <Card title="事件时间线" loading={loading}>
        {events.length === 0 ? (
          <Empty description="暂无事件" />
        ) : (
          <Timeline
            items={events.slice(0, 100).map((event) => ({
              dot: getTimelineDot(event.type),
              color: getTimelineColor(event.type),
              children: (
                <div style={{ marginBottom: 16 }}>
                  <Space style={{ marginBottom: 8 }}>
                    <Tag color={event.type === 'Warning' ? 'error' : 'default'}>{event.type}</Tag>
                    <Tag>{event.reason}</Tag>
                    <Tag color="blue">{event.resource_kind}/{event.resource_name}</Tag>
                    {event.namespace && <Tag>{event.namespace}</Tag>}
                    {event.count > 1 && <Tag>×{event.count}</Tag>}
                  </Space>
                  <div>
                    <Text>{event.message}</Text>
                  </div>
                  <div style={{ marginTop: 4 }}>
                    <Text type="secondary" style={{ fontSize: 12 }}>{event.time}</Text>
                  </div>
                </div>
              )
            }))}
          />
        )}
      </Card>
    </div>
  )
}

export default EventTimelinePage
