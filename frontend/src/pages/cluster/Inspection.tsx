import { useState, useEffect } from 'react'
import {
  Card,
  Table,
  Button,
  Space,
  Typography,
  message,
  Tag,
  Modal,
  Form,
  Input,
  Select,
  Tabs,
  Badge,
  Tooltip,
} from 'antd'
import {
  PlusOutlined,
  PlayCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  WarningOutlined,
  FileTextOutlined,
} from '@ant-design/icons'
import { getClusterList, Cluster } from '../../api/cluster'
import {
  listInspectionRules,
  createInspectionRule,
  updateInspectionRule,
  deleteInspectionRule,
  runInspection,
  listInspectionReports,
  getInspectionResults,
  InspectionRule,
  InspectionReport,
  InspectionResult,
} from '../../api/system'

const { Title, Text } = Typography

const Inspection: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [rules, setRules] = useState<InspectionRule[]>([])
  const [reports, setReports] = useState<InspectionReport[]>([])
  const [results, setResults] = useState<InspectionResult[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedCluster, setSelectedCluster] = useState<number>(0)
  const [showRuleModal, setShowRuleModal] = useState(false)
  const [showResultsModal, setShowResultsModal] = useState(false)
  const [editingRule, setEditingRule] = useState<InspectionRule | null>(null)
  const [selectedReport, setSelectedReport] = useState<InspectionReport | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (selectedCluster) {
      fetchRules()
      fetchReports()
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

  const fetchRules = async () => {
    setLoading(true)
    try {
      const res = await listInspectionRules(selectedCluster)
      setRules(res.data || [])
    } catch (error) {
      console.error('Failed to fetch rules:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchReports = async () => {
    try {
      const res = await listInspectionReports(selectedCluster)
      setReports(res.data || [])
    } catch (error) {
      console.error('Failed to fetch reports:', error)
    }
  }

  const handleCreate = () => {
    setEditingRule(null)
    form.resetFields()
    form.setFieldsValue({ cluster_id: selectedCluster, resource: 'node', check_type: 'status' })
    setShowRuleModal(true)
  }

  const handleEdit = (rule: InspectionRule) => {
    setEditingRule(rule)
    form.setFieldsValue(rule)
    setShowRuleModal(true)
  }

  const handleSave = async () => {
    try {
      const values = await form.validateFields()
      if (editingRule) {
        await updateInspectionRule(editingRule.id, values)
        message.success('规则已更新')
      } else {
        await createInspectionRule(values)
        message.success('规则已创建')
      }
      setShowRuleModal(false)
      fetchRules()
    } catch (error) {
      message.error('保存失败')
    }
  }

  const handleDelete = async (id: number) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除此巡检规则吗？',
      onOk: async () => {
        await deleteInspectionRule(id)
        message.success('规则已删除')
        fetchRules()
      },
    })
  }

  const handleRun = async (ruleId: number) => {
    try {
      await runInspection(ruleId)
      message.success('巡检已启动')
      // 等待一下再刷新报告
      setTimeout(fetchReports, 2000)
    } catch (error) {
      message.error('启动巡检失败')
    }
  }

  const handleViewResults = async (report: InspectionReport) => {
    setSelectedReport(report)
    try {
      const res = await getInspectionResults(report.id)
      setResults(res.data || [])
      setShowResultsModal(true)
    } catch (error) {
      message.error('获取结果失败')
    }
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'pass':
        return <Tag color="success" icon={<CheckCircleOutlined />}>通过</Tag>
      case 'fail':
        return <Tag color="error" icon={<CloseCircleOutlined />}>失败</Tag>
      case 'warn':
        return <Tag color="warning" icon={<WarningOutlined />}>警告</Tag>
      case 'running':
        return <Tag color="processing">运行中</Tag>
      case 'completed':
        return <Tag color="success">完成</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const ruleColumns = [
    { title: '规则名称', dataIndex: 'name', key: 'name' },
    { title: '检查资源', dataIndex: 'resource', key: 'resource' },
    { title: '检查类型', dataIndex: 'check_type', key: 'check_type' },
    { title: '调度', dataIndex: 'schedule', key: 'schedule', render: (s: string) => s || '手动' },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: InspectionRule) => (
        <Space>
          <Tooltip title="执行巡检">
            <Button
              type="link"
              icon={<PlayCircleOutlined />}
              onClick={() => handleRun(record.id)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Tooltip title="删除">
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
              onClick={() => handleDelete(record.id)}
            />
          </Tooltip>
        </Space>
      ),
    },
  ]

  const reportColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    { title: '总检查', dataIndex: 'total_checks', key: 'total_checks' },
    {
      title: '通过',
      dataIndex: 'passed',
      key: 'passed',
      render: (v: number) => <Text type="success">{v}</Text>,
    },
    {
      title: '失败',
      dataIndex: 'failed',
      key: 'failed',
      render: (v: number) => <Text type="danger">{v}</Text>,
    },
    {
      title: '警告',
      dataIndex: 'warnings',
      key: 'warnings',
      render: (v: number) => <Text type="warning">{v}</Text>,
    },
    {
      title: '开始时间',
      dataIndex: 'started_at',
      key: 'started_at',
      render: (t: string) => new Date(t).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: InspectionReport) => (
        <Button
          type="link"
          icon={<FileTextOutlined />}
          onClick={() => handleViewResults(record)}
        >
          查看详情
        </Button>
      ),
    },
  ]

  const resultColumns = [
    { title: '资源类型', dataIndex: 'resource_type', key: 'resource_type' },
    { title: '资源名称', dataIndex: 'resource_name', key: 'resource_name' },
    { title: '命名空间', dataIndex: 'namespace', key: 'namespace' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    { title: '信息', dataIndex: 'message', key: 'message' },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>集群巡检</Title>
        <Space>
          <Select
            value={selectedCluster}
            onChange={setSelectedCluster}
            style={{ width: 200 }}
            placeholder="选择集群"
            options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
          />
          <Button icon={<ReloadOutlined />} onClick={() => { fetchRules(); fetchReports() }}>
            刷新
          </Button>
        </Space>
      </div>

      <Tabs
        items={[
          {
            key: 'rules',
            label: '巡检规则',
            children: (
              <Card
                title="巡检规则"
                extra={
                  <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
                    新建规则
                  </Button>
                }
              >
                <Table
                  columns={ruleColumns}
                  dataSource={rules}
                  rowKey="id"
                  loading={loading}
                />
              </Card>
            ),
          },
          {
            key: 'reports',
            label: '巡检报告',
            children: (
              <Card title="巡检报告">
                <Table
                  columns={reportColumns}
                  dataSource={reports}
                  rowKey="id"
                />
              </Card>
            ),
          },
        ]}
      />

      {/* 规则编辑弹窗 */}
      <Modal
        title={editingRule ? '编辑规则' : '新建规则'}
        open={showRuleModal}
        onOk={handleSave}
        onCancel={() => setShowRuleModal(false)}
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="cluster_id" hidden>
            <Input />
          </Form.Item>
          <Form.Item name="name" label="规则名称" rules={[{ required: true }]}>
            <Input placeholder="输入规则名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="输入规则描述" />
          </Form.Item>
          <Form.Item name="resource" label="检查资源" rules={[{ required: true }]}>
            <Select
              options={[
                { label: '节点 (Node)', value: 'node' },
                { label: 'Pod', value: 'pod' },
                { label: 'Deployment', value: 'deployment' },
                { label: 'Service', value: 'service' },
                { label: '自定义', value: 'custom' },
              ]}
            />
          </Form.Item>
          <Form.Item name="check_type" label="检查类型">
            <Select
              options={[
                { label: '状态检查', value: 'status' },
                { label: '资源使用', value: 'resource' },
                { label: '自定义脚本', value: 'custom' },
              ]}
            />
          </Form.Item>
          <Form.Item name="schedule" label="调度（Cron 表达式，留空为手动）">
            <Input placeholder="例如: 0 */6 * * * (每6小时)" />
          </Form.Item>
          <Form.Item name="script" label="自定义脚本">
            <Input.TextArea rows={4} placeholder="输入自定义检查脚本..." />
          </Form.Item>
        </Form>
      </Modal>

      {/* 结果详情弹窗 */}
      <Modal
        title={`巡检结果 - 报告 #${selectedReport?.id}`}
        open={showResultsModal}
        onCancel={() => setShowResultsModal(false)}
        footer={null}
        width={800}
      >
        {selectedReport && (
          <div style={{ marginBottom: 16 }}>
            <Space>
              <Badge status={selectedReport.status === 'completed' ? 'success' : 'processing'} text={selectedReport.status} />
              <Text>总检查: {selectedReport.total_checks}</Text>
              <Text type="success">通过: {selectedReport.passed}</Text>
              <Text type="danger">失败: {selectedReport.failed}</Text>
              <Text type="warning">警告: {selectedReport.warnings}</Text>
            </Space>
          </div>
        )}
        <Table
          columns={resultColumns}
          dataSource={results}
          rowKey="id"
          size="small"
        />
      </Modal>
    </div>
  )
}

export default Inspection
