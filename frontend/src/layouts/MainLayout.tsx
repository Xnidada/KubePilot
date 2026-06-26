import { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Avatar, Dropdown, Space, Button, theme, Tooltip } from 'antd'
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
  RobotOutlined,
  ApiOutlined,
  ControlOutlined,
  ApartmentOutlined,
  GithubOutlined,
  ScheduleOutlined,
  ToolOutlined,
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
        key: '/workloads/statefulsets',
        label: 'StatefulSet',
      },
      {
        key: '/workloads/daemonsets',
        label: 'DaemonSet',
      },
      {
        key: '/workloads/replicasets',
        label: 'ReplicaSet',
      },
      {
        key: '/workloads/pods',
        label: 'Pod',
      },
      {
        key: '/workloads/jobs',
        label: 'Job',
      },
      {
        key: '/workloads/cronjobs',
        label: 'CronJob',
      },
      {
        key: '/workloads/hpas',
        label: 'HPA 自动伸缩',
      },
      {
        key: '/workloads/batch',
        label: '批量操作',
      },
      {
        key: '/workloads/compare',
        label: '资源对比',
      },
    ],
  },
  {
    key: '/network',
    icon: <ApiOutlined />,
    label: '服务与网络',
    children: [
      {
        key: '/workloads/services',
        label: 'Service',
      },
      {
        key: '/workloads/ingresses',
        label: 'Ingress',
      },
    ],
  },
  {
    key: '/config',
    icon: <ControlOutlined />,
    label: '配置管理',
    children: [
      {
        key: '/workloads/configmaps',
        label: 'ConfigMap',
      },
      {
        key: '/workloads/secrets',
        label: 'Secret',
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
      {
        key: '/storage/storageclasses',
        label: 'StorageClass',
      },
    ],
  },
  {
    key: '/cluster-resources',
    icon: <ApartmentOutlined />,
    label: '集群资源',
    children: [
      {
        key: '/workloads/namespaces',
        label: '命名空间',
      },
      {
        key: '/workloads/crds',
        label: 'CRD 管理',
      },
      {
        key: '/cluster/inspection',
        label: '集群巡检',
      },
      {
        key: '/cluster/event-forward',
        label: 'Event 转发',
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
    key: '/ops',
    icon: <ToolOutlined />,
    label: '运维工具',
    children: [
      {
        key: '/ops/diagnosis',
        label: 'Pod 诊断',
      },
      {
        key: '/ops/node-pressure',
        label: '节点压力',
      },
      {
        key: '/ops/events',
        label: '事件时间线',
      },
      {
        key: '/ops/resource-graph',
        label: '资源依赖图',
      },
      {
        key: '/ops/idle-resources',
        label: '闲置资源清理',
      },
    ],
  },
  {
    key: '/appstore',
    icon: <AppstoreOutlined />,
    label: '应用商店',
  },
  {
    key: '/scheduler',
    icon: <ScheduleOutlined />,
    label: '任务调度',
    children: [
      {
        key: '/scheduler/tasks',
        label: '任务管理',
      },
      {
        key: '/scheduler/queues',
        label: '队列管理',
      },
    ],
  },
  {
    key: '/aiops',
    icon: <RobotOutlined />,
    label: 'AI 智能',
    children: [
      {
        key: '/aiops/agent',
        label: 'AI 助手',
      },
      {
        key: '/aiops/diagnosis',
        label: '智能诊断',
      },
      {
        key: '/aiops/tools',
        label: 'AI 工具箱',
      },
      {
        key: '/aiops/settings',
        label: 'AI 设置',
      },
    ],
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
      {
        key: '/system/2fa',
        label: '两步验证',
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

    // 根据路径判断应该展开哪个菜单组
    const menuGroupMap: { [key: string]: string[] } = {
      '/workloads/deployments': ['/workloads'],
      '/workloads/statefulsets': ['/workloads'],
      '/workloads/daemonsets': ['/workloads'],
      '/workloads/replicasets': ['/workloads'],
      '/workloads/pods': ['/workloads'],
      '/workloads/jobs': ['/workloads'],
      '/workloads/cronjobs': ['/workloads'],
      '/workloads/hpas': ['/workloads'],
      '/workloads/batch': ['/workloads'],
      '/workloads/compare': ['/workloads'],
      '/workloads/services': ['/network'],
      '/workloads/ingresses': ['/network'],
      '/workloads/configmaps': ['/config'],
      '/workloads/secrets': ['/config'],
      '/workloads/namespaces': ['/cluster-resources'],
      '/workloads/crds': ['/cluster-resources'],
      '/cluster/inspection': ['/cluster-resources'],
      '/cluster/event-forward': ['/cluster-resources'],
      '/storage': ['/storage'],
      '/monitor': ['/monitor'],
      '/aiops': ['/aiops'],
      '/scheduler': ['/scheduler'],
      '/ops': ['/ops'],
      '/system': ['/system'],
    }

    for (const [prefix, groups] of Object.entries(menuGroupMap)) {
      if (path.startsWith(prefix)) {
        keys.push(...groups)
        break
      }
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
          <Space size="middle">
            <Tooltip title="GitHub">
              <Button
                type="text"
                icon={<GithubOutlined />}
                href="https://github.com/Xnidada/KubePilot"
                target="_blank"
                style={{ fontSize: 18 }}
              />
            </Tooltip>
            <Dropdown menu={{ items: userMenu }} placement="bottomRight">
              <Space style={{ cursor: 'pointer' }}>
                <Avatar icon={<UserOutlined />} />
                <span>{user?.real_name || user?.username || 'Admin'}</span>
              </Space>
            </Dropdown>
          </Space>
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
