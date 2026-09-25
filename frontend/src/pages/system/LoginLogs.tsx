import { useEffect, useState } from 'react'
import { Button, Card, Table, Input, Popconfirm, Space, Tag, Typography, message } from 'antd'
import { DeleteOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { batchDeleteLoginLogs, deleteLoginLog, getLoginLogs, LoginLog } from '../../api/system'
import { useAuthStore } from '../../stores/auth'

const { Title } = Typography
const { Search } = Input

const LoginLogs: React.FC = () => {
  const [logs, setLogs] = useState<LoginLog[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [usernameFilter, setUsernameFilter] = useState('')
  const [ipFilter, setIpFilter] = useState('')
  const [appliedFilters, setAppliedFilters] = useState({ username: '', ip: '' })
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([])
  const [deleting, setDeleting] = useState(false)
  const canDelete = useAuthStore((state) => state.hasPermission('login_logs', 'delete'))

  useEffect(() => {
    void fetchLogs(page, pageSize, appliedFilters)
  }, [page, pageSize, appliedFilters])

  const fetchLogs = async (currentPage: number, currentSize: number, filters: { username: string; ip: string }) => {
    setLoading(true)
    try {
      const res = await getLoginLogs({
        page: currentPage,
        size: currentSize,
        username: filters.username || undefined,
        ip: filters.ip || undefined,
      })
      if (currentPage > 1 && res.total <= (currentPage - 1) * currentSize) {
        setPage(Math.max(1, Math.ceil(res.total / currentSize)))
        return
      }
      setLogs(res.data || [])
      setTotal(res.total || 0)
    } catch (error) {
      console.error('Failed to fetch login logs:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleSearch = () => {
    setSelectedKeys([])
    setPage(1)
    setAppliedFilters({ username: usernameFilter.trim(), ip: ipFilter.trim() })
  }

  const handleDelete = async (ids: number[]) => {
    if (ids.length === 0) return
    setDeleting(true)
    try {
      const res = ids.length === 1 ? await deleteLoginLog(ids[0]) : await batchDeleteLoginLogs(ids)
      message.success(`已删除 ${res.data.deleted} 条登入日志`)
      setSelectedKeys([])
      await fetchLogs(page, pageSize, appliedFilters)
    } catch (error) {
      console.error('Failed to delete login logs:', error)
    } finally {
      setDeleting(false)
    }
  }

  const columns: ColumnsType<LoginLog> = [
    {
      title: '用户名',
      dataIndex: 'username',
      key: 'username',
      width: 150,
    },
    {
      title: 'IP 地址',
      dataIndex: 'ip',
      key: 'ip',
      width: 160,
    },
    {
      title: 'User-Agent',
      dataIndex: 'user_agent',
      key: 'user_agent',
      ellipsis: true,
      render: (text: string) => (
        <span style={{ fontSize: 12, color: '#888' }}>{text}</span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'success',
      key: 'success',
      width: 80,
      render: (success: boolean) =>
        success ? (
          <Tag color="success">成功</Tag>
        ) : (
          <Tag color="error">失败</Tag>
        ),
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (text: string) => {
        if (!text) return '-'
        try {
          return new Date(text).toLocaleString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
          })
        } catch {
          return text
        }
      },
    },
    ...(canDelete ? [{
      title: '操作',
      key: 'actions',
      width: 90,
      render: (_: unknown, row: LoginLog) => (
        <Popconfirm
          title="确认删除这条登入日志？"
          description="删除后无法恢复，操作会写入审计日志。"
          okText="删除"
          okButtonProps={{ danger: true, loading: deleting }}
          cancelText="取消"
          onConfirm={() => handleDelete([row.id])}
        >
          <Button type="link" danger icon={<DeleteOutlined />} disabled={deleting}>删除</Button>
        </Popconfirm>
      ),
    }] : []),
  ]

  return (
    <div style={{ padding: 24 }}>
      <Title level={4}>登入日志</Title>
      <Card>
        <Space style={{ marginBottom: 16 }} wrap>
          <Search
            placeholder="搜索用户名"
            allowClear
            value={usernameFilter}
            onChange={(e) => setUsernameFilter(e.target.value)}
            onSearch={handleSearch}
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
          />
          <Search
            placeholder="搜索 IP 地址"
            allowClear
            value={ipFilter}
            onChange={(e) => setIpFilter(e.target.value)}
            onSearch={handleSearch}
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
          />
          {canDelete && (
            <Popconfirm
              title={`确认删除选中的 ${selectedKeys.length} 条登入日志？`}
              description="删除后无法恢复，操作会写入审计日志。"
              okText="删除"
              okButtonProps={{ danger: true, loading: deleting }}
              cancelText="取消"
              onConfirm={() => handleDelete(selectedKeys.map(Number))}
            >
              <Button danger icon={<DeleteOutlined />} disabled={selectedKeys.length === 0 || deleting}>
                删除所选{selectedKeys.length > 0 ? ` (${selectedKeys.length})` : ''}
              </Button>
            </Popconfirm>
          )}
        </Space>
        <Table
          columns={columns}
          dataSource={logs}
          rowKey="id"
          loading={loading}
          rowSelection={canDelete ? {
            selectedRowKeys: selectedKeys,
            onChange: setSelectedKeys,
          } : undefined}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => {
              setSelectedKeys([])
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>
    </div>
  )
}

export default LoginLogs
