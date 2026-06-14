<div align="center">

# 🚀 KubePilot

**K8S 智能运维管理平台**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react)](https://react.dev/)
[![Ant Design](https://img.shields.io/badge/Ant%20Design-5.x-0170FE?style=flat&logo=antdesign)](https://ant.design/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28+-326CE5?style=flat&logo=kubernetes)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

</div>

---

## 📖 简介

KubePilot 是一个基于 Kubernetes 的生产级运维管理平台，采用 **Go** 后端 + **React** 前端架构，提供多集群管理、监控告警、RBAC 权限控制、审计日志和应用商店等功能，帮助企业简化 K8S 运维操作。

## ✨ 核心功能

### 🎯 集群管理
- 多集群统一管理
- 集群健康检查
- 节点资源概览
- Namespace 管理（支持状态显示和自动刷新）

### 📦 工作负载
- **Deployment** - 完整 CRUD、伸缩、回滚、企业级创建表单
- **Pod** - 查看、日志、Web Terminal、删除
- **Service** - 创建、编辑、删除，支持 NodePort 配置
- **ConfigMap** - 创建、编辑、删除，支持键值对管理
- **Secret** - 创建、编辑、删除，支持 Base64 编解码
- **Ingress** - 创建、删除，支持域名和路径规则配置
- **存储管理** - PV/PVC/StorageClass 管理

### 📊 监控告警
- 集群资源概览仪表盘
- CPU/内存使用率图表 (ECharts)
- Pod 状态分布
- Deployment 资源使用统计
- 节点资源详情
- 告警规则管理
- 通知渠道配置

### 🔐 安全管理
- JWT 用户认证
- RBAC 权限控制（16种资源 × 6种操作）
- 操作审计日志
- 角色权限可视化管理
- 用户管理（创建/编辑/删除/重置密码）

### 📡 资源状态
- **Terminating 状态显示** - 删除资源时显示终止状态
- **自动刷新** - 检测到 Terminating 状态时每 3 秒自动刷新
- **状态标签** - 统一的状态显示组件（Active/Running/Terminating/Pending/Failed）

## 🛠️ 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go, Gin, client-go, GORM, PostgreSQL, Redis |
| **前端** | React, TypeScript, Ant Design, ECharts, Zustand |
| **部署** | Docker, Helm, systemd |

## 📁 项目结构

```
kubepilot/
├── cmd/server/              # 入口文件
├── internal/
│   ├── config/             # 配置管理
│   ├── handler/            # HTTP 处理器
│   │   ├── auth/          # 认证
│   │   ├── cluster/       # 集群管理
│   │   ├── workload/      # 工作负载
│   │   ├── system/        # 系统管理
│   │   └── alert/         # 告警管理
│   ├── k8s/                # K8S 客户端封装
│   ├── middleware/          # 中间件
│   │   ├── auth.go        # JWT 认证
│   │   ├── rbac.go        # RBAC 权限
│   │   ├── audit.go       # 审计日志
│   │   └── cors.go        # CORS 跨域
│   ├── model/              # 数据模型
│   │   ├── permission.go  # 权限模型
│   │   └── ...
│   ├── pkg/                # 工具包
│   ├── repository/         # 数据访问层
│   ├── router/             # 路由定义
│   └── service/            # 业务逻辑层
├── frontend/               # React 前端
│   ├── src/
│   │   ├── api/           # API 封装
│   │   ├── components/    # 组件
│   │   │   ├── StatusTag.tsx      # 状态标签组件
│   │   │   ├── PodTerminal.tsx    # Pod 终端
│   │   │   └── ...
│   │   ├── hooks/         # 自定义 Hooks
│   │   │   └── usePolling.ts  # 自动轮询 Hook
│   │   ├── pages/         # 页面
│   │   └── stores/        # 状态管理
├── configs/                # 配置文件
├── deploy/                 # 部署配置
└── scripts/                # 脚本工具
```

## 🚀 快速开始

### 前置条件

- Go 1.22+
- Node.js 18+
- PostgreSQL 14+
- K8S 集群 (可选)

### 1. 克隆项目

```bash
git clone https://github.com/Xnidada/KubePilot.git
cd KubePilot
```

### 2. 配置后端

```bash
# 复制配置文件
cp configs/config.example.yaml configs/config.yaml

# 编辑配置文件，修改数据库连接等信息
vim configs/config.yaml
```

### 3. 启动后端

```bash
# 安装依赖
go mod tidy

# 初始化数据库和默认角色
go run scripts/init-admin.go

# 运行服务
go run cmd/server/main.go

# 或编译后运行
go build -o bin/kubepilot cmd/server/main.go
./bin/kubepilot
```

### 4. 启动前端

```bash
cd frontend

# 安装依赖
npm install

# 开发模式
npm run dev

# 构建生产版本
npm run build
```

### 5. 访问

- 前端: http://localhost:3000 (开发模式)
- API: http://localhost:8080/api/v1
- 默认账号: `admin / admin123`

## 🐳 Docker 部署

```bash
# 使用 Docker Compose
cd deploy/docker
docker-compose up -d
```

## ☸️ Helm 部署

```bash
# 安装到 K8S 集群
cd deploy/helm
helm install kubepilot ./kubepilot
```

## 📡 API 文档

### 认证
```
POST /api/v1/auth/login     - 用户登录
POST /api/v1/auth/register  - 用户注册
```

### 集群管理
```
GET    /api/v1/clusters           - 集群列表
POST   /api/v1/clusters           - 创建集群
GET    /api/v1/clusters/:id       - 集群详情
PUT    /api/v1/clusters/:id       - 更新集群
DELETE /api/v1/clusters/:id       - 删除集群
POST   /api/v1/clusters/:id/health - 健康检查
```

### 工作负载
```
GET    /api/v1/clusters/:id/workloads/deployments     - Deployment 列表
POST   /api/v1/clusters/:id/workloads/deployments     - 创建 Deployment
GET    /api/v1/clusters/:id/workloads/pods            - Pod 列表
GET    /api/v1/clusters/:id/workloads/services        - Service 列表
GET    /api/v1/clusters/:id/workloads/configmaps      - ConfigMap 列表
GET    /api/v1/clusters/:id/workloads/secrets         - Secret 列表
GET    /api/v1/clusters/:id/workloads/ingresses       - Ingress 列表
GET    /api/v1/clusters/:id/workloads/namespaces      - 命名空间列表
GET    /api/v1/clusters/:id/workloads/metrics/overview - 集群概览
```

### 系统管理
```
GET    /api/v1/system/users      - 用户列表
GET    /api/v1/system/roles      - 角色列表
GET    /api/v1/system/audit-logs - 审计日志
```

### 告警管理
```
GET    /api/v1/alerts/rules    - 告警规则
GET    /api/v1/alerts/history  - 告警历史
GET    /api/v1/alerts/channels - 通知渠道
```

## 🔐 权限说明

### 预定义角色

| 角色 | 说明 | 权限 |
|------|------|------|
| admin | 系统管理员 | 全部权限 |
| operator | 运维人员 | 管理工作负载、告警、应用商店 |
| user | 开发人员 | 查看、创建、编辑工作负载 |
| viewer | 只读用户 | 仅查看 |

### 资源类型

clusters, deployments, pods, services, configmaps, secrets, pvcs, pvs, namespaces, nodes, events, alerts, users, roles, audit_logs, appstore

### 操作类型

view, create, edit, delete, exec, admin

## 📊 状态说明

| 状态 | 颜色 | 说明 |
|------|------|------|
| Active | 🟢 绿色 | 正常运行 |
| Running | 🟢 绿色 | 运行中 |
| Terminating | 🟠 橙色 | 删除中（自动刷新） |
| Updating | 🔵 蓝色 | 更新中 |
| Pending | 🟠 橙色 | 等待中 |
| Failed | 🔴 红色 | 失败 |

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 📄 许可证

本项目采用 [Apache License 2.0](LICENSE) 许可证。

## 🙏 致谢

- [Kubernetes](https://kubernetes.io/)
- [Gin](https://github.com/gin-gonic/gin)
- [Ant Design](https://ant.design/)
- [React](https://react.dev/)

---

<div align="center">

**如果觉得不错，请给个 ⭐ Star 支持一下！**

</div>
