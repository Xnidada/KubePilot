import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Typography, message, Tag, Modal, Form, Input, Select,
  Tabs, Popconfirm
} from 'antd'
import {
  PlusOutlined, DeleteOutlined, CloudDownloadOutlined,
  HistoryOutlined, ScheduleOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import { get, post, del } from '../../api/request'

const { Title } = Typography

interface BackupRecord {
  id: number
  backup_name: string
  cluster_id: number
  namespaces: string
  resources: string
  status: string
  volume_snapshots: number
  errors: number
  warnings: number
  started_at: string
  completed_at: string
  created_at: string
}

interface BackupSchedule {
  id: number
  name: string
  cluster_id: number
  namespaces: string
  resources: string
  schedule: string
  ttl: string
  status: string
  last_backup: string
  created_at: string
}

const Backup: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [backups, setBackups] = useState<BackupRecord[]>([])
  const [schedules, setSchedules] = useState<BackupSchedule[]>([])
  const [loading, setLoading] = useState(false)
  const [backupModalVisible, setBackupModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => { fetchClusters(); fetchBackups(); fetchSchedules() }, [])

  const fetchClusters = async () => {
    try {
      const res = await getClusterList(1, 100)
      setClusters(res.data || [])
    } catch (e) { console.error(e) }
  }

  const fetchBackups = async () => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: BackupRecord[] }>('/backups')
      setBackups(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }

  const fetchSchedules = async () => {
    try {
      const res = await get<{ code: number; data: BackupSchedule[] }>('/backups/schedules')
      setSchedules(res.data || [])
    } catch (e) { console.error(e) }
  }

  const handleCreateBackup = async (values: any) => {
    try {
      await post('/backups', {
        cluster_id: values.cluster_id,
        backup_name: values.backup_name,
        namespaces: values.namespaces ? values.namespaces.split(',').map((s: string) => s.trim()) : [],
        ttl: values.ttl || '720h',
      })
      message.success('备份已创建')
      setBackupModalVisible(false)
      form.resetFields()
      fetchBackups()
    } catch (e) { message.error('创建失败') }
  }

  const backupColumns: ColumnsType<BackupRecord> = [
    { title: '备份名称', dataIndex: 'backup_name', key: 'name' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => (
        <Tag color={s === 'completed' ? 'success' : s === 'failed' ? 'error' : 'processing'}>
          {s === 'completed' ? '完成' : s === 'failed' ? '失败' : s === 'in_progress' ? '进行中' : '等待中'}
        </Tag>
      )
    },
    { title: '快照数', dataIndex: 'volume_snapshots', key: 'snapshots' },
    { title: '错误', dataIndex: 'errors', key: 'errors', render: (v) => v > 0 ? <Tag color="error">{v}</Tag> : <Tag>0</Tag> },
    { title: '警告', dataIndex: 'warnings', key: 'warnings', render: (v) => v > 0 ? <Tag color="warning">{v}</Tag> : <Tag>0</Tag> },
    {
      title: '开始时间', dataIndex: 'started_at', key: 'started_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
    {
      title: '完成时间', dataIndex: 'completed_at', key: 'completed_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
  ]

  const scheduleColumns: ColumnsType<BackupSchedule> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '调度', dataIndex: 'schedule', key: 'schedule', render: (s) => <Tag>{s}</Tag> },
    { title: '保留时间', dataIndex: 'ttl', key: 'ttl' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => <Tag color={s === 'active' ? 'success' : 'default'}>{s === 'active' ? '活跃' : s}</Tag>
    },
    {
      title: '最后备份', dataIndex: 'last_backup', key: 'last_backup',
      render: (t) => t ? new Date(t).toLocaleString() : '从未'
    },
    {
      title: '操作', key: 'action', width: 80,
      render: (_, record) => (
        <Popconfirm title="确定删除？" onConfirm={async () => {
          await del(`/backups/schedules/${record.id}`)
          message.success('已删除')
          fetchSchedules()
        }}>
          <Button type="link" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}><CloudDownloadOutlined /> 备份管理</Title>

      <Tabs
        items={[
          {
            key: 'backups',
            label: <span><HistoryOutlined /> 备份记录</span>,
            children: (
              <Card
                extra={
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); setBackupModalVisible(true) }}>
                    创建备份
                  </Button>
                }
              >
                <Table columns={backupColumns} dataSource={backups} rowKey="id" loading={loading} />
              </Card>
            ),
          },
          {
            key: 'schedules',
            label: <span><ScheduleOutlined /> 备份计划</span>,
            children: (
              <Card>
                <Table columns={scheduleColumns} dataSource={schedules} rowKey="id" />
              </Card>
            ),
          },
        ]}
      />

      <Modal
        title="创建备份"
        open={backupModalVisible}
        onCancel={() => { setBackupModalVisible(false); form.resetFields() }}
        onOk={() => form.submit()}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleCreateBackup}>
          <Form.Item name="cluster_id" label="集群" rules={[{ required: true }]}>
            <Select options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          </Form.Item>
          <Form.Item name="backup_name" label="备份名称" rules={[{ required: true }]}>
            <Input placeholder="例如: daily-backup-2024" />
          </Form.Item>
          <Form.Item name="namespaces" label="命名空间（逗号分隔，留空为全量）">
            <Input placeholder="例如: default,kube-system" />
          </Form.Item>
          <Form.Item name="ttl" label="保留时间" initialValue="720h">
            <Select options={[
              { label: '24 小时', value: '24h' },
              { label: '7 天', value: '168h' },
              { label: '30 天', value: '720h' },
              { label: '90 天', value: '2160h' },
            ]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Backup
