import { useEffect, useState } from 'react'
import { Card, Table, Input, Space, Tag, Typography } from 'antd'
import { SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getLoginLogs, LoginLog } from '../../api/system'

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

  useEffect(() => {
    fetchLogs()
  }, [page, pageSize])

  const fetchLogs = async () => {
    setLoading(true)
    try {
      const res = await getLoginLogs({
        page,
        size: pageSize,
        username: usernameFilter || undefined,
        ip: ipFilter || undefined,
      })
      setLogs(res.data || [])
      setTotal(res.total || 0)
    } catch (error) {
      console.error('Failed to fetch login logs:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleSearch = () => {
    setPage(1)
    fetchLogs()
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
        </Space>
        <Table
          columns={columns}
          dataSource={logs}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => {
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
