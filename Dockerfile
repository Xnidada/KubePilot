# 构建阶段
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建二进制
RUN CGO_ENABLED=0 GOOS=linux go build -o kubepilot ./cmd/server/

# 运行阶段
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /build/kubepilot .

# 复制前端文件
COPY frontend/dist ./web

# 复制配置文件
COPY configs ./configs

# 暴露端口
EXPOSE 8080

# 启动命令
CMD ["./kubepilot"]
