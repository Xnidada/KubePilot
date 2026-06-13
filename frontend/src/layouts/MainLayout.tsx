import { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Avatar, Dropdown, Space, Button, theme } from 'antd'
import {
  DashboardOutlined,
  ClusterOutlined,
  CloudServerOutlined,
  LineChartOutlined,
  AppstoreOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  DatabaseOutlined,
} from '@ant-design/icons'
import type { MenuProps } from 'antd'
import { useAuthStore } from '../stores/auth'

const { Header, Sider, Content } = Layout

const menuItems: MenuProps['items'] = [
  {
    key: '/dashboard',
    icon: <DashboardOutlined />,
    label: '仪表盘',
  },
  {
    key: '/clusters',
    icon: <ClusterOutlined />,
    label: '集群管理',
  },
  {
    key: '/workloads',
    icon: <CloudServerOutlined />,
    label: '工作负载',
    children: [
      {
        key: '/workloads/deployments',
        label: 'Deployment',
      },
      {
        key: '/workloads/pods',
        label: 'Pod',
      },
      {
        key: '/workloads/services',
        label: 'Service',
      },
    ],
  },
  {
    key: '/storage',
    icon: <DatabaseOutlined />,
    label: '存储管理',
    children: [
      {
        key: '/storage/pvs',
        label: 'PersistentVolume',
      },
      {
        key: '/storage/pvcs',
        label: 'PersistentVolumeClaim',
      },
    ],
  },
  {
    key: '/monitor',
    icon: <LineChartOutlined />,
    label: '监控告警',
    children: [
      {
        key: '/monitor/dashboard',
        label: '资源监控',
      },
      {
        key: '/monitor',
        label: '事件告警',
      },
    ],
  },
  {
    key: '/appstore',
    icon: <AppstoreOutlined />,
    label: '应用商店',
  },
  {
    key: '/system',
    icon: <SettingOutlined />,
    label: '系统管理',
    children: [
      {
        key: '/system/users',
        label: '用户管理',
      },
      {
        key: '/system/roles',
        label: '角色管理',
      },
    ],
  },
]

const MainLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { user, logout } = useAuthStore()
  const { token: { colorBgContainer, borderRadiusLG } } = theme.useToken()

  const handleMenuClick: MenuProps['onClick'] = ({ key }) => {
    navigate(key)
  }

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const userMenu: MenuProps['items'] = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人中心',
    },
    {
      type: 'divider',
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
    },
  ]

  const getSelectedKeys = () => {
    const path = location.pathname
    if (path.startsWith('/workloads/')) {
      return [path]
    }
    return [path]
  }

  const getOpenKeys = () => {
    const path = location.pathname
    const keys: string[] = []
    if (path.startsWith('/workloads')) {
      keys.push('/workloads')
    }
    if (path.startsWith('/system')) {
      keys.push('/system')
    }
    return keys
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        style={{
          overflow: 'auto',
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
          background: '#001529',
        }}
      >
        <div
          style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'white',
            fontSize: collapsed ? 16 : 20,
            fontWeight: 'bold',
            borderBottom: '1px solid rgba(255,255,255,0.1)',
          }}
        >
          {collapsed ? 'KP' : '🚀 KubePilot'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={getSelectedKeys()}
          defaultOpenKeys={getOpenKeys()}
          items={menuItems}
          onClick={handleMenuClick}
        />
      </Sider>
      <Layout style={{ marginLeft: collapsed ? 80 : 200, transition: 'all 0.2s' }}>
        <Header
          style={{
            padding: '0 24px',
            background: colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            boxShadow: '0 1px 4px rgba(0,0,0,0.08)',
            position: 'sticky',
            top: 0,
            zIndex: 10,
          }}
        >
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
            style={{ fontSize: 16, width: 48, height: 48 }}
          />
          <Dropdown menu={{ items: userMenu }} placement="bottomRight">
            <Space style={{ cursor: 'pointer' }}>
              <Avatar icon={<UserOutlined />} />
              <span>{user?.real_name || user?.username || 'Admin'}</span>
            </Space>
          </Dropdown>
        </Header>
        <Content
          style={{
            margin: 24,
            padding: 24,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
            minHeight: 280,
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

export default MainLayout
