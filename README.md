# AgentOS Backend

Agent OS 单服务器服务端进程（Go + Rust SDK FFI），提供身份接入、即时通讯、跨设备同步、知识库 Hub、SAGE Plugin Open Platform、对象存储、平台治理与运维能力。Federation / 多服务器网络暂时不做；现有服务器列表 API 仅用于同步用户本地服务器配置。

## 平台规划文档

Backend 当前以 `main` 上的单服务器平台内核为基线推进；过时的 Phase/M2/M3 进度记录已清理，当前只保留 living docs：

- [`docs/platform-roadmap.md`](docs/platform-roadmap.md) — 当前平台完成路线图与下一步工作
- [`docs/system-capability-matrix.md`](docs/system-capability-matrix.md) — 已实现 / 部分实现 / 待实现能力矩阵
- [`docs/architecture-boundaries.md`](docs/architecture-boundaries.md) — Go/Rust、存储、同步、KB、SAGE、治理、对象存储等边界冻结
- [`docs/stage5a-client-integration-handoff.md`](docs/stage5a-client-integration-handoff.md) — Stage 5A 客户端集成契约交接

当前特别注意：Skill Hub MVP 已实现 registry / package versioning / catalog / install library / admin takedown / publisher restriction。Skill settings sync 仍保留为兼容旧客户端的轻量设置同步接口。尚未实现评分、下载统计、商业化、复杂审核、后端执行 Skill 等非 MVP 能力。

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
| POST | `/api/v1/conversations` | 创建会话，产生 `conversation.created` 同步事件 |
| GET | `/api/v1/conversations` | 获取会话列表 |
| GET | `/api/v1/conversations/:id` | 获取会话详情 |
| PATCH | `/api/v1/conversations/:id` | 更新会话名称/地理元数据，产生 `conversation.updated` 同步事件 |
| POST | `/api/v1/conversations/:id/read-state` | 标记会话已读，产生 actor-owned `conversation.read` 同步事件 |
| GET | `/api/v1/conversations/:id/messages` | 获取消息历史 |
| PUT | `/api/v1/conversations/:id/messages/:message_id` | 编辑本人发送的消息，产生 `message.updated` 同步事件 |
| DELETE | `/api/v1/conversations/:id/messages/:message_id` | 软删除本人发送的消息，产生 `message.deleted` 同步事件 |
| POST | `/api/v1/conversations/:id/messages/:message_id/reactions` | 添加消息 reaction，产生 `message.reaction_added` 同步事件 |
| DELETE | `/api/v1/conversations/:id/messages/:message_id/reactions/:emoji` | 移除消息 reaction，产生 `message.reaction_removed` 同步事件 |
| POST | `/api/v1/conversations/:id/participants` | 添加参与者，产生 `participant.added` 同步事件 |
| PATCH | `/api/v1/conversations/:id/participants/:user_id` | 更新参与者角色，产生 `participant.updated` 同步事件 |
| DELETE | `/api/v1/conversations/:id/participants/:user_id` | 管理员移除参与者，产生 `participant.removed` 同步事件 |
| DELETE | `/api/v1/conversations/:id/participants/me` | 退出会话，产生 `participant.removed` 同步事件 |

#### Skill Hub

Skill Hub MVP 是开放技能分享目录。Backend 负责 Skill metadata、manifest 版本快照、基础校验、目录、安装库、对象资源绑定和最低限度治理；不执行 Skill 代码，不做商业化、评分、下载统计或复杂人工审核。

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/skills/catalog` | 公开 Skill 目录搜索，支持 `q`、`category`、`limit`、`offset` |
| GET | `/api/v1/skills/catalog/:skill_key` | 获取公开 Skill 详情 |
| POST | `/api/v1/skills` | 创建 Skill draft |
| POST | `/api/v1/skills/:skill_id/versions` | 提交 Skill manifest 版本；校验通过后 MVP 直接公开发布 |
| GET | `/api/v1/skills/:skill_id/versions/:version_id/validation` | 获取版本校验结果 |
| POST | `/api/v1/skills/catalog/:skill_key/install` | 安装 Skill，产生 `skill.installed` 同步事件 |
| GET | `/api/v1/skills/installations` | 获取当前用户 Skill Library |
| PUT | `/api/v1/skills/installations/:installation_id/config` | 更新安装配置 / track mode / pin version，产生 `skill.updated` 同步事件 |
| POST | `/api/v1/skills/installations/:installation_id/enable` | 启用已安装 Skill，产生 `skill.enabled` 同步事件 |
| POST | `/api/v1/skills/installations/:installation_id/disable` | 禁用已安装 Skill，产生 `skill.disabled` 同步事件 |
| DELETE | `/api/v1/skills/installations/:installation_id` | 卸载 Skill，产生 `skill.uninstalled` 同步事件 |
| POST | `/api/v1/admin/skills/:skill_id/takedown` | 管理员下架 Skill；隐藏目录但保留安装历史 |
| POST | `/api/v1/admin/skills/publishers/:publisher_id/restrict` | 管理员限制发布者继续上传 Skill |
| POST | `/api/v1/admin/skills/publishers/:publisher_id/lift-restriction` | 管理员解除发布者上传限制 |

#### Skill 设置（兼容旧客户端）

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

Federation / Federated KB Discovery 暂时不做；KB Hub 保持 single-server / local-server scoped。

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

Stage 3A Platform Ops Gate 增加了维护任务运行历史和 admin 操作入口：

- `GET /api/v1/admin/ops/background-job-runs`
- `GET /api/v1/admin/ops/background-job-runs/:id`
- `POST /api/v1/admin/ops/background-jobs/run-once`

这些接口均受 JWT 与 `AdminMiddleware` 保护。

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

#### Local Release Gate

默认 release gate 会运行 Go tests、shell syntax checks、Python syntax checks，不要求 PostgreSQL 或真实 BGE-M3：

```bash
./scripts/release-gate-local.sh
```

本地 PostgreSQL 已启动时，可打开 migration apply gate：

```bash
RUN_MIGRATION_GATE=1 ./scripts/release-gate-local.sh
```

该 gate 会创建 disposable database，按顺序 apply `migrations/*.sql`，完成后自动删除数据库。若本机没有 `psql`，脚本会 fallback 到 `docker compose exec postgres psql`。

本地完整栈已启动时，可加 live smoke：

```bash
RUN_LIVE_SMOKES=1 BASE_URL=http://localhost:8080 ./scripts/release-gate-local.sh
```

真实 BGE-M3 / `local_http` 语义 smoke 可单独打开：

```bash
RUN_LOCAL_HTTP_SEMANTIC=1 EMBEDDING_ENDPOINT=http://localhost:8091 ./scripts/release-gate-local.sh
```

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
- `message.send` — 发送消息；payload 支持 `conversation_id`、`type`、`content`、`metadata`、`reply_to`、`thread_id`、`visibility`、`client_event_id`
- `offline.fetch` — 获取离线消息
- `message.ack` — 确认离线消息已送达（当前确认的是 `offline_messages.id`）
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

### Governance Enforcement Modes

Stage 4D governance enforcement is enabled in safe rollout mode by default:

```bash
GOVERNANCE_ENFORCEMENT_MODE=observe
```

Supported values are:

- `disabled` — skip governance evaluation and allow execution;
- `observe` — evaluate and persist policy decisions without blocking business execution;
- `enforce` — block denied operations and require valid approval receipts for approval-gated operations.

Domain-specific overrides are available:

```bash
GOVERNANCE_ENFORCEMENT_SAGE_MODE=enforce
GOVERNANCE_ENFORCEMENT_OBJECT_MODE=observe
GOVERNANCE_ENFORCEMENT_KB_MODE=disabled
```

Governance errors returned by SAGE, Object, and KB surfaces include stable machine-readable codes:

- `governance_denied`
- `governance_approval_required`
- `governance_approval_invalid`

Stage 4D error responses also include stable `details` when the failure comes from `GovernanceEnforcer`: `policy_decision_id`, subject type/id, capability key, risk level, decision/reason, and approval receipt expiry metadata when available.

Approval receipt consumption is bound to actor, subject type, subject id, and capability key when used through `GovernanceEnforcer`, so an approval token for one operation cannot authorize another operation. Admin approval operations remain one-time-token safe: raw tokens are only returned on create/reissue responses, list APIs never expose tokens, reissue rotates the stored token hash and invalidates old tokens, and revoked/consumed/expired receipts cannot be reissued.

Admin governance APIs:

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/admin/governance/capabilities` | 创建 capability definition |
| GET | `/api/v1/admin/governance/capabilities` | 查看 capability definitions |
| POST | `/api/v1/admin/governance/policy-rules` | 创建 policy rule |
| GET | `/api/v1/admin/governance/policy-rules` | 查看 policy rules |
| POST | `/api/v1/admin/governance/kill-switches` | 创建 kill switch |
| POST | `/api/v1/admin/governance/evaluate` | 手动评估并持久化 policy decision |
| GET | `/api/v1/admin/governance/summary?window=24` | 查看治理汇总窗口 |
| GET | `/api/v1/admin/governance/policy-decisions` | 查看 policy decisions，支持 subject/capability/decision/risk 过滤 |
| GET | `/api/v1/admin/governance/policy-decisions/:id` | 查看单个 policy decision |
| POST | `/api/v1/admin/governance/approval-receipts` | 创建 approval receipt，并只在响应中返回一次 raw token |
| GET | `/api/v1/admin/governance/approval-receipts` | 查看 approval receipts，不暴露 raw token |
| POST | `/api/v1/admin/governance/approval-receipts/:id/reissue-token` | 对 pending 且未过期 receipt 轮换 raw token；旧 token 立即失效 |
| POST | `/api/v1/admin/governance/approval-receipts/:id/revoke` | 撤销 pending approval receipt，阻止后续消费或 reissue |
| GET | `/api/v1/admin/governance/scan-results` | 查看 scanner findings |
| POST | `/api/v1/admin/governance/scan-results/:id/resolve` | 标记 scanner finding resolved |

When a local stack is running, verify governance evaluation and audit-chain compatibility with:

```bash
BASE_URL=http://localhost:8080 ./scripts/smoke-governance-enforcement.sh
```

To verify enforce-mode denial behavior, stable error codes/details, admin evidence APIs, governance summary, and Stage 4D approval receipt reissue/revoke semantics:

```bash
BASE_URL=http://localhost:8080 ./scripts/smoke-governance-enforce-readiness.sh
```
