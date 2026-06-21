#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助
show_help() {
    echo "KubePilot 部署脚本"
    echo ""
    echo "用法: ./deploy.sh [命令]"
    echo ""
    echo "命令:"
    echo "  docker-compose    使用 Docker Compose 部署"
    echo "  k8s               使用 Kubernetes 部署"
    echo "  build             构建 Docker 镜像"
    echo "  stop              停止 Docker Compose 服务"
    echo "  clean             清理所有资源"
    echo "  help              显示此帮助信息"
}

# 构建 Docker 镜像
build_image() {
    print_info "构建 Docker 镜像..."
    docker build -t kubepilot:latest .
    print_info "镜像构建完成"
}

# Docker Compose 部署
deploy_docker_compose() {
    print_info "使用 Docker Compose 部署..."

    # 检查 docker-compose 是否安装
    if ! command -v docker-compose &> /dev/null; then
        print_error "docker-compose 未安装"
        exit 1
    fi

    # 构建并启动服务
    docker-compose up -d --build

    print_info "等待服务启动..."
    sleep 10

    # 检查服务状态
    docker-compose ps

    print_info "部署完成！"
    print_info "访问地址: http://localhost:8080"
    print_info "默认管理员: admin / changeme"
}

# Kubernetes 部署
deploy_k8s() {
    print_info "使用 Kubernetes 部署..."

    # 检查 kubectl 是否安装
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl 未安装"
        exit 1
    fi

    # 构建镜像（如果需要）
    if [ "$1" != "--skip-build" ]; then
        build_image
    fi

    # 创建命名空间
    print_info "创建命名空间..."
    kubectl apply -f deploy/k8s/namespace.yaml

    # 部署 PostgreSQL
    print_info "部署 PostgreSQL..."
    kubectl apply -f deploy/k8s/postgres.yaml

    # 部署 Redis
    print_info "部署 Redis..."
    kubectl apply -f deploy/k8s/redis.yaml

    # 等待数据库就绪
    print_info "等待数据库就绪..."
    kubectl wait --namespace=kubepilot --for=condition=ready pod -l app=postgres --timeout=120s

    # 部署 KubePilot
    print_info "部署 KubePilot..."
    kubectl apply -f deploy/k8s/kubepilot.yaml

    # 等待应用就绪
    print_info "等待应用就绪..."
    kubectl wait --namespace=kubepilot --for=condition=ready pod -l app=kubepilot --timeout=120s

    print_info "部署完成！"
    print_info "查看服务: kubectl get pods -n kubepilot"
    print_info "访问方式: kubectl port-forward -n kubepilot svc/kubepilot 8080:8080"
}

# 停止 Docker Compose 服务
stop_docker_compose() {
    print_info "停止 Docker Compose 服务..."
    docker-compose down
    print_info "服务已停止"
}

# 清理所有资源
clean_all() {
    print_warn "即将清理所有资源..."
    read -p "确认继续？(y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # 停止 Docker Compose
        docker-compose down -v 2>/dev/null || true

        # 删除 Kubernetes 资源
        kubectl delete namespace kubepilot 2>/dev/null || true

        print_info "清理完成"
    else
        print_info "已取消"
    fi
}

# 主函数
main() {
    case "$1" in
        docker-compose)
            deploy_docker_compose
            ;;
        k8s)
            deploy_k8s "$2"
            ;;
        build)
            build_image
            ;;
        stop)
            stop_docker_compose
            ;;
        clean)
            clean_all
            ;;
        help|*)
            show_help
            ;;
    esac
}

main "$@"
