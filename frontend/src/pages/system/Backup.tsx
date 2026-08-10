import { useState, useEffect, useCallback } from 'react'
import {
  Card, Table, Button, Typography, message, Tag, Modal, Form, Input, Select,
  Tabs, Popconfirm, Space, Tooltip, Switch
} from 'antd'
import {
  PlusOutlined, DeleteOutlined, CloudDownloadOutlined,
  HistoryOutlined, ScheduleOutlined, PauseCircleOutlined,
  PlayCircleOutlined, ClearOutlined, EditOutlined, RollbackOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getClusterList, Cluster } from '../../api/cluster'
import {
  listBackupSchedules,
  createBackupSchedule,
  updateBackupSchedule,
  deleteBackupSchedule,
  pauseBackupSchedule,
  resumeBackupSchedule,
  clearBackupCron,
  listBackupRecords,
  createBackupRecord,
  listRestoreRecords,
  createRestore,
  BackupScheduleItem,
  BackupRecordItem,
  RestoreRecordItem,
} from '../../api/system'
import { useQueryTab } from '../../hooks/useQueryTab'
import { useInterval } from '../../hooks/useInterval'
import { ModuleHealthAlert } from '../../components/ModuleHealthAlert'

const { Title, Text } = Typography

const BACKUP_TABS = ['schedules', 'backups', 'restores'] as const
const REFRESH_MS = 10000

function parseJSONArray(raw?: string): string {
  if (!raw) return ''
  try {
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? arr.join(',') : ''
  } catch {
    return raw
  }
}

const Backup: React.FC = () => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [backups, setBackups] = useState<BackupRecordItem[]>([])
  const [schedules, setSchedules] = useState<BackupScheduleItem[]>([])
  const [restores, setRestores] = useState<RestoreRecordItem[]>([])
  const [loading, setLoading] = useState(false)
  const [backupModalVisible, setBackupModalVisible] = useState(false)
  const [scheduleModalVisible, setScheduleModalVisible] = useState(false)
  const [restoreModalVisible, setRestoreModalVisible] = useState(false)
  const [restoreBackup, setRestoreBackup] = useState<BackupRecordItem | null>(null)
  const [editingSchedule, setEditingSchedule] = useState<BackupScheduleItem | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [activeTab, setActiveTab] = useQueryTab(BACKUP_TABS, 'schedules')
  const [form] = Form.useForm()
  const [scheduleForm] = Form.useForm()
  const [restoreForm] = Form.useForm()

  const fetchSchedules = useCallback(async () => {
    try {
      const res = await listBackupSchedules()
      setSchedules(res.data || [])
    } catch (e) { console.error(e) }
  }, [])

  const fetchBackups = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listBackupRecords()
      setBackups(res.data || [])
    } catch (e) { console.error(e) }
    finally { setLoading(false) }
  }, [])

  const fetchRestores = useCallback(async () => {
    try {
      const res = await listRestoreRecords()
      setRestores(res.data || [])
    } catch (e) { console.error(e) }
  }, [])

  const refreshAll = useCallback(() => {
    fetchSchedules()
    fetchBackups()
    fetchRestores()
  }, [fetchSchedules, fetchBackups, fetchRestores])

  useEffect(() => {
    getClusterList(1, 100).then((res) => setClusters(res.data || [])).catch(console.error)
    refreshAll()
  }, [refreshAll])

  useInterval(() => {
    if (activeTab === 'schedules') fetchSchedules()
    else if (activeTab === 'backups') fetchBackups()
    else fetchRestores()
  }, REFRESH_MS, autoRefresh)

  const openCreateSchedule = () => {
    setEditingSchedule(null)
    scheduleForm.resetFields()
    scheduleForm.setFieldsValue({ ttl: '720h', schedule: '0 2 * * *', status: 'active' })
    setScheduleModalVisible(true)
  }

  const openEditSchedule = (record: BackupScheduleItem) => {
    setEditingSchedule(record)
    scheduleForm.setFieldsValue({
      name: record.name,
      cluster_id: record.cluster_id,
      schedule: record.schedule || '',
      namespaces: parseJSONArray(record.namespaces),
      ttl: record.ttl || '720h',
      storage_location: record.storage_location || '',
      status: record.status || 'active',
    })
    setScheduleModalVisible(true)
  }

  const handleCreateBackup = async (values: any) => {
    try {
      await createBackupRecord({
        cluster_id: values.cluster_id,
        backup_name: values.backup_name,
        namespaces: values.namespaces ? values.namespaces.split(',').map((s: string) => s.trim()) : [],
        ttl: values.ttl || '720h',
      })
      message.success('备份已创建')
      setBackupModalVisible(false)
      form.resetFields()
      fetchBackups()
      setActiveTab('backups')
    } catch (e) { message.error('创建失败') }
  }

  const handleSaveSchedule = async (values: any) => {
    const namespaces = values.namespaces
      ? values.namespaces.split(',').map((s: string) => s.trim()).filter(Boolean)
      : []
    try {
      if (editingSchedule) {
        await updateBackupSchedule(editingSchedule.id, {
          name: values.name,
          schedule: values.schedule ?? '',
          namespaces,
          ttl: values.ttl || '720h',
          storage_location: values.storage_location || '',
          status: values.status || 'active',
        })
        message.success('备份计划已更新')
      } else {
        await createBackupSchedule({
          name: values.name,
          cluster_id: values.cluster_id,
          schedule: values.schedule,
          namespaces,
          ttl: values.ttl || '720h',
          storage_location: values.storage_location || '',
        })
        message.success('备份计划已创建')
      }
      setScheduleModalVisible(false)
      setEditingSchedule(null)
      scheduleForm.resetFields()
      fetchSchedules()
    } catch (e: any) {
      message.error(e?.message || (editingSchedule ? '更新失败' : '创建计划失败'))
    }
  }

  const openRestore = (record: BackupRecordItem) => {
    setRestoreBackup(record)
    restoreForm.resetFields()
    restoreForm.setFieldsValue({
      backup_id: record.id,
      cluster_id: record.cluster_id,
      namespaces: parseJSONArray(record.namespaces),
    })
    setRestoreModalVisible(true)
  }

  const handleCreateRestore = async (values: any) => {
    try {
      await createRestore({
        backup_id: values.backup_id,
        cluster_id: values.cluster_id,
        namespaces: values.namespaces
          ? values.namespaces.split(',').map((s: string) => s.trim()).filter(Boolean)
          : [],
      })
      message.success('恢复任务已创建')
      setRestoreModalVisible(false)
      setRestoreBackup(null)
      restoreForm.resetFields()
      fetchRestores()
      setActiveTab('restores')
    } catch (e: any) {
      message.error(e?.message || '创建恢复失败')
    }
  }

  const backupColumns: ColumnsType<BackupRecordItem> = [
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
    {
      title: '开始时间', dataIndex: 'started_at', key: 'started_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
    {
      title: '操作', key: 'action', width: 100,
      render: (_, record) => (
        <Tooltip title={record.status === 'completed' ? '从此备份恢复' : '仅完成状态可恢复'}>
          <Button
            type="link"
            icon={<RollbackOutlined />}
            disabled={record.status !== 'completed'}
            onClick={() => openRestore(record)}
          />
        </Tooltip>
      ),
    },
  ]

  const scheduleColumns: ColumnsType<BackupScheduleItem> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '调度', dataIndex: 'schedule', key: 'schedule',
      render: (s) => s ? <Tag>{s}</Tag> : <Tag>手动</Tag>
    },
    { title: '保留时间', dataIndex: 'ttl', key: 'ttl' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => (
        <Tag color={s === 'active' ? 'success' : 'default'}>
          {s === 'active' ? '活跃' : s === 'paused' ? '已暂停' : s}
        </Tag>
      )
    },
    {
      title: '最后备份', dataIndex: 'last_backup', key: 'last_backup',
      render: (t) => t ? new Date(t).toLocaleString() : '从未'
    },
    {
      title: '操作', key: 'action', width: 220,
      render: (_, record) => (
        <Space size={0}>
          <Tooltip title="编辑">
            <Button type="link" icon={<EditOutlined />} onClick={() => openEditSchedule(record)} />
          </Tooltip>
          {record.status === 'active' ? (
            <Tooltip title="暂停">
              <Button type="link" icon={<PauseCircleOutlined />} onClick={async () => {
                try { await pauseBackupSchedule(record.id); message.success('已暂停'); fetchSchedules() }
                catch { message.error('暂停失败') }
              }} />
            </Tooltip>
          ) : (
            <Tooltip title={record.schedule ? '恢复' : '请先设置 cron'}>
              <Button
                type="link"
                icon={<PlayCircleOutlined />}
                disabled={!record.schedule}
                onClick={async () => {
                  try { await resumeBackupSchedule(record.id); message.success('已恢复'); fetchSchedules() }
                  catch (e: any) { message.error(e?.message || '恢复失败') }
                }}
              />
            </Tooltip>
          )}
          {record.schedule ? (
            <Popconfirm title="清空 cron 并暂停？" onConfirm={async () => {
              try { await clearBackupCron(record.id); message.success('已清空调度'); fetchSchedules() }
              catch { message.error('清空失败') }
            }}>
              <Tooltip title="清空调度">
                <Button type="link" icon={<ClearOutlined />} />
              </Tooltip>
            </Popconfirm>
          ) : null}
          <Popconfirm title="确定删除？" onConfirm={async () => {
            await deleteBackupSchedule(record.id)
            message.success('已删除')
            fetchSchedules()
          }}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const restoreColumns: ColumnsType<RestoreRecordItem> = [
    { title: '恢复名称', dataIndex: 'restore_name', key: 'name' },
    { title: '备份 ID', dataIndex: 'backup_id', key: 'backup_id', width: 90 },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s) => (
        <Tag color={s === 'completed' ? 'success' : s === 'failed' ? 'error' : 'processing'}>
          {s === 'completed' ? '完成' : s === 'failed' ? '失败' : s === 'in_progress' ? '进行中' : '等待中'}
        </Tag>
      )
    },
    { title: '错误', dataIndex: 'errors', key: 'errors', render: (v) => v || 0 },
    {
      title: '开始时间', dataIndex: 'started_at', key: 'started_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
    {
      title: '完成时间', dataIndex: 'completed_at', key: 'completed_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}><CloudDownloadOutlined /> 备份管理</Title>
        <Space>
          <Space size={6}>
            <Text type="secondary" style={{ fontSize: 12 }}>自动刷新</Text>
            <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
          </Space>
          <Button onClick={refreshAll}>刷新</Button>
        </Space>
      </div>

      <ModuleHealthAlert
        module="backup"
        title="备份模块异常"
        fixPath="/system/modules"
        fixLabel="查看模块详情"
      />

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'schedules',
            label: <span><ScheduleOutlined /> 备份计划</span>,
            children: (
              <Card
                extra={
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreateSchedule}>
                    创建计划
                  </Button>
                }
              >
                <Table columns={scheduleColumns} dataSource={schedules} rowKey="id" />
              </Card>
            ),
          },
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
            key: 'restores',
            label: <span><RollbackOutlined /> 恢复记录</span>,
            children: (
              <Card>
                <Table columns={restoreColumns} dataSource={restores} rowKey="id" />
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

      <Modal
        title={editingSchedule ? '编辑备份计划' : '创建备份计划'}
        open={scheduleModalVisible}
        onCancel={() => {
          setScheduleModalVisible(false)
          setEditingSchedule(null)
          scheduleForm.resetFields()
        }}
        onOk={() => scheduleForm.submit()}
        width={520}
      >
        <Form form={scheduleForm} layout="vertical" onFinish={handleSaveSchedule}>
          <Form.Item name="name" label="计划名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如: nightly-full" />
          </Form.Item>
          <Form.Item name="cluster_id" label="集群" rules={[{ required: !editingSchedule, message: '请选择集群' }]}>
            <Select
              disabled={!!editingSchedule}
              options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))}
            />
          </Form.Item>
          <Form.Item
            name="schedule"
            label="Cron 表达式"
            rules={editingSchedule ? [] : [{ required: true, message: '请输入 cron' }]}
            extra="支持标准五段 cron 或 @every 1h / @daily；编辑时可留空表示清空调度"
          >
            <Input placeholder="0 2 * * *" allowClear={!!editingSchedule} />
          </Form.Item>
          {editingSchedule ? (
            <Form.Item name="status" label="状态">
              <Select options={[
                { label: '活跃', value: 'active' },
                { label: '已暂停', value: 'paused' },
              ]} />
            </Form.Item>
          ) : null}
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
          <Form.Item name="storage_location" label="存储位置（可选）">
            <Input placeholder="例如: default" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={restoreBackup ? `恢复：${restoreBackup.backup_name}` : '创建恢复'}
        open={restoreModalVisible}
        onCancel={() => { setRestoreModalVisible(false); setRestoreBackup(null); restoreForm.resetFields() }}
        onOk={() => restoreForm.submit()}
        width={500}
      >
        <Form form={restoreForm} layout="vertical" onFinish={handleCreateRestore}>
          <Form.Item name="backup_id" hidden><Input /></Form.Item>
          <Form.Item name="cluster_id" label="目标集群" rules={[{ required: true }]}>
            <Select options={clusters.map(c => ({ label: c.display_name || c.name, value: c.id }))} />
          </Form.Item>
          <Form.Item name="namespaces" label="命名空间（逗号分隔，留空为全量）">
            <Input placeholder="例如: default" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Backup
