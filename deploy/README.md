# KubePilot 部署指南

## 快速开始

### Docker Compose 部署（推荐）

```bash
# 一键部署
./deploy.sh docker-compose

# 或手动部署
docker-compose up -d --build
```

部署完成后访问: http://localhost:8080

### Kubernetes 部署

```bash
# 一键部署
./deploy.sh k8s

# 或手动部署
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/postgres.yaml
kubectl apply -f deploy/k8s/redis.yaml
kubectl apply -f deploy/k8s/kubepilot.yaml

# 访问服务
kubectl port-forward -n kubepilot svc/kubepilot 8080:8080
```

## 服务说明

| 服务 | 端口 | 说明 |
|------|------|------|
| KubePilot | 8080 | Web 管理界面 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |

## 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| KUBEPILOT_SERVER_HOST | 0.0.0.0 | 监听地址 |
| KUBEPILOT_SERVER_PORT | 8080 | 监听端口 |
| KUBEPILOT_DATABASE_HOST | postgres | 数据库地址 |
| KUBEPILOT_DATABASE_PORT | 5432 | 数据库端口 |
| KUBEPILOT_DATABASE_USERNAME | kubepilot | 数据库用户 |
| KUBEPILOT_DATABASE_PASSWORD | kubepilot123 | 数据库密码 |
| KUBEPILOT_CACHE_TYPE | redis | 缓存类型 |
| KUBEPILOT_CACHE_ADDR | redis:6379 | Redis 地址 |
| KUBEPILOT_JWT_SECRET | - | JWT 密钥（必须修改） |
| KUBEPILOT_BOOTSTRAP_ADMIN_PASSWORD | - | 首次运行初始化脚本时设置管理员密码（至少 12 字符） |
| KUBEPILOT_SEED_DEMO_USERS | false | 仅测试环境显式创建演示用户 |
| KUBEPILOT_DEMO_PASSWORD | - | 开启演示用户时必填；不会自动授权集群 |

### 初始化管理员

连接数据库后执行 `KUBEPILOT_BOOTSTRAP_ADMIN_PASSWORD='<强随机密码>' go run scripts/init-admin.go`。脚本只创建缺失账号，不重置已有密码、角色或集群授权。演示用户必须显式启用并由管理员手工分配集群权限。

## 常用命令

```bash
# Docker Compose
docker-compose logs -f kubepilot  # 查看日志
docker-compose restart kubepilot  # 重启服务
docker-compose down               # 停止服务
docker-compose down -v            # 偯止并删除数据

# Kubernetes
kubectl get pods -n kubepilot           # 查看 Pod 状态
kubectl logs -f deployment/kubepilot -n kubepilot  # 查看日志
kubectl delete namespace kubepilot      # 删除所有资源
```

## 生产环境建议

- 设置强管理员密码与 JWT Secret
- 使用独立 PostgreSQL / Redis，并做好备份
- 配置 Ingress TLS

## 运行时依赖：kubectl

镜像构建（`Dockerfile`）会安装 kubectl，供 AI describe、YAML apply/delete 等能力使用。

若使用 hostPath 挂载二进制（Alpine 基础镜像），请同时把 kubectl 放到同一目录并挂载进容器，例如：

```bash
# 宿主机
curl -fsSL -o /opt/kubepilot/bin/kubectl \
  https://dl.k8s.io/release/v1.29.14/bin/linux/amd64/kubectl
chmod +x /opt/kubepilot/bin/kubectl

# Deployment volumeMount 示例
# mountPath: /usr/local/bin/kubectl
# subPath: kubectl
# name: app-binary   # hostPath=/opt/kubepilot/bin
```

## 备份依赖 Velero

KubePilot 备份模块会在目标集群创建真实的 `velero.io/v1` Backup/Restore。  
一键安装（开发/单机，含 MinIO）见：

```bash
./deploy/velero/install.sh install
```

说明文档：[`deploy/velero/README.md`](velero/README.md)

1. **管理员密码**: 首次初始化使用独立强密码
2. **修改 JWT 密钥**: 设置强随机密钥
3. **启用 HTTPS**: 配置 Ingress TLS
4. **数据备份**: 定期备份 PostgreSQL 数据
5. **监控告警**: 配置 Prometheus + Grafana
