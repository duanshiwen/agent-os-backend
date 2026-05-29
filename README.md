# AgentOS Backend

Agent OS 联邦化网络的服务端进程（Go + Rust SDK FFI），提供身份接入、即时通讯、跨设备同步、知识库 Hub、插件市场和多服务器支持。

## 快速开始

### 1. 环境准备

```bash
# 复制环境变量
cp .env.example .env

# 启动依赖服务（PostgreSQL + Redis + MinIO）
docker compose up -d
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
