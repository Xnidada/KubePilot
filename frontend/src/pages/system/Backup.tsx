import { useState, useEffect, useCallback, type Key } from 'react'
import {
  Card, Table, Button, Typography, message, Tag, Modal, Form, Input, Select,
  Tabs, Popconfirm, Space, Tooltip, Switch, Alert
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
  deleteBackupRecord,
  listRestoreRecords,
  createRestore,
  deleteRestoreRecord,
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
const VELERO_TIP_KEY = 'kubepilot.backup.veleroTipDismissed'

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
  const [selectedBackupKeys, setSelectedBackupKeys] = useState<Key[]>([])
  const [veleroTipVisible, setVeleroTipVisible] = useState(
    () => localStorage.getItem(VELERO_TIP_KEY) !== '1'
  )
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
        storage_location: values.storage_location || '',
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

  const canDeleteBackup = (record: BackupRecordItem) =>
    record.status !== 'pending' && record.status !== 'in_progress'

  const handleDeleteBackup = (record: BackupRecordItem) => {
    Modal.confirm({
      title: '删除备份',
      content: `确定删除备份「${record.backup_name}」？将同时删除集群中的 Velero Backup 对象及关联恢复记录。`,
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        try {
          await deleteBackupRecord(record.id)
          message.success('已删除')
          setSelectedBackupKeys((keys) => keys.filter((k) => k !== record.id))
          fetchBackups()
          fetchRestores()
        } catch (e: any) {
          message.error(e?.message || '删除失败')
        }
      },
    })
  }

  const handleBatchDeleteBackups = () => {
    if (selectedBackupKeys.length === 0) {
      message.warning('请先选择要删除的备份')
      return
    }
    const selected = backups.filter((b) => selectedBackupKeys.includes(b.id))
    const deletable = selected.filter(canDeleteBackup)
    const skipped = selected.length - deletable.length
    if (deletable.length === 0) {
      message.warning('选中项均为进行中/等待中，无法删除')
      return
    }
    Modal.confirm({
      title: '批量删除备份',
      content: `确定删除选中的 ${deletable.length} 个备份？${skipped > 0 ? `（跳过 ${skipped} 个进行中/等待中）` : ''}将同时删除集群中的 Velero Backup 对象及关联恢复记录。`,
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        let success = 0
        let failed = 0
        for (const item of deletable) {
          try {
            await deleteBackupRecord(item.id)
            success++
          } catch {
            failed++
          }
        }
        if (failed === 0) message.success(`成功删除 ${success} 个备份`)
        else message.warning(`删除完成：成功 ${success}，失败 ${failed}`)
        setSelectedBackupKeys([])
        fetchBackups()
        fetchRestores()
      },
    })
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
    {
      title: 'Velero Phase', dataIndex: 'phase', key: 'phase',
      render: (p) => p ? <Tag>{p}</Tag> : '-',
    },
    { title: '快照数', dataIndex: 'volume_snapshots', key: 'snapshots' },
    { title: '错误', dataIndex: 'errors', key: 'errors', render: (v) => v > 0 ? <Tag color="error">{v}</Tag> : <Tag>0</Tag> },
    {
      title: '开始时间', dataIndex: 'started_at', key: 'started_at',
      render: (t) => t ? new Date(t).toLocaleString() : '-'
    },
    {
      title: '操作', key: 'action', width: 120,
      render: (_, record) => (
        <Space size={0}>
          <Tooltip title={record.status === 'completed' ? '从此备份恢复' : '仅完成状态可恢复'}>
            <Button
              type="link"
              icon={<RollbackOutlined />}
              disabled={record.status !== 'completed'}
              onClick={() => openRestore(record)}
            />
          </Tooltip>
          <Tooltip title={canDeleteBackup(record) ? '删除' : '进行中/等待中不可删除'}>
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
              disabled={!canDeleteBackup(record)}
              onClick={() => handleDeleteBackup(record)}
            />
          </Tooltip>
        </Space>
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
    {
      title: '操作', key: 'action', width: 80,
      render: (_, record) => {
        const canDelete = record.status !== 'pending' && record.status !== 'in_progress'
        return (
          <Tooltip title={canDelete ? '删除' : '进行中/等待中不可删除'}>
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
              disabled={!canDelete}
              onClick={() => {
                Modal.confirm({
                  title: '删除恢复记录',
                  content: `确定删除恢复「${record.restore_name}」？将同时尝试删除集群中的 Velero Restore 对象。`,
                  okText: '删除',
                  okType: 'danger',
                  onOk: async () => {
                    try {
                      await deleteRestoreRecord(record.id)
                      message.success('已删除')
                      fetchRestores()
                    } catch (e: any) {
                      message.error(e?.message || '删除失败')
                    }
                  },
                })
              }}
            />
          </Tooltip>
        )
      },
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

      {veleroTipVisible && (
        <Alert
          type="warning"
          showIcon
          closable
          onClose={() => {
            localStorage.setItem(VELERO_TIP_KEY, '1')
            setVeleroTipVisible(false)
          }}
          style={{ marginBottom: 16 }}
          message="备份依赖目标集群中的 Velero"
          description={
            <span>
              未安装 Velero CRD 时，创建备份/恢复会被拒绝，不再返回虚假成功。
              安装 Velero 后将创建真实的 velero.io/v1 Backup/Restore 对象。
              可使用仓库模板一键安装（开发/单机）：
              <Text code>deploy/velero/install.sh install</Text>
              ，说明见 <Text code>deploy/velero/README.md</Text>。
            </span>
          }
        />
      )}

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
                  <Space>
                    {selectedBackupKeys.length > 0 && (
                      <>
                        <Text type="secondary">已选 {selectedBackupKeys.length} 项</Text>
                        <Button danger icon={<DeleteOutlined />} onClick={handleBatchDeleteBackups}>
                          批量删除
                        </Button>
                      </>
                    )}
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.resetFields(); setBackupModalVisible(true) }}>
                      创建备份
                    </Button>
                  </Space>
                }
              >
                <Table
                  columns={backupColumns}
                  dataSource={backups}
                  rowKey="id"
                  loading={loading}
                  rowSelection={{
                    selectedRowKeys: selectedBackupKeys,
                    onChange: setSelectedBackupKeys,
                    getCheckboxProps: (record) => ({
                      disabled: !canDeleteBackup(record),
                    }),
                  }}
                />
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
          <Form.Item name="storage_location" label="存储位置（可选）" extra="对应 Velero BackupStorageLocation 名称，默认 default">
            <Input placeholder="例如: default" />
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
