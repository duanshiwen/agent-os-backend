# AgentOS Backend

Agent OS 联邦化网络的服务端进程（Go + Rust Sidecar），提供身份接入、即时通讯、跨设备同步、知识库 Hub、插件市场和多服务器支持。

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

### 3. API 文档

#### 身份认证

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/auth/challenge` | 发起 Ed25519 挑战 |
| POST | `/api/v1/auth/verify` | 验证签名，获取 JWT |
| POST | `/api/v1/auth/register` | 注册新用户 |

#### 用户

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/users/me` | 获取当前用户信息 |
| PUT | `/api/v1/users/me` | 更新用户信息 |
| POST | `/api/v1/users/me/devices` | 配对新设备 |
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
│   ├── service/         # 业务逻辑层
│   ├── sidecar/         # Rust Sidecar gRPC 客户端
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
└────────────────┬────────────────┘
                 │ gRPC / Unix Socket
┌────────────────▼────────────────┐
│       Rust Sidecar 层           │
│     (connor-agent-core)         │
└────────────────┬────────────────┘
                 │
┌────────────────▼────────────────┐
│         存储层                  │
│ PostgreSQL │ Redis │ S3/MinIO  │
└─────────────────────────────────┘
```
