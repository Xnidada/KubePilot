import { useEffect, useMemo, useState, useCallback } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Avatar, Dropdown, Space, Button, theme, Tooltip, Badge } from 'antd'
import {
  DashboardOutlined,
  ClusterOutlined,
  CloudServerOutlined,
  LineChartOutlined,
  SettingOutlined,
  UserOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  DatabaseOutlined,
  RobotOutlined,
  ApiOutlined,
  ControlOutlined,
  GithubOutlined,
  ScheduleOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import type { MenuProps } from 'antd'
import { useAuthStore } from '../stores/auth'
import { listModules } from '../api/modules'
import { useInterval } from '../hooks/useInterval'

const { Header, Sider, Content } = Layout

/** Menu keys owned by feature modules — hidden when the module is disabled. */
const menuModuleMap: Record<string, string> = {
  '/aiops': 'aiops',
  '/aiops/agent': 'aiops',
  '/aiops/diagnosis': 'aiops',
  '/aiops/tools': 'aiops',
  '/aiops/settings': 'aiops',
  '/scheduler': 'scheduler',
  '/scheduler/tasks': 'scheduler',
  '/scheduler/queues': 'scheduler',
  '/cluster/inspection': 'inspection',
  '/cluster/event-forward': 'eventforward',
  '/system/backup': 'backup',
  '/system/webhooks': 'webhook',
  '/appstore': 'appstore',
}

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
    type: 'divider',
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
    ],
  },
  {
    key: '/network',
    icon: <ApiOutlined />,
    label: '网络',
    children: [
      {
        key: '/workloads/services',
        label: 'Service',
      },
      {
        key: '/workloads/ingresses',
        label: 'Ingress',
      },
      {
        key: '/workloads/networkpolicies',
        label: 'NetworkPolicy',
      },
    ],
  },
  {
    key: '/storage',
    icon: <DatabaseOutlined />,
    label: '存储',
    children: [
      {
        key: '/storage/storageclasses',
        label: 'StorageClass',
      },
      {
        key: '/storage/pvs',
        label: 'PersistentVolume',
      },
      {
        key: '/storage/pvcs',
        label: 'PVC',
      },
    ],
  },
  {
    key: '/config',
    icon: <ControlOutlined />,
    label: '配置',
    children: [
      {
        key: '/workloads/configmaps',
        label: 'ConfigMap',
      },
      {
        key: '/workloads/secrets',
        label: 'Secret',
      },
      {
        key: '/workloads/namespaces',
        label: '命名空间',
      },
      {
        key: '/workloads/crds',
        label: 'CRD',
      },
    ],
  },
  {
    type: 'divider',
  },
  {
    key: '/monitor',
    icon: <LineChartOutlined />,
    label: '监控',
    children: [
      {
        key: '/monitor/dashboard',
        label: '资源监控',
      },
      {
        key: '/monitor/node-pressure',
        label: '节点压力',
      },
      {
        key: '/monitor/cost',
        label: '资源成本',
      },
      {
        key: '/monitor/alerts',
        label: '告警中心',
      },
      {
        key: '/monitor',
        label: '事件告警',
      },
      {
        key: '/ops/events',
        label: '事件时间线',
      },
    ],
  },
  {
    key: '/ops',
    icon: <ToolOutlined />,
    label: '运维',
    children: [
      {
        key: '/cluster/inspection',
        label: '集群巡检',
      },
      {
        key: '/cluster/event-forward',
        label: 'Event 转发',
      },
      {
        key: '/ops/resource-graph',
        label: '资源依赖图',
      },
      {
        key: '/ops/idle-resources',
        label: '闲置资源清理',
      },
      {
        key: '/workloads/compare',
        label: '资源对比',
      },
      {
        key: '/workloads/batch',
        label: '批量操作',
      },
      {
        key: '/workloads/yaml-diff',
        label: 'YAML 对比',
      },
      {
        key: '/ops/pod-diagnosis',
        label: 'Pod 诊断',
      },
      {
        key: '/workloads/env-clone',
        label: '环境克隆',
      },
      {
        key: '/workloads/gpu',
        label: 'GPU 调度',
      },
    ],
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
    type: 'divider',
  },
  {
    key: '/aiops',
    icon: <RobotOutlined />,
    label: 'AI 智能',
    children: [
      {
        key: '/aiops/agent',
        label: 'AI Agent',
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
    label: '系统',
    children: [
      {
        key: '/system/users',
        label: '用户管理',
      },
      {
        key: '/system/user-groups',
        label: '用户组管理',
      },
      {
        key: '/system/roles',
        label: '角色管理',
      },
      {
        key: '/system/2fa',
        label: '两步验证',
      },
      {
        key: '/system/backup',
        label: '备份管理',
      },
      {
        key: '/system/webhooks',
        label: 'Webhook 通知',
      },
      {
        key: '/system/sso',
        label: 'SSO 配置',
      },
      {
        key: '/system/modules',
        label: '功能模块',
      },
    ],
  },
]

const menuPermissionMap: Record<string, { resource: string; action: string }> = {
  '/system/users': { resource: 'users', action: 'view' },
  '/system/user-groups': { resource: 'user_groups', action: 'view' },
  '/system/roles': { resource: 'roles', action: 'view' },
  '/system/backup': { resource: 'backups', action: 'view' },
  '/system/webhooks': { resource: 'webhooks', action: 'view' },
  '/system/sso': { resource: 'users', action: 'admin' },
  '/aiops/agent': { resource: 'aiops', action: 'execute' },
  '/aiops/diagnosis': { resource: 'aiops', action: 'execute' },
  '/aiops/tools': { resource: 'aiops', action: 'execute' },
  '/aiops/settings': { resource: 'aiops_config', action: 'view' },
  '/scheduler': { resource: 'scheduler', action: 'view' },
  '/cluster/inspection': { resource: 'inspection', action: 'view' },
  '/cluster/event-forward': { resource: 'event_forward', action: 'view' },
}

const MainLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const [enabledModules, setEnabledModules] = useState<Set<string> | null>(null)
  const [unhealthyCount, setUnhealthyCount] = useState(0)
  const [warnedCount, setWarnedCount] = useState(0)
  const navigate = useNavigate()
  const location = useLocation()
  const { user, logout, hasPermission } = useAuthStore()
  const { token: { colorBgContainer, borderRadiusLG } } = theme.useToken()

  const refreshModuleMeta = useCallback(async () => {
    try {
      const res = await listModules()
      const list = res.data || []
      setEnabledModules(new Set(list.filter((m) => m.enabled).map((m) => m.name)))
      const enabled = list.filter((m) => m.enabled)
      setUnhealthyCount(enabled.filter((m) => !m.healthy).length)
      setWarnedCount(enabled.filter((m) => m.healthy && typeof m.details?.health_warning === 'string').length)
    } catch {
      setEnabledModules(null) // fail open: show all module menus
    }
  }, [])

  useEffect(() => {
    refreshModuleMeta()
  }, [user?.id, refreshModuleMeta])

  useInterval(refreshModuleMeta, 20000, !!user)

  const visibleMenuItems = useMemo(() => {
    const healthBadge = (unhealthy: number, warned: number) => (
      unhealthy > 0
        ? <Badge count={unhealthy} size="small" overflowCount={9} />
        : warned > 0
          ? <Badge status="warning" />
          : null
    )
    const moduleEnabled = (key: string) => {
      if (!enabledModules) return true
      const mod = menuModuleMap[key]
      if (!mod) return true
      return enabledModules.has(mod)
    }
    const filterItems = (items: MenuProps['items']): MenuProps['items'] => {
      if (!items) return items
      return items
        .map(item => {
          if (!item || item.type === 'divider') return item
          const key = 'key' in item ? String(item.key || '') : ''
          if (!moduleEnabled(key)) return null
          const children = 'children' in item ? item.children : undefined
          let next: typeof item = item
          if (children) {
            const filteredChildren = filterItems(children)
            if (!filteredChildren || filteredChildren.length === 0) return null
            next = { ...item, children: filteredChildren }
          }
          const required = menuPermissionMap[key]
          if (required && !hasPermission(required.resource, required.action)) return null

          const badge = healthBadge(unhealthyCount, warnedCount)
          if (badge && key === '/system/modules') {
            next = { ...next, label: <span>功能模块 {badge}</span> }
          } else if (badge && key === '/system') {
            next = { ...next, label: <span>系统 {badge}</span> }
          }
          return next
        })
        .filter(Boolean) as MenuProps['items']
    }
    return filterItems(menuItems)
  }, [hasPermission, user, enabledModules, unhealthyCount, warnedCount])

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
      onClick: () => navigate('/profile'),
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
      '/workloads/gpu': ['/workloads'],
      '/workloads/env-clone': ['/workloads'],
      '/workloads/networkpolicies': ['/network'],
      '/monitor/cost': ['/monitor'],
      '/monitor/alerts': ['/monitor'],
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
          items={visibleMenuItems}
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
