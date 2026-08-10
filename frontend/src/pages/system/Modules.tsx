import { useState, useEffect, useCallback, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Card, Table, Typography, Tag, Button, Space, Switch, Alert, Tooltip, message, Popconfirm,
} from 'antd'
import {
  AppstoreOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined,
  LinkOutlined, WarningOutlined, ClearOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listModules, ModuleStatus } from '../../api/modules'
import { resetEventForwardStats } from '../../api/system'
import { useInterval } from '../../hooks/useInterval'
import { MODULE_LINKS } from '../../constants/modules'

const { Title, Text } = Typography

const REFRESH_MS = 10000

function formatDetails(details?: Record<string, unknown>): string {
  if (!details || Object.keys(details).length === 0) return '-'
  return Object.entries(details)
    .filter(([k]) => k !== 'health_warning' && k !== 'fail_since' && k !== 'note')
    .map(([k, v]) => {
      if (k === 'fail_rate' && typeof v === 'number') {
        return `${k}=${(v * 100).toFixed(0)}%`
      }
      return `${k}=${v}`
    })
    .join(' · ')
}

const Modules: React.FC = () => {
  const navigate = useNavigate()
  const [modules, setModules] = useState<ModuleStatus[]>([])
  const [loading, setLoading] = useState(false)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [resetting, setResetting] = useState(false)

  const fetchModules = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      const res = await listModules()
      setModules(res.data || [])
      setLastUpdated(new Date())
    } catch (e) {
      console.error(e)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => { fetchModules(false) }, [fetchModules])
  useInterval(() => fetchModules(true), REFRESH_MS, autoRefresh)

  const unhealthy = useMemo(
    () => modules.filter((m) => m.enabled && !m.healthy),
    [modules],
  )
  const warned = useMemo(
    () => modules.filter((m) => m.enabled && m.healthy && typeof m.details?.health_warning === 'string'),
    [modules],
  )

  const handleResetEFStats = async () => {
    setResetting(true)
    try {
      await resetEventForwardStats()
      message.success('Event 转发计数已重置')
      await fetchModules(true)
    } catch {
      message.error('重置失败')
    } finally {
      setResetting(false)
    }
  }

  const columns: ColumnsType<ModuleStatus> = [
    {
      title: '模块',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{name}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>{row.description}</Text>
        </Space>
      ),
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      width: 90,
      render: (v: string) => <Tag>{v}</Tag>,
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>{enabled ? '是' : '否'}</Tag>
      ),
    },
    {
      title: '健康',
      dataIndex: 'healthy',
      key: 'healthy',
      width: 260,
      render: (healthy: boolean, row) => {
        const warning = typeof row.details?.health_warning === 'string'
          ? String(row.details.health_warning)
          : ''
        if (!row.enabled) {
          return <Tag>未启用</Tag>
        }
        if (!healthy) {
          return (
            <Space direction="vertical" size={0}>
              <Tag icon={<CloseCircleOutlined />} color="error">异常</Tag>
              {row.health_error ? (
                <Text type="danger" style={{ fontSize: 12 }}>{row.health_error}</Text>
              ) : null}
            </Space>
          )
        }
        if (warning) {
          return (
            <Space direction="vertical" size={0}>
              <Tag icon={<WarningOutlined />} color="warning">警告</Tag>
              <Tooltip title={warning}>
                <Text type="warning" style={{ fontSize: 12 }} ellipsis>
                  {warning}
                </Text>
              </Tooltip>
            </Space>
          )
        }
        return <Tag icon={<CheckCircleOutlined />} color="success">正常</Tag>
      },
    },
    {
      title: '多实例',
      dataIndex: 'multi_instance',
      key: 'multi_instance',
      width: 110,
      render: (v?: string) => v || '-',
    },
    {
      title: '运行详情',
      dataIndex: 'details',
      key: 'details',
      render: (details?: Record<string, unknown>) => (
        <Tooltip title={details ? JSON.stringify(details, null, 2) : undefined}>
          <Text code style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
            {formatDetails(details)}
          </Text>
        </Tooltip>
      ),
    },
    {
      title: '入口 / 操作',
      key: 'links',
      width: 280,
      render: (_, row) => {
        const links = MODULE_LINKS[row.name] || []
        return (
          <Space wrap size={[4, 4]}>
            {links.map((link) => (
              <Button
                key={link.path}
                type="link"
                size="small"
                icon={<LinkOutlined />}
                disabled={!row.enabled}
                onClick={() => navigate(link.path)}
              >
                {link.label}
              </Button>
            ))}
            {row.name === 'eventforward' && row.enabled ? (
              <Popconfirm
                title="重置 Event 转发内存计数？"
                description="不会停止 Watcher，仅清零失败/成功计数以便健康恢复。"
                onConfirm={handleResetEFStats}
              >
                <Button type="link" size="small" icon={<ClearOutlined />} loading={resetting}>
                  重置计数
                </Button>
              </Popconfirm>
            ) : null}
            {links.length === 0 && row.name !== 'eventforward' ? (
              <Text type="secondary">-</Text>
            ) : null}
          </Space>
        )
      },
    },
  ]

  return (
    <div>
      <Title level={4}><AppstoreOutlined /> 功能模块</Title>
      {unhealthy.length > 0 ? (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message={`${unhealthy.length} 个模块健康检查异常`}
          description={unhealthy.map((m) => `${m.name}: ${m.health_error || 'unknown'}`).join('；')}
        />
      ) : null}
      {warned.length > 0 ? (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={`${warned.length} 个模块有健康警告`}
          description={warned.map((m) => `${m.name}: ${String(m.details?.health_warning)}`).join('；')}
        />
      ) : null}
      <Card
        extra={
          <Space>
            {lastUpdated ? (
              <Text type="secondary" style={{ fontSize: 12 }}>
                更新于 {lastUpdated.toLocaleTimeString()}
              </Text>
            ) : null}
            <Space size={6}>
              <Text type="secondary" style={{ fontSize: 12 }}>自动刷新</Text>
              <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
            </Space>
            <Button icon={<ReloadOutlined />} onClick={() => fetchModules(false)} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="name"
          loading={loading}
          columns={columns}
          dataSource={modules}
          pagination={false}
        />
      </Card>
    </div>
  )
}

export default Modules
