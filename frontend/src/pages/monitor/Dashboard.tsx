import { useEffect, useState, useRef } from 'react'
import { Card, Row, Col, Statistic, Typography, Select, Table, Tag, Space, Progress } from 'antd'
import {
  ClusterOutlined,
  CloudServerOutlined,
  AppstoreOutlined,
  DashboardOutlined,
} from '@ant-design/icons'
import * as echarts from 'echarts'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import {
  getClusterOverview,
  getDeploymentMetrics,
  getNodeMetrics,
  ClusterOverview,
  DeploymentMetric,
  NodeMetric,
} from '../../api/metrics'

const { Title, Text } = Typography

const MonitorDashboard: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [overview, setOverview] = useState<ClusterOverview | null>(null)
  const [deployMetrics, setDeployMetrics] = useState<DeploymentMetric[]>([])
  const [nodeMetrics, setNodeMetrics] = useState<NodeMetric[]>([])

  const cpuChartRef = useRef<HTMLDivElement>(null)
  const memChartRef = useRef<HTMLDivElement>(null)
  const podChartRef = useRef<HTMLDivElement>(null)
  const deployChartRef = useRef<HTMLDivElement>(null)

  const cpuChartInstance = useRef<echarts.ECharts | null>(null)
  const memChartInstance = useRef<echarts.ECharts | null>(null)
  const podChartInstance = useRef<echarts.ECharts | null>(null)
  const deployChartInstance = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    fetchClusters()

    // 窗口大小调整时重新渲染图表
    const handleResize = () => {
      cpuChartInstance.current?.resize()
      memChartInstance.current?.resize()
      podChartInstance.current?.resize()
      deployChartInstance.current?.resize()
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      // 清理图表实例
      cpuChartInstance.current?.dispose()
      memChartInstance.current?.dispose()
      podChartInstance.current?.dispose()
      deployChartInstance.current?.dispose()
    }
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchMetrics()
    }
  }, [selectedCluster])

  useEffect(() => {
    if (overview) {
      // 延迟渲染图表，确保DOM已准备好
      setTimeout(() => {
        renderCharts()
      }, 100)
    }
  }, [overview, deployMetrics, nodeMetrics])

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

  const fetchMetrics = async () => {
    try {
      const [overviewRes, deployRes, nodeRes] = await Promise.all([
        getClusterOverview(selectedCluster),
        getDeploymentMetrics(selectedCluster),
        getNodeMetrics(selectedCluster),
      ])
      setOverview(overviewRes.data)
      setDeployMetrics(deployRes.data || [])
      setNodeMetrics(nodeRes.data || [])
    } catch (error) {
      console.error('Failed to fetch metrics:', error)
    }
  }

  const renderCharts = () => {
    if (!overview) return

    // CPU 使用率图表
    if (cpuChartRef.current) {
      if (!cpuChartInstance.current) {
        cpuChartInstance.current = echarts.init(cpuChartRef.current)
      }
      cpuChartInstance.current.setOption({
        title: { text: 'CPU 使用率', left: 'center' },
        tooltip: { trigger: 'item' },
        series: [{
          type: 'gauge',
          startAngle: 180,
          endAngle: 0,
          min: 0,
          max: 100,
          splitNumber: 10,
          axisLine: {
            lineStyle: {
              width: 30,
              color: [
                [0.3, '#67e0e3'],
                [0.7, '#37a2da'],
                [1, '#fd666d'],
              ],
            },
          },
          pointer: { itemStyle: { color: 'auto' } },
          axisTick: { distance: -30, length: 8, lineStyle: { color: '#fff', width: 2 } },
          splitLine: { distance: -30, length: 30, lineStyle: { color: '#fff', width: 4 } },
          axisLabel: { color: 'inherit', distance: 40, fontSize: 14 },
          detail: {
            valueAnimation: true,
            formatter: '{value}%',
            color: 'inherit',
            fontSize: 24,
            offsetCenter: [0, '70%'],
          },
          data: [{ value: overview.cpu_allocated_percent.toFixed(1) }],
        }],
      })
    }

    // 内存使用率图表
    if (memChartRef.current) {
      if (!memChartInstance.current) {
        memChartInstance.current = echarts.init(memChartRef.current)
      }
      memChartInstance.current.setOption({
        title: { text: '内存使用率', left: 'center' },
        tooltip: { trigger: 'item' },
        series: [{
          type: 'gauge',
          startAngle: 180,
          endAngle: 0,
          min: 0,
          max: 100,
          splitNumber: 10,
          axisLine: {
            lineStyle: {
              width: 30,
              color: [
                [0.3, '#91cc75'],
                [0.7, '#fac858'],
                [1, '#ee6666'],
              ],
            },
          },
          pointer: { itemStyle: { color: 'auto' } },
          axisTick: { distance: -30, length: 8, lineStyle: { color: '#fff', width: 2 } },
          splitLine: { distance: -30, length: 30, lineStyle: { color: '#fff', width: 4 } },
          axisLabel: { color: 'inherit', distance: 40, fontSize: 14 },
          detail: {
            valueAnimation: true,
            formatter: '{value}%',
            color: 'inherit',
            fontSize: 24,
            offsetCenter: [0, '70%'],
          },
          data: [{ value: overview.memory_allocated_percent.toFixed(1) }],
        }],
      })
    }

    // Pod 状态图表
    if (podChartRef.current) {
      if (!podChartInstance.current) {
        podChartInstance.current = echarts.init(podChartRef.current)
      }
      podChartInstance.current.setOption({
        title: { text: 'Pod 状态分布', left: 'center' },
        tooltip: { trigger: 'item' },
        legend: { bottom: 10 },
        series: [{
          type: 'pie',
          radius: ['40%', '70%'],
          avoidLabelOverlap: false,
          itemStyle: { borderRadius: 10, borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          emphasis: {
            label: { show: true, fontSize: 16, fontWeight: 'bold' },
          },
          labelLine: { show: false },
          data: [
            { value: overview.pod_running, name: 'Running', itemStyle: { color: '#52c41a' } },
            { value: overview.pod_pending, name: 'Pending', itemStyle: { color: '#faad14' } },
            { value: overview.pod_succeeded, name: 'Succeeded', itemStyle: { color: '#1890ff' } },
            { value: overview.pod_failed, name: 'Failed', itemStyle: { color: '#ff4d4f' } },
          ].filter(item => item.value > 0),
        }],
      })
    }

    // Deployment 资源使用图表
    if (deployChartRef.current && deployMetrics.length > 0) {
      if (!deployChartInstance.current) {
        deployChartInstance.current = echarts.init(deployChartRef.current)
      }

      const names = deployMetrics.map(d => d.name)
      const cpuUsage = deployMetrics.map(d => d.cpu_usage_m || d.cpu_request_m)
      const memUsage = deployMetrics.map(d => d.memory_usage_mi || d.memory_request_mi)

      deployChartInstance.current.setOption({
        title: { text: 'Deployment 资源使用', left: 'center' },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
        },
        legend: { bottom: 10 },
        grid: { left: '3%', right: '4%', bottom: '15%', top: '15%', containLabel: true },
        xAxis: {
          type: 'category',
          data: names,
          axisLabel: { rotate: 30, fontSize: 10 },
        },
        yAxis: [
          { type: 'value', name: 'CPU (millicores)', position: 'left' },
          { type: 'value', name: 'Memory (Mi)', position: 'right' },
        ],
        series: [
          {
            name: 'CPU',
            type: 'bar',
            data: cpuUsage,
            itemStyle: { color: '#1890ff' },
          },
          {
            name: 'Memory',
            type: 'bar',
            yAxisIndex: 1,
            data: memUsage,
            itemStyle: { color: '#52c41a' },
          },
        ],
      })
    }
  }

  // 节点表格列
  const nodeColumns: ColumnsType<NodeMetric> = [
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
      dataIndex: 'ready',
      key: 'ready',
      render: (ready) => (
        <Tag color={ready ? 'success' : 'error'}>
          {ready ? 'Ready' : 'NotReady'}
        </Tag>
      ),
    },
    {
      title: 'CPU 分配率',
      key: 'cpu',
      render: (_, record) => (
        <Space direction="vertical" size={0} style={{ width: 120 }}>
          <Text style={{ fontSize: 12 }}>
            {record.cpu_allocated_m}m / {record.cpu_capacity_m}m
          </Text>
          <Progress
            percent={record.cpu_allocated_percent}
            size="small"
            status={record.cpu_allocated_percent > 80 ? 'exception' : 'normal'}
          />
        </Space>
      ),
    },
    {
      title: '内存 分配率',
      key: 'memory',
      render: (_, record) => (
        <Space direction="vertical" size={0} style={{ width: 120 }}>
          <Text style={{ fontSize: 12 }}>
            {record.memory_allocated_mi}Mi / {record.memory_capacity_mi}Mi
          </Text>
          <Progress
            percent={record.memory_allocated_percent}
            size="small"
            status={record.memory_allocated_percent > 80 ? 'exception' : 'normal'}
          />
        </Space>
      ),
    },
    {
      title: 'Pod 数量',
      key: 'pods',
      render: (_, record) => (
        <Text>{record.pod_count} / {record.pod_capacity}</Text>
      ),
    },
  ]

  // Deployment 表格列
  const deployColumns: ColumnsType<DeploymentMetric> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
    },
    {
      title: '副本',
      key: 'replicas',
      render: (_, record) => (
        <Tag color={record.ready === record.replicas ? 'success' : 'warning'}>
          {record.ready}/{record.replicas}
        </Tag>
      ),
    },
    {
      title: 'CPU 使用',
      key: 'cpu',
      render: (_, record) => (
        <Space direction="vertical" size={0} style={{ width: 120 }}>
          <Text style={{ fontSize: 12 }}>
            {record.cpu_usage_m || record.cpu_request_m}m
          </Text>
          {record.cpu_usage_percent > 0 && (
            <Progress
              percent={record.cpu_usage_percent}
              size="small"
              status={record.cpu_usage_percent > 80 ? 'exception' : 'normal'}
            />
          )}
        </Space>
      ),
    },
    {
      title: '内存 使用',
      key: 'memory',
      render: (_, record) => (
        <Space direction="vertical" size={0} style={{ width: 120 }}>
          <Text style={{ fontSize: 12 }}>
            {record.memory_usage_mi || record.memory_request_mi}Mi
          </Text>
          {record.memory_usage_percent > 0 && (
            <Progress
              percent={record.memory_usage_percent}
              size="small"
              status={record.memory_usage_percent > 80 ? 'exception' : 'normal'}
            />
          )}
        </Space>
      ),
    },
    {
      title: 'Pod 数',
      dataIndex: 'pod_count',
      key: 'pod_count',
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>监控概览</Title>
        <Select
          value={selectedCluster}
          onChange={setSelectedCluster}
          style={{ width: 200 }}
          placeholder="选择集群"
          options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
        />
      </div>

      {/* 集群概览卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="节点数量"
              value={overview?.node_count || 0}
              prefix={<ClusterOutlined style={{ color: '#1890ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Deployment"
              value={overview?.deployment_count || 0}
              prefix={<AppstoreOutlined style={{ color: '#52c41a' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Pod 总数"
              value={overview?.pod_count || 0}
              prefix={<CloudServerOutlined style={{ color: '#722ed1' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="运行中"
              value={overview?.pod_running || 0}
              prefix={<DashboardOutlined style={{ color: '#52c41a' }} />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
      </Row>

      {/* 资源使用图表 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={8}>
          <Card>
            <div ref={cpuChartRef} style={{ height: 300 }} />
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card>
            <div ref={memChartRef} style={{ height: 300 }} />
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card>
            <div ref={podChartRef} style={{ height: 300 }} />
          </Card>
        </Col>
      </Row>

      {/* Deployment 资源图表 */}
      {deployMetrics.length > 0 && (
        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col span={24}>
            <Card>
              <div ref={deployChartRef} style={{ height: 400 }} />
            </Card>
          </Col>
        </Row>
      )}

      {/* 节点详情表格 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <Card title="节点资源详情">
            <Table
              columns={nodeColumns}
              dataSource={nodeMetrics}
              rowKey="name"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>

      {/* Deployment 资源表格 */}
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title="Deployment 资源详情">
            <Table
              columns={deployColumns}
              dataSource={deployMetrics}
              rowKey={(record) => `${record.namespace}/${record.name}`}
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default MonitorDashboard
