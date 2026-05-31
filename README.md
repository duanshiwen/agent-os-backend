# AgentOS Backend

Agent OS 联邦化网络的服务端进程（Go + Rust SDK FFI），提供身份接入、即时通讯、跨设备同步、知识库 Hub、插件市场和多服务器支持。

## 平台规划文档

Stage 0 已将 backend 从“分阶段 MVP”重新锁定为完整平台化交付路线。当前平台基线与架构边界见：

- [`docs/platform-roadmap.md`](docs/platform-roadmap.md) — 一步到位平台完成路线图
- [`docs/system-capability-matrix.md`](docs/system-capability-matrix.md) — 已实现 / 部分实现 / 待实现能力矩阵
- [`docs/architecture-boundaries.md`](docs/architecture-boundaries.md) — Go/Rust、存储、同步、KB、SAGE、治理、联邦等边界冻结
- [`docs/phase1-status.md`](docs/phase1-status.md) — 当前 Phase 1 → M3.5 / Stage 2 实现状态和验证记录

## 快速开始

### 1. 环境准备

```bash
# 复制环境变量
cp .env.example .env

# 启动基础依赖服务（PostgreSQL + Redis + MinIO）
# 不会下载 BGE-M3，适合普通 API / Go 开发。
docker compose up -d postgres redis minio
```

如需同时在 Docker 中启动 API：

```bash
docker compose up -d api
```

如需启用真实 KB 语义搜索，再单独启动 embedding workers；首次启动会下载 BGE-M3 模型，耗时较长且需要数 GB 磁盘空间：

```bash
docker compose up -d embedding-worker embedding-job-worker
```

### 2. 运行

```bash
# 开发模式（需要 Air 热重载）
go install github.com/air-verse/air@latest
air

# 或直接运行
go run ./cmd/server
```

### 2.1 Rust SDK FFI（Purego + 动态库）

后端通过 Purego 调用仓库内置的 connor-agent-core Rust SDK 动态库完成身份验签：

```text
internal/runtime/darwin-arm64/libagentos_ffi.dylib
```

启动后端：

```bash
# from the agent-os-backend repository root
go run ./cmd/server
```

`/health` 会返回 `identity_verifier` 检查项。如果内置动态库不可加载，服务会启动失败。

### 3. API 文档

#### 身份认证

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/auth/challenge` | 发起 Ed25519 挑战 |
| POST | `/api/v1/auth/verify` | 验证签名，获取 JWT |
| POST | `/api/v1/auth/register` | **Gone**：直接注册已禁用；新用户必须通过 `/auth/challenge` + `/auth/verify` 并经过准入策略 |

#### 用户

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/users/me` | 获取当前用户信息 |
| PUT | `/api/v1/users/me` | 更新用户信息 |
| POST | `/api/v1/devices/pairing/start` | 已登录旧设备发起 QR 配对会话 |
| POST | `/api/v1/devices/pairing/claim` | 新设备扫码后提交 QR payload、设备公钥和签名完成配对 |
| POST | `/api/v1/users/me/devices` | **Gone**：直接配对已禁用；必须使用 QR-only 配对流程 |
| GET | `/api/v1/users/me/devices` | 获取设备列表 |

#### 即时通讯

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/conversations` | 创建会话 |
| GET | `/api/v1/conversations` | 获取会话列表 |
| GET | `/api/v1/conversations/:id` | 获取会话详情 |
| GET | `/api/v1/conversations/:id/messages` | 获取消息历史 |
| POST | `/api/v1/conversations/:id/participants` | 添加参与者 |
| DELETE | `/api/v1/conversations/:id/participants/me` | 退出会话 |

#### Skill 设置

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/skills/settings` | 获取当前用户的 Skill 设置 |
| PUT | `/api/v1/skills/settings/:skill_id` | 更新 Skill 配置，并产生 `skill.updated` 同步事件 |
| POST | `/api/v1/skills/settings/:skill_id/enable` | 启用 Skill，并产生 `skill.enabled` 同步事件 |
| POST | `/api/v1/skills/settings/:skill_id/disable` | 禁用 Skill，并产生 `skill.disabled` 同步事件 |

#### Agent 设置

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/agents/settings` | 获取当前用户的 Agent 设置 |
| PUT | `/api/v1/agents/settings/:agent_id` | 更新 Agent 设置，并产生 `agent.updated` 同步事件 |

#### 服务器列表

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/servers` | 获取当前用户的服务器连接列表 |
| POST | `/api/v1/servers` | 添加服务器连接，并产生 `server.added` 同步事件 |
| PUT | `/api/v1/servers/:id` | 更新服务器连接，并产生 `server.updated` 同步事件 |
| DELETE | `/api/v1/servers/:id` | 移除服务器连接，并产生 `server.removed` 同步事件 |

Skill / Agent / Server 变更接口支持可选 `client_event_id`，用于同步写入幂等。

#### 跨设备同步

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/sync/events` | 从设备 cursor 或显式 `after_sequence` 拉取同步事件 |
| POST | `/api/v1/sync/ack` | 设备确认已持久化应用到的最后 sequence |

拉取示例：

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/sync/events?after_sequence=0&limit=100"
```

响应 `data` 为稳定 pull envelope：

```json
{
  "events": [],
  "next_after_sequence": 123,
  "has_more": false,
  "server_time": 1780054321000,
  "schema_version": 1
}
```

确认示例：

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"last_sequence": 123}' \
  http://localhost:8080/api/v1/sync/ack
```

也可以使用脚本做手动 smoke 验证：

```bash
TOKEN="<jwt>" ./scripts/smoke-sync.sh
ACK_SEQUENCE=123 TOKEN="<jwt>" ./scripts/smoke-sync.sh
```

M2.1 配置对象同步的本地 smoke 验证：

```bash
./scripts/smoke-sync-settings.sh
```

完整契约见 `docs/sync-contract.md`。

#### KB Hub 生产化治理（Stage 2）

Stage 2 将 KB Hub 从发布/安装/搜索基础能力推进到可治理的本地知识市场基础。当前已实现：

- collection source / copyright declarations；
- collection review 状态：`pending`、`approved`、`rejected`、`takedown`、`archived`；
- public collection 只暴露 `status=published` 且 `review_status=approved` 的合集；
- user moderation report；
- admin review / takedown / report resolve；
- snapshot `active` / `archived` lifecycle；
- snapshot diff；
- subscription `expires_at` 和过期清理；
- 对高影响 KB governance 操作写入 audit events。

Owner / user APIs：

| 方法 | 路径 | 描述 |
|------|------|------|
| PUT | `/api/v1/kb/collections/:id/declarations` | 更新 collection 来源/版权声明，更新后回到 `pending` review 状态 |
| POST | `/api/v1/kb/collections/:id/reports` | 举报 collection |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/archive` | 归档 snapshot；不会修改 snapshot 内容 |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/restore` | 恢复 archived snapshot |
| GET | `/api/v1/kb/collections/:id/snapshot-diff?from_snapshot_id=...&to_snapshot_id=...` | 比较两个 snapshot 的 added / removed / changed / unchanged entries |

Admin APIs：

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/admin/kb/collections/review` | 查看 review queue；支持 `review_status`、`limit`、`offset` |
| POST | `/api/v1/admin/kb/collections/:id/review` | 审核、拒绝或 takedown collection |
| GET | `/api/v1/admin/kb/moderation/reports` | 查看 moderation reports；支持 `status`、`limit`、`offset` |
| POST | `/api/v1/admin/kb/moderation/reports/:report_id/resolve` | 标记 report 为 `resolved` 或 `dismissed` |
| POST | `/api/v1/admin/kb/subscriptions/expire` | 将已过期 active subscriptions 标记为 `expired` |

示例：更新 collection 声明：

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_declaration":"Original notes","copyright_declaration":"Owned by author"}' \
  http://localhost:8080/api/v1/kb/collections/$COLLECTION_ID/declarations
```

示例：admin 审核通过：

```bash
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"review_status":"approved","reason":"source declaration accepted"}' \
  http://localhost:8080/api/v1/admin/kb/collections/$COLLECTION_ID/review
```

示例：snapshot diff：

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/kb/collections/$COLLECTION_ID/snapshot-diff?from_snapshot_id=$FROM_SNAPSHOT_ID&to_snapshot_id=$TO_SNAPSHOT_ID"
```

Stage 2 尚未包含 Federated KB Discovery；KB Hub 仍保持 local-server scoped。

#### KB Hub Billing / Entitlements（Stage 2）

Stage 2 继续补齐 KB Hub 商业化基础能力：

- collection entitlement mode：`free`、`paid`、`trial`、`granted`；
- billing interval：`none`、`month`、`year`；
- versioned billing plans；
- subscription entitlement type、renewal status、current period；
- invoice / invoice item；
- refund request / admin resolution；
- billing dispute / admin resolution；
- contributor payout period aggregation and admin paid marker。

User / owner APIs：

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/billing/invoices` | 查看当前用户 invoices |
| GET | `/api/v1/billing/invoices/:invoice_id` | 查看 invoice detail 和 items |
| POST | `/api/v1/billing/refunds` | 发起退款请求 |
| GET | `/api/v1/billing/refunds` | 查看当前用户退款请求 |
| POST | `/api/v1/billing/disputes` | 发起账单争议 |
| GET | `/api/v1/billing/disputes` | 查看当前用户账单争议 |
| GET | `/api/v1/billing/payout-periods` | contributor 查看 payout periods |
| GET | `/api/v1/kb/collections/:id/billing-plans` | collection owner 查看 billing plan versions |

Admin APIs：

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/admin/kb/billing/invoices` | 创建并 issue invoice |
| POST | `/api/v1/admin/kb/billing/invoices/:invoice_id/pay` | 标记 invoice paid |
| POST | `/api/v1/admin/kb/billing/refunds/:refund_id/resolve` | 审核退款：`approved` / `rejected` / `processed` |
| POST | `/api/v1/admin/kb/billing/disputes/:dispute_id/resolve` | 处理争议：`accepted` / `rejected` / `cancelled` |
| POST | `/api/v1/admin/kb/billing/payout-periods/:payout_id/pay` | 标记 contributor payout period paid |

示例：设置 paid monthly entitlement：

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"is_free":false,"pricing_model":"monthly","monthly_price":9900,"entitlement_mode":"paid","billing_interval":"month","currency":"CNY","trial_days":7}' \
  http://localhost:8080/api/v1/kb/collections/$COLLECTION_ID/pricing
```

示例：创建 invoice：

```bash
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"'$USER_ID'","collection_id":"'$COLLECTION_ID'","currency":"CNY","items":[{"description":"Monthly KB access","quantity":1,"unit_price":9900}]}' \
  http://localhost:8080/api/v1/admin/kb/billing/invoices
```

仍未包含真实外部支付通道、自动扣款、税务/发票导出。

#### Unified Background Job Runner（Stage 2）

服务端现在内置统一 background job runner，用于承载周期性维护任务。当前覆盖：

- offline message cleanup；
- sync event cleanup；
- expired sensitive operation confirmation cleanup；
- KB subscription expiry cleanup；
- 可选的 in-process KB embedding worker。

默认配置：

| 环境变量 | 默认值 | 描述 |
|----------|--------|------|
| `BACKGROUND_RUN_ON_START` | `false` | 服务启动时是否立即执行一次维护任务 |
| `BACKGROUND_OFFLINE_CLEANUP_INTERVAL_SECONDS` | `3600` | offline message cleanup 间隔 |
| `BACKGROUND_SYNC_CLEANUP_INTERVAL_SECONDS` | `21600` | sync event cleanup 间隔 |
| `BACKGROUND_SENSITIVE_CONFIRMATION_INTERVAL_SECONDS` | `900` | sensitive confirmation cleanup 间隔 |
| `BACKGROUND_KB_SUBSCRIPTION_EXPIRY_INTERVAL_SECONDS` | `900` | KB subscription expiry cleanup 间隔 |
| `BACKGROUND_EMBEDDING_WORKER_ENABLED` | `false` | 是否在 API server 进程内启动 embedding worker |
| `BACKGROUND_EMBEDDING_WORKER_ID` | 自动生成 | in-process embedding worker ID |
| `BACKGROUND_EMBEDDING_WORKER_BATCH_SIZE` | `8` | embedding worker batch size |
| `BACKGROUND_EMBEDDING_WORKER_POLL_INTERVAL_SECONDS` | `2` | embedding worker poll interval |

生产部署建议仍优先使用独立 worker 进程运行 embedding pipeline；`BACKGROUND_EMBEDDING_WORKER_ENABLED=true` 主要用于单机开发、测试或轻量部署。

#### KB Hub 语义搜索（M3.5）

KB Hub semantic search 使用 PostgreSQL + pgvector 持久化 embedding，并通过异步队列生成向量：

- 默认模型：`BAAI/bge-m3`
- 默认维度：`1024`
- Go API server 不直接加载模型
- `cmd/worker` 负责 claim PostgreSQL durable queue 并调用 embedding provider
- `services/embedding-worker` 是本地 FastAPI embedding model worker

本地启用方式：

```bash
# .env：本机 go run ./cmd/server 时使用 localhost
EMBEDDING_PROVIDER=local_http
EMBEDDING_ENDPOINT=http://localhost:8091
EMBEDDING_MODEL=BAAI/bge-m3
EMBEDDING_DIMENSIONS=1024

# 启动 PostgreSQL+pgvector、Redis、MinIO、embedding model worker、Go embedding job worker
docker compose up -d postgres redis minio embedding-worker embedding-job-worker
```

如果 API 也运行在 Docker Compose 内，`embedding-job-worker` 已在 compose 中使用容器网络地址 `http://embedding-worker:8091`。

手动检查真实 embedding worker（会触发/依赖本地 BGE-M3 环境）：

```bash
./scripts/smoke-kb-embedding-worker.sh
```

BGE-M3 首次下载需要数 GB 空间。Docker Compose 默认把模型缓存 bind mount 到 `${EMBEDDING_MODEL_CACHE_DIR:-./.cache/embedding-models}`，避免 Docker named volume 空间不足；如需改位置：

```bash
EMBEDDING_MODEL_CACHE_DIR=/path/with/free-space docker compose up -d embedding-worker
```

不下载 BGE-M3 的端到端确定性 smoke（验证 publish → durable queue → Go worker → semantic search）：

```bash
APP_PORT=18080 EMBEDDING_PROVIDER=deterministic EMBEDDING_MODEL=deterministic-test EMBEDDING_DIMENSIONS=1024 go run ./cmd/server
BASE_URL=http://localhost:18080 ./scripts/smoke-kb-semantic-deterministic.sh
```

真实 BGE-M3 / `local_http` 端到端 smoke（验证真实 embedding worker → publish → durable queue → Go worker → pgvector → semantic search）：

```bash
docker compose up -d postgres redis minio embedding-worker
APP_PORT=18081 EMBEDDING_PROVIDER=local_http EMBEDDING_ENDPOINT=http://localhost:8091 EMBEDDING_MODEL=BAAI/bge-m3 EMBEDDING_DIMENSIONS=1024 go run ./cmd/server
BASE_URL=http://localhost:18081 EMBEDDING_ENDPOINT=http://localhost:8091 ./scripts/smoke-kb-semantic-local-http.sh
```

查询 snapshot embedding 状态：

```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/kb/collections/$COLLECTION_ID/snapshots/$SNAPSHOT_ID/embedding-status
```

搜索模式：

- `mode=lexical`：普通关键词搜索
- `mode=semantic`：只使用 ready embeddings；不可用时返回显式错误
- `mode=hybrid`：semantic 不可用时降级 lexical，并返回 `semantic_available=false` 与原因

普通 Go CI 使用 deterministic provider / SQLite fallback，不下载真实 BGE-M3。

#### SAGE Plugin Runtime / Control-Plane Smoke

SAGE Backend 是开放平台控制面，不执行第三方插件 Flow。可用自包含 smoke 验证 object upload → icon/package binding → developer submit → admin review → user install/grant → sync events → policy bundle → mock client flow call → invocation/report → developer metrics：

```bash
# 需要 PostgreSQL/Redis/MinIO 与后端 API 已启动。
# 脚本会启动 examples/sage-plugins/hotel-booking/mock_server.py 作为第三方 Plugin Server mock。
BASE_URL=http://localhost:8080 ./scripts/smoke-sage-plugin-runtime.sh
```

该 smoke 会通过 ObjectService / MinIO 上传插件 icon 与 package mock 资产，并验证 catalog 只返回安全资产 metadata（object id、filename、content type、hash、size）。它也会在本地 PostgreSQL 中把临时 smoke 用户标记为 admin，以覆盖 review route。正式产品仍需要独立 admin bootstrap 策略。

#### WebSocket

```
ws://localhost:8080/api/v1/ws?token=<jwt>
```

**客户端消息类型：**
- `message.send` — 发送消息
- `offline.fetch` — 获取离线消息
- `message.ack` — 确认离线消息已送达
- `ping` — 心跳

**服务端消息类型：**
- `message.new` — 新消息
- `message.ack` — 消息发送确认
- `offline.batch` — 离线消息批量推送
- `presence.update` — 在线状态变更
- `sync.event` — 跨设备同步事件通知
- `pong` — 心跳响应
- `error` — 错误

## 项目结构

```
agent-os-backend/
├── cmd/server/          # 入口
├── internal/
│   ├── config/          # 配置加载 + DB/Redis 初始化
│   ├── handler/         # HTTP 处理器
│   ├── middleware/       # CORS、JWT、限流
│   ├── model/           # GORM 数据模型
│   ├── pkg/response/    # 统一响应格式
│   ├── repository/      # 数据访问层
│   ├── router/          # 路由注册
│   ├── service/         # 业务逻辑层（含 Purego FFI verifier）
│   └── ws/              # WebSocket Hub + 消息分发
├── migrations/          # SQL 迁移文件
├── docker-compose.yml   # 开发环境
└── .env.example         # 环境变量模板
```

## 架构

```
┌─────────────────────────────────┐
│         客户端 (AgentOS)        │
│      WebSocket + HTTP REST      │
└────────────────┬────────────────┘
                 │
┌────────────────▼────────────────┐
│         Go 网络服务层           │
│  API GW │ WS Hub │ IM Router   │
│  Identity │ Conversation │ ...  │
└───────┬─────────────────┬───────┘
        │ Purego / dylib  │
┌───────▼────────┐ ┌──────▼─────────┐
│ Rust SDK FFI   │ │     存储层     │
│ libagentos_ffi │ │ PostgreSQL等   │
└────────────────┘ └────────────────┘
```
