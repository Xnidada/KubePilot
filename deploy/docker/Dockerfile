# Frontend build stage
FROM node:22-alpine AS frontend-builder

WORKDIR /build/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# Backend build stage
FROM golang:1.26-alpine AS backend-builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o kubepilot ./cmd/server/

# A matching kubectl is needed by the existing workload executor and Gateway API installer.
FROM alpine:3.23 AS kubectl-fetcher
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates curl && \
    case "$TARGETARCH" in \
      amd64) digest=a2e984a18a0c063279d692533031c1eff93a262afcc0afdc517375432d060989 ;; \
      arm64) digest=58f82f9fe796c375c5c4b8439850b0f3f4d401a52434052f2df46035a8789e25 ;; \
      *) exit 1 ;; \
    esac && \
    curl -fLsS --retry 3 "https://dl.k8s.io/release/v1.35.0/bin/linux/${TARGETARCH}/kubectl" -o /tmp/kubectl && \
    echo "$digest  /tmp/kubectl" | sha256sum -c - && \
    install -m 0755 /tmp/kubectl /usr/local/bin/kubectl

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=backend-builder /build/kubepilot .
COPY --from=frontend-builder /build/frontend/dist ./web
COPY --from=kubectl-fetcher /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY configs ./configs

EXPOSE 8080

CMD ["./kubepilot"]
