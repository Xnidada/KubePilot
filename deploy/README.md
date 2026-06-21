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

### 修改密码

部署后请立即修改默认密码：

1. 访问 http://localhost:8080
2. 使用默认账号 admin / admin123 登录
3. 进入系统管理 → 用户管理 → 修改密码

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

1. **修改默认密码**: 立即修改 admin 默认密码
2. **修改 JWT 密钥**: 设置强随机密钥
3. **启用 HTTPS**: 配置 Ingress TLS
4. **数据备份**: 定期备份 PostgreSQL 数据
5. **监控告警**: 配置 Prometheus + Grafana
