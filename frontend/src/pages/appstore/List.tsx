import { useState } from 'react'
import { Card, Row, Col, Typography, Input, Tag, Button, Space, Rate, Avatar } from 'antd'
import {
  SearchOutlined,
  DownloadOutlined,
  CloudOutlined,
  DatabaseOutlined,
  ToolOutlined,
  BarChartOutlined,
  SafetyOutlined,
  AppstoreOutlined,
} from '@ant-design/icons'

const { Title, Text, Paragraph } = Typography

interface AppTemplate {
  id: number
  name: string
  description: string
  icon: React.ReactNode
  category: string
  version: string
  downloads: number
  rating: number
  tags: string[]
}

const mockApps: AppTemplate[] = [
  {
    id: 1,
    name: 'Nginx',
    description: '高性能 HTTP 和反向代理服务器',
    icon: <CloudOutlined style={{ fontSize: 32, color: '#1890ff' }} />,
    category: 'Web服务器',
    version: '1.21.0',
    downloads: 1234,
    rating: 4.5,
    tags: ['web', 'proxy'],
  },
  {
    id: 2,
    name: 'MySQL',
    description: '世界上最流行的开源关系型数据库',
    icon: <DatabaseOutlined style={{ fontSize: 32, color: '#52c41a' }} />,
    category: '数据库',
    version: '8.0.30',
    downloads: 2345,
    rating: 4.8,
    tags: ['database', 'sql'],
  },
  {
    id: 3,
    name: 'Redis',
    description: '内存数据结构存储，用作数据库、缓存和消息代理',
    icon: <DatabaseOutlined style={{ fontSize: 32, color: '#ff4d4f' }} />,
    category: '数据库',
    version: '7.0.5',
    downloads: 3456,
    rating: 4.7,
    tags: ['cache', 'nosql'],
  },
  {
    id: 4,
    name: 'Prometheus',
    description: '开源系统监控和警报工具包',
    icon: <BarChartOutlined style={{ fontSize: 32, color: '#722ed1' }} />,
    category: '监控',
    version: '2.45.0',
    downloads: 5678,
    rating: 4.9,
    tags: ['monitoring', 'alerting'],
  },
  {
    id: 5,
    name: 'Grafana',
    description: '开源数据可视化和监控平台',
    icon: <BarChartOutlined style={{ fontSize: 32, color: '#fa8c16' }} />,
    category: '监控',
    version: '10.0.0',
    downloads: 4567,
    rating: 4.6,
    tags: ['visualization', 'dashboard'],
  },
  {
    id: 6,
    name: 'GitLab',
    description: '完整的 DevOps 平台',
    icon: <ToolOutlined style={{ fontSize: 32, color: '#eb2f96' }} />,
    category: 'DevOps',
    version: '16.0.0',
    downloads: 1890,
    rating: 4.4,
    tags: ['ci-cd', 'git'],
  },
  {
    id: 7,
    name: 'Harbor',
    description: '云原生容器镜像仓库',
    icon: <SafetyOutlined style={{ fontSize: 32, color: '#13c2c2' }} />,
    category: '安全',
    version: '2.9.0',
    downloads: 2100,
    rating: 4.5,
    tags: ['registry', 'security'],
  },
  {
    id: 8,
    name: 'Kubernetes Dashboard',
    description: 'K8S 通用 Web UI',
    icon: <AppstoreOutlined style={{ fontSize: 32, color: '#2f54eb' }} />,
    category: '管理',
    version: '2.7.0',
    downloads: 6789,
    rating: 4.3,
    tags: ['dashboard', 'ui'],
  },
]

const categories = ['全部', 'Web服务器', '数据库', '监控', 'DevOps', '安全', '管理']

const AppStoreList: React.FC = () => {
  const [searchText, setSearchText] = useState('')
  const [selectedCategory, setSelectedCategory] = useState('全部')

  const filteredApps = mockApps.filter((app) => {
    const matchesSearch =
      app.name.toLowerCase().includes(searchText.toLowerCase()) ||
      app.description.toLowerCase().includes(searchText.toLowerCase())
    const matchesCategory = selectedCategory === '全部' || app.category === selectedCategory
    return matchesSearch && matchesCategory
  })

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4}>应用商店</Title>
        <Space>
          <Input
            placeholder="搜索应用..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 300 }}
          />
        </Space>
      </div>

      <Card style={{ marginBottom: 16 }}>
        <Space size="middle" wrap>
          {categories.map((category) => (
            <Button
              key={category}
              type={selectedCategory === category ? 'primary' : 'default'}
              onClick={() => setSelectedCategory(category)}
            >
              {category}
            </Button>
          ))}
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        {filteredApps.map((app) => (
          <Col xs={24} sm={12} lg={8} xl={6} key={app.id}>
            <Card
              hoverable
              style={{ height: '100%' }}
              actions={[
                <Button type="link" icon={<DownloadOutlined />} key="install">
                  安装
                </Button>,
                <Button type="link" key="detail">
                  详情
                </Button>,
              ]}
            >
              <Card.Meta
                avatar={
                  <Avatar
                    shape="square"
                    size={48}
                    icon={app.icon}
                    style={{ background: '#f5f5f5' }}
                  />
                }
                title={
                  <Space>
                    <Text strong>{app.name}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      v{app.version}
                    </Text>
                  </Space>
                }
                description={
                  <div>
                    <Paragraph
                      ellipsis={{ rows: 2 }}
                      style={{ marginBottom: 8, minHeight: 44 }}
                    >
                      {app.description}
                    </Paragraph>
                    <Space size={[0, 4]} wrap>
                      {app.tags.map((tag) => (
                        <Tag key={tag} style={{ margin: 0 }}>
                          {tag}
                        </Tag>
                      ))}
                    </Space>
                    <div style={{ marginTop: 8 }}>
                      <Space>
                        <Rate disabled defaultValue={app.rating} allowHalf style={{ fontSize: 14 }} />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {app.downloads} 次下载
                        </Text>
                      </Space>
                    </div>
                  </div>
                }
              />
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  )
}

export default AppStoreList
