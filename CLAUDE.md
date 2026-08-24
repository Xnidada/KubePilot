# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

KubePilot is an enterprise-grade Kubernetes intelligent operations platform. It provides multi-cluster management, workload lifecycle control, AI-powered diagnostics (via native Tool Calling), backup/inspection/event forwarding, and more — all through a unified web UI.

## Tech Stack

- **Backend**: Go 1.26+, Gin (HTTP), GORM (ORM), client-go (Kubernetes), gorilla/websocket
- **Frontend**: React 18, TypeScript, Ant Design 5, Zustand (state), Vite 8
- **Data**: PostgreSQL 15, Redis 7
- **AI**: OpenAI API / Anthropic API (configurable provider)

## Build & Development Commands

### Backend (Go)
```bash
make build              # Build binary to ./bin/kubepilot
make dev                # Run in dev mode (go run)
make run                # Build + run
make test               # Run all Go tests
make test-coverage      # Tests with HTML coverage report
go test -v ./internal/authz/...  # Run tests for a specific package
make fmt                # Format Go code
make lint               # Lint with golangci-lint
make deps               # Download + tidy Go modules
make db-migrate         # Run database migrations
```

### Frontend (React/TypeScript)
```bash
cd frontend
npm install             # Install dependencies
npm run dev             # Dev server on port 3000 (proxies /api to :8080)
npm run build           # TypeScript check + Vite production build
npm run lint            # ESLint
```

### Docker
```bash
docker-compose up -d    # Start full stack (Postgres + Redis + App)
make docker-build       # Build Docker image
```

### Initial Setup
```bash
cp configs/config.example.yaml configs/config.yaml
# Edit config: set database password, JWT secret, encrypt_key
go run scripts/init-admin.go   # Seed admin user
```

## Architecture

### Backend Layout (`internal/`)

The backend follows a layered architecture: **Router → Middleware → Handler → Service → Repository/K8s**.

- **`cmd/server/main.go`** — Application entry point. Loads config, initializes DB/cache/K8S client, registers and starts modules, sets up Gin router, handles graceful shutdown.
- **`internal/config/`** — YAML config loading (Viper) with `KUBEPILOT_` env var overrides. `EncryptKey()` returns the separate encryption key for kubeconfig at-rest encryption (falls back to JWT secret for legacy compat).
- **`internal/router/`** — Central route registration (`router.go`), RBAC policy definitions (`policies.go`), OAuth handler (`oauth.go`). All API routes live under `/api/v1`.
- **`internal/middleware/`** — Auth (JWT), RBAC/Policy authorization, audit logging, CORS, rate limiting.
- **`internal/authz/`** — Explicit `PolicyRegistry` + `Authorizer`. Two-tier authorization: platform RBAC (role → permissions) + cluster/namespace grants. Fail-closed: no grant = no access.
- **`internal/handler/`** — HTTP handlers organized by domain: `auth`, `cluster`, `workload`, `ops`, `alert`, `system`, `tenant`, `aiops`, `inspection`, `eventforward`, `backup`, `scheduler`, `webhook`.
- **`internal/service/`** — Business logic layer. Key subdirectories: `auth`, `cluster`, `aiops`, `alert`, `access`.
- **`internal/k8s/`** — Kubernetes client manager (`client.go`), cluster DB adapter for kubeconfig storage (`dbadapter.go`), kubectl executor (`executor.go`). Multi-cluster support via per-cluster client instances.
- **`internal/llm/`** — LLM abstraction: `client.go` (interface), `openai.go`, `anthropic.go`. AI Agent uses native Tool Calling with `stage_mutation` for write operations (requires UI confirmation).
- **`internal/model/`** — GORM models + `seed.go` (default roles/users including `aiviewer`). `database.go` handles DB init and auto-migration.
- **`internal/repository/`** — Data access layer.
- **`internal/module/`** — **In-process module framework** (see below).
- **`internal/modules/`** — Concrete module implementations.
- **`internal/pkg/`** — Shared utilities: `cache`, `crypto`, `errors`, `logger`, `netutil` (SSRF validation), `response`, `utils` (JWT), `wsticket` (WebSocket ticket auth).

### Module System

Feature modules are in-process units that implement the `Module` interface (`internal/module/module.go`):

```go
type Module interface {
    Meta() Metadata
    Migrations() []any
    RegisterPolicies(reg *authz.Registry)
    RegisterRoutes(ctx *Context, protected *gin.RouterGroup)
    Menus() []MenuItem
    Permissions() []PermissionDef
    Start(ctx context.Context, host *Host) error
    Stop(ctx context.Context) error
    Health(ctx context.Context) error
}
```

Modules are registered in `internal/modules/register.go` and started in dependency order (topological sort). The `Base` struct provides no-op defaults. Each module can declare dependencies, contribute DB migrations, register RBAC policies and HTTP routes, and expose menu items and health status.

Registered modules: `aiops`, `inspection`, `eventforward`, `scheduler`, `backup`, `webhook`, `appstore`.

Modules are enabled/disabled via `configs/config.yaml` under the `modules` key. The `/api/v1/modules` endpoint exposes health and status.

### Frontend Layout (`frontend/src/`)

- **`api/`** — Axios API calls, one file per domain. `request.ts` sets up the shared axios instance with JWT interceptors.
- **`pages/`** — Route pages: `dashboard`, `cluster`, `workload`, `storage`, `monitor`, `ops`, `scheduler`, `aiops`, `appstore`, `system`.
- **`components/`** — Shared components: `Terminal` (xterm.js), `NodeShell`, `YAMLEditor`, `LogViewer`, `PodFileManager`, `MarkdownRenderer`, `ChatSidebar`, `AIReadOnlyBanner`, `ModuleHealthAlert`, `StatusTag`, etc.
- **`stores/`** — Zustand stores (`auth.ts` for auth state).
- **`hooks/`** — Custom React hooks (session/conversation management).
- **`layouts/`** — `MainLayout.tsx` (app shell with sidebar navigation).
- **`constants/`** — App-wide constants.

Frontend dev server (port 3000) proxies `/api` requests to the backend (port 8080).

### Key Patterns

- **WebSocket auth**: Terminal/NodeShell connections use a ticket-based flow — client requests a short-lived single-use ticket via POST, then opens WebSocket with that ticket. Revoked permissions take effect between ticket issuance and use.
- **AI Agent writes**: Write operations go through `stage_mutation` which stores the proposed change; the UI must confirm before execution. Read operations (list/get/describe/events/logs) auto-execute.
- **aiviewer role**: Can browse all AI Agent conversations (read-only) but cannot execute write operations.
- **Audit logging**: All API requests pass through `AuditMiddleware`; sensitive data is automatically redacted.
- **SSRF protection**: Webhook/event-forward URLs validated via `internal/pkg/netutil/`.
- **Kubeconfig encryption**: Cluster kubeconfigs are encrypted at rest using `security.encrypt_key` (separate from JWT secret).

## Configuration

- **Config file**: `configs/config.yaml` (copy from `configs/config.example.yaml`)
- **Env var override**: All config supports `KUBEPILOT_` prefix env vars (e.g., `KUBEPILOT_DATABASE_PASSWORD`, `KUBEPILOT_JWT_SECRET`)
- **LLM config**: Under `llm` key — `provider` (openai/anthropic), `api_key`, `base_url` (for custom endpoints like Ollama), `model`

## Testing

Go tests use standard `go test`. Test files follow `*_test.go` convention. Key test locations:
- `internal/authz/` — authorization and policy tests
- `internal/config/` — config encryption and module parsing tests
- `internal/module/` — module registry and lifecycle tests
- `internal/middleware/` — audit middleware tests
- `internal/service/aiops/` — agent evaluation tests
- `internal/service/access/` — access control tests
- `internal/pkg/` — wsticket, netutil (SSRF) tests

## Deployment

The project is deployed to a Kubernetes cluster on the test server **`ssh root@192.168.30.112`**. After every code change, a deployment must be completed to facilitate testing.

### Deploy Workflow

1. Build the Docker image and push / transfer to the target server
2. Apply the Kubernetes manifests on the remote cluster:
   ```bash
   ssh root@192.168.30.112
   # On the remote server, apply updated manifests
   kubectl apply -f deploy/k8s/namespace.yaml
   kubectl apply -f deploy/k8s/kubepilot.yaml
   # Or use the deployment script
   ./deploy.sh
   ```
3. Verify the rollout:
   ```bash
   kubectl rollout status deployment/kubepilot -n kubepilot
   ```

**Rule: Every code modification must be followed by a deployment to `192.168.30.112` so changes can be tested immediately.**

## Predefined Roles

| Role | Scope |
|------|-------|
| `admin` | Full access |
| `operator` | Workload management, alerts, scheduler |
| `user` | View + create workloads |
| `viewer` | Read-only (no AI features) |
| `aiviewer` | viewer + browse all AI conversations (no write execution) |
