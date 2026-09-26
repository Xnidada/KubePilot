# 前端构建阶段
FROM node:22-alpine AS frontend-builder

WORKDIR /build/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# 后端构建阶段
FROM golang:1.26-alpine AS backend-builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o kubepilot ./cmd/server/

# 运行阶段
FROM alpine:3.19

ARG KUBECTL_VERSION=1.29.14
ARG TARGETARCH=amd64

RUN apk add --no-cache ca-certificates tzdata curl \
    && curl -fsSL -o /usr/local/bin/kubectl \
       "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
    && curl -fsSL -o /tmp/kubectl.sha256 \
       "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" \
    && printf '%s  %s\n' "$(cat /tmp/kubectl.sha256)" /usr/local/bin/kubectl | sha256sum -c - \
    && rm /tmp/kubectl.sha256 \
    && chmod +x /usr/local/bin/kubectl \
    && kubectl version --client \
    && addgroup -S kubepilot && adduser -S -G kubepilot -u 10001 kubepilot

WORKDIR /app

COPY --from=backend-builder /build/kubepilot .
COPY --from=frontend-builder /build/frontend/dist ./web
COPY configs ./configs

EXPOSE 8080

USER 10001:10001

CMD ["./kubepilot"]
