# AgentOS Backend

AgentOS Backend is a single-server AgentOS platform service written in Go, with a narrow Rust SDK FFI boundary for identity verification and client sync projection support.

It provides identity access, QR-only device pairing, instant messaging, cross-device sync, personal knowledge sync, KB Hub, Skill Hub, SAGE Plugin Open Platform, object storage, governance, audit, billing foundations, semantic search, and platform operations.

Federation / multi-server networking is intentionally out of scope for the current architecture. The server-list APIs only synchronize a user's local server configuration across devices; they do not implement cross-server trust, discovery, or data replication.

## Current Reality Baseline

Last verified locally: 2026-06-02.

Active milestone: **Stage 5B — Runtime Consumption Loop**.

Stage 5B rule:

> No new platform surface unless it is consumed by Client / SDK / Agent Runtime in an end-to-end user-visible loop.

This branch shifts the project from control-plane expansion to runtime capability realization: Skill Hub, SAGE Plugin Platform, KB Hub, Sync, and Governance must prove that backend state changes reach runtime behavior.

```text
Backend branch: main
Backend latest observed commit: 7977ba6 feat: add contact management sync
Backend tests: go test ./... -> 218 passed in 11 packages
Backend release gate: ./scripts/release-gate-local.sh -> passed
SDK bridge/FFI targeted tests: cargo test -p agentos-client-bridge -p agentos-ffi --locked -> 21 passed in 7 suites
```

Default release gate currently covers:

- Go tests
- shell syntax checks
- Python syntax checks
- bundled FFI artifact checks
- current-platform FFI artifact contract
- Stage 5A client-ready sync bridge smoke
- SAGE client runtime contract gate
- governance client error contract gate

Optional gates are available for PostgreSQL migrations, live-stack smokes, real local HTTP semantic search, and Stage 5B SDK/runtime contract checks.

## Current Capability Summary

| Domain | Status | Notes |
|---|---:|---|
| Identity / Auth | ✅ Implemented | Ed25519 challenge/verify, JWT, Rust FFI verifier, direct registration disabled |
| Admission | ✅ Foundation | protocol / invitation / approval policies, admin policy APIs, sensitive confirmation for policy changes |
| Device Management | ✅ Foundation | QR-only pairing, device rename/revoke, direct device create disabled |
| Password / Sensitive Ops | ✅ Foundation | password setup/change, one-time sensitive operation confirmation tokens |
| IM / WebSocket | ✅ Foundation | conversations, participants, messages, edit/delete, reactions, read state, offline fetch/ack, WS dispatcher |
| Cross-device Sync | ✅ Stage 5A contract | pull/ack, capabilities, monotonic sequences, idempotency, profile/message/conversation/skill/plugin/knowledge/contact/server sync families |
| Personal Knowledge | ✅ Foundation | user-scoped entries, tombstones, versions, content hash, conflict semantics |
| Contacts | ✅ Foundation | user-scoped contact CRUD, tombstones, optimistic version conflict handling, sync events, SDK bridge projection |
| Object Storage | ✅ Foundation | object records, upload intent, complete, download URL, delete, MinIO backend |
| KB Hub | ✅ Large foundation | collections, immutable snapshots, governance, subscriptions, installed access, usage, billing ledger, refunds/disputes/payout foundations |
| Semantic Search | 🟡 Foundation | pgvector, embedding jobs, deterministic provider, local HTTP BGE-M3 worker contract; chunking/rerank/eval still missing |
| Skill Hub | ✅ Backend MVP + signals | registry, immutable versions, validation, catalog, install library, sync events, takedown, publisher restriction, ratings, downloads, recommendation sort |
| SAGE Plugin Platform | ✅ Backend control plane | registry, manifest validation, review/catalog, install/grants, policy bundle, invocation/report, developer metrics, asset bindings |
| Governance / Audit | ✅ Foundation | capability taxonomy, policy rules, kill switches, approval receipts, scanner findings, stable client errors, audit hash chain |
| Background Jobs / Ops | ✅ Foundation | cleanup jobs, subscription expiry, optional embedding worker, run history, admin run-once trigger |
| Commercialization | 🟡 Internal foundation | invoices/refunds/disputes/payout records exist; real external payment/reconciliation/tax/export not implemented |
| Federation | 🚫 Non-goal | no server discovery, signed server handshake, remote plugin/KB discovery, or cross-server sync |

## Stage 5B Runtime Consumption Loop

The backend is no longer the main bottleneck for basic platform capability. The active milestone is the runtime consumption loop:

```text
User installs a Skill and/or SAGE Plugin
→ Client syncs installation state
→ SDK/client projection applies sync envelope through FFI bridge
→ Agent Runtime loads installed Skill package/instructions
→ Client executes SAGE Plugin flow under policy bundle and governance guard
→ Backend receives invocation report, sync events, audit evidence, and usage records
```

Stage 5B contract matrix:

| Backend object / surface | Sync/API contract | Client / SDK projection | Runtime consumer | Stage 5B proof |
|---|---|---|---|---|
| Skill installation | `skill.installed`, `skill.updated`, `skill.enabled`, `skill.disabled`, `skill.uninstalled` | `ClientReadySyncProjection.skills` | task-scoped Skill instruction bundle | install reaches runtime context; uninstall removes behavior |
| SAGE plugin installation | `plugin.installed`, `plugin.enabled`, `plugin.disabled`, `plugin.uninstalled` | `ClientReadySyncProjection.plugins` | plugin runtime executor | installed plugin can be selected for execution |
| SAGE permission grant | `plugin.permission_granted`, `plugin.permission_revoked` | `ClientReadySyncProjection.plugin_permissions` | policy/permission decision | allow / deny / requires-confirmation decision is deterministic |
| Knowledge entry / KB runtime context | `knowledge.created`, `knowledge.updated`, `knowledge.deleted` plus installed KB access APIs | `ClientReadySyncProjection.knowledge` | citation-ready retriever | runtime can retrieve relevant context with citations |
| Governance errors | stable error codes and policy bundle outcomes | client error handling | execution guard / confirmation UI | high-risk actions are blocked or require confirmation before execution |
| Sync cursor | `/sync/events`, `/sync/ack`, monotonic sequence | `ServerSyncCursor.last_applied_sequence` | durable local projection | hosts ack only after local projection succeeds |

Stage 5B Definition of Done:

1. Skill install state reaches SDK/client projection.
2. Runtime can load/apply a Skill instruction bundle to a task-scoped context.
3. Disabled/uninstalled Skill is no longer applied.
4. SAGE mock/plugin execution can be allowed, denied, or confirmation-gated from projected policy state.
5. SAGE invocation/report remains a backend responsibility after client-side execution.
6. Knowledge projection can supply citation-ready runtime context.
7. Runtime loop checks are covered by SDK tests and optional release-gate commands.
8. Backend `go test ./...` and default `./scripts/release-gate-local.sh` remain green.

Recommended next work:

1. Integrate the SDK Stage 5B projection helpers into the real AgentOS Client UI/runtime.
2. Replace fixture Skill entrypoint content with real Skill package/object download and cache handling.
3. Connect the SAGE decision contract to a real client-side plugin flow executor.
4. Connect KB citations to installed KB collection access and later improve semantic quality with chunking/reranking/evals.
5. Add production-grade metrics, dashboards, runbooks, and CI runtime gates after the loops are productized.

## Quick Start

### 1. Prepare environment

```bash
cp .env.example .env

# Start base dependencies: PostgreSQL + Redis + MinIO.
# This does not download BGE-M3 and is enough for normal API / Go development.
docker compose up -d postgres redis minio
```

Optionally run the API in Docker:

```bash
docker compose up -d api
```

Optionally enable real KB semantic search workers. The first run may download several GB of BGE-M3 model files:

```bash
docker compose up -d embedding-worker embedding-job-worker
```

### 2. Run server

```bash
# Development hot reload.
go install github.com/air-verse/air@latest
air

# Or direct run.
go run ./cmd/server
```

### 3. Run embedding job worker

```bash
go run ./cmd/worker
```

The API server can also run a lightweight in-process embedding worker for development by setting:

```bash
BACKGROUND_EMBEDDING_WORKER_ENABLED=true
```

For production, prefer a separate worker process.

## Rust SDK FFI

Backend uses Purego to load bundled Rust SDK dynamic libraries from:

```text
internal/runtime/darwin-arm64/libagentos_ffi.dylib
internal/runtime/linux-arm64/libagentos_ffi.so
```

Required exported symbols:

```text
agentos_ffi_version
agentos_ffi_free_string
agentos_identity_verify_ed25519_challenge
agentos_apply_knowledge_sync_events_json
agentos_apply_knowledge_sync_pull_response_json
agentos_apply_sync_pull_response_json
```

The server fails startup if the configured identity verifier library cannot be loaded. `/health` exposes verifier status and version metadata.

## API Surface

Base path: `/api/v1`.

### Health

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health checks including database, Redis, and identity verifier |
| GET | `/ready` | Readiness check |

### Auth / Identity

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/challenge` | Create Ed25519 challenge |
| POST | `/api/v1/auth/verify` | Verify signature and issue JWT |
| POST | `/api/v1/auth/register` | Gone; direct registration is disabled |

### User / Device / Sensitive Operations

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/users/me` | Get current user profile |
| PUT | `/api/v1/users/me` | Update current user profile; emits `profile.updated` |
| POST | `/api/v1/users/me/password` | Set password |
| PUT | `/api/v1/users/me/password` | Change password |
| POST | `/api/v1/sensitive-operations/confirmations` | Issue one-time sensitive operation confirmation token |
| POST | `/api/v1/devices/pairing/start` | Old logged-in device starts QR pairing session |
| POST | `/api/v1/devices/pairing/claim` | New device claims QR pairing payload |
| POST | `/api/v1/users/me/devices` | Gone; direct device create is disabled |
| GET | `/api/v1/users/me/devices` | List devices |
| PUT | `/api/v1/users/me/devices/:device_id` | Rename device |
| DELETE | `/api/v1/users/me/devices/:device_id` | Revoke device |

### Conversations / Messages

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/conversations` | Create conversation; emits `conversation.created` |
| GET | `/api/v1/conversations` | List conversations |
| GET | `/api/v1/conversations/:id` | Get conversation |
| PATCH | `/api/v1/conversations/:id` | Update conversation; emits `conversation.updated` |
| POST | `/api/v1/conversations/:id/read-state` | Mark read; emits `conversation.read` |
| GET | `/api/v1/conversations/:id/messages` | List messages |
| PUT | `/api/v1/conversations/:id/messages/:message_id` | Edit own message; emits `message.updated` |
| DELETE | `/api/v1/conversations/:id/messages/:message_id` | Soft-delete own message; emits `message.deleted` |
| POST | `/api/v1/conversations/:id/messages/:message_id/reactions` | Add reaction; emits `message.reaction_added` |
| DELETE | `/api/v1/conversations/:id/messages/:message_id/reactions/:emoji` | Remove reaction; emits `message.reaction_removed` |
| POST | `/api/v1/conversations/:id/participants` | Add participant; emits `participant.added` |
| PATCH | `/api/v1/conversations/:id/participants/:user_id` | Update participant; emits `participant.updated` |
| DELETE | `/api/v1/conversations/:id/participants/:user_id` | Remove participant; emits `participant.removed` |
| DELETE | `/api/v1/conversations/:id/participants/me` | Leave conversation; emits `participant.removed` |

### WebSocket

```text
ws://localhost:8080/api/v1/ws?token=<jwt>
```

Client message types:

- `message.send`
- `offline.fetch`
- `message.ack`
- `typing.start`
- `typing.stop`
- `ping`

Server message types:

- `message.new`
- `message.ack`
- `offline.batch`
- `presence.update`
- `sync.event`
- `pong`
- `error`

### Sync

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/sync/capabilities` | Get sync schema/capability metadata |
| GET | `/api/v1/sync/events` | Pull sync events by cursor / `after_sequence` |
| POST | `/api/v1/sync/ack` | Ack last locally persisted sequence |

Example pull:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/sync/events?after_sequence=0&limit=100"
```

Example response envelope:

```json
{
  "events": [],
  "next_after_sequence": 123,
  "has_more": false,
  "server_time": 1780054321000,
  "schema_version": 1
}
```

Example ack:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"last_sequence": 123}' \
  http://localhost:8080/api/v1/sync/ack
```

### Skill Hub

Skill Hub is an open skill sharing directory. Backend stores metadata, immutable manifest/package versions, validation results, catalog, install library, object bindings, ratings/download signals, and minimal governance. Backend does not execute Skill code.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/skills/catalog` | Public catalog search; supports `q`, `category`, `sort=recommended/downloads/rating/recent`, `limit`, `offset` |
| GET | `/api/v1/skills/catalog/:skill_key` | Public skill detail |
| POST | `/api/v1/skills` | Create skill draft |
| POST | `/api/v1/skills/:skill_id/versions` | Submit manifest/package version |
| GET | `/api/v1/skills/:skill_id/versions/:version_id/validation` | Get validation result |
| POST | `/api/v1/skills/catalog/:skill_key/install` | Install skill; emits `skill.installed` and may increment `download_count` |
| PUT | `/api/v1/skills/catalog/:skill_key/rating` | Rate skill, body `{ "rating": 1..5 }` |
| DELETE | `/api/v1/skills/catalog/:skill_key/rating` | Delete current user's rating |
| GET | `/api/v1/skills/installations` | List current user's Skill Library |
| PUT | `/api/v1/skills/installations/:installation_id/config` | Update config / track mode / pinned version; emits `skill.updated` |
| POST | `/api/v1/skills/installations/:installation_id/enable` | Enable installation; emits `skill.enabled` |
| POST | `/api/v1/skills/installations/:installation_id/disable` | Disable installation; emits `skill.disabled` |
| DELETE | `/api/v1/skills/installations/:installation_id` | Uninstall skill; emits `skill.uninstalled` |
| POST | `/api/v1/admin/skills/:skill_id/takedown` | Admin takedown |
| POST | `/api/v1/admin/skills/publishers/:publisher_id/restrict` | Restrict publisher uploads |
| POST | `/api/v1/admin/skills/publishers/:publisher_id/lift-restriction` | Lift publisher restriction |

Legacy skill settings APIs remain for old clients:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/skills/settings` | List legacy skill settings |
| PUT | `/api/v1/skills/settings/:skill_id` | Update legacy skill config |
| POST | `/api/v1/skills/settings/:skill_id/enable` | Enable legacy skill setting |
| POST | `/api/v1/skills/settings/:skill_id/disable` | Disable legacy skill setting |

### Agent Settings / Server List

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/agents/settings` | List agent settings |
| PUT | `/api/v1/agents/settings/:agent_id` | Update agent setting; emits `agent.updated` |
| GET | `/api/v1/servers` | List local server connections |
| POST | `/api/v1/servers` | Add local server connection; emits `server.added` |
| PUT | `/api/v1/servers/:id` | Update local server connection; emits `server.updated` |
| DELETE | `/api/v1/servers/:id` | Delete local server connection; emits `server.removed` |

### Personal Knowledge / Contacts

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/knowledge/entries` | List personal knowledge entries |
| POST | `/api/v1/knowledge/entries` | Create personal knowledge entry |
| GET | `/api/v1/knowledge/entries/*entry_id` | Get personal knowledge entry |
| PUT | `/api/v1/knowledge/entries/*entry_id` | Update personal knowledge entry |
| DELETE | `/api/v1/knowledge/entries/*entry_id` | Tombstone personal knowledge entry |
| GET | `/api/v1/contacts` | List contacts |
| POST | `/api/v1/contacts` | Create contact |
| GET | `/api/v1/contacts/:contact_id` | Get contact |
| PUT | `/api/v1/contacts/:contact_id` | Update contact |
| DELETE | `/api/v1/contacts/:contact_id` | Tombstone contact |

### Object Storage

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/objects/upload-intents` | Create upload intent and presigned URL |
| POST | `/api/v1/objects/uploads/:id/complete` | Complete upload and verify object metadata |
| GET | `/api/v1/objects/:id` | Get object metadata |
| POST | `/api/v1/objects/:id/download-url` | Create short-lived download URL |
| DELETE | `/api/v1/objects/:id` | Delete / tombstone object after authorization |

### KB Hub

Public APIs:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/kb/public/collections` | List public approved collections |
| GET | `/api/v1/kb/public/search` | Search public KB collections |
| GET | `/api/v1/kb/public/collections/:id` | Get public collection |
| GET | `/api/v1/kb/public/collections/:id/snapshots/:snapshot_id` | Get public snapshot metadata |
| POST | `/api/v1/kb/public/collections/:id/snapshots/:snapshot_id/manifest-download-url` | Create public manifest download URL |

Authenticated owner/user APIs:

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/kb/collections` | Create collection |
| GET | `/api/v1/kb/collections` | List own collections |
| GET | `/api/v1/kb/collections/:id` | Get collection |
| PUT | `/api/v1/kb/collections/:id/pricing` | Update pricing / entitlement mode |
| PUT | `/api/v1/kb/collections/:id/declarations` | Update source/copyright declarations |
| POST | `/api/v1/kb/collections/:id/reports` | Report collection |
| GET | `/api/v1/kb/collections/:id/stats` | Get collection stats |
| GET | `/api/v1/kb/collections/:id/earnings` | List contributor earnings |
| GET | `/api/v1/kb/collections/:id/billing-plans` | List billing plans |
| POST | `/api/v1/kb/collections/:id/snapshots` | Publish immutable snapshot |
| GET | `/api/v1/kb/collections/:id/snapshots` | List snapshots |
| GET | `/api/v1/kb/collections/:id/snapshots/:snapshot_id` | Get snapshot |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/archive` | Archive snapshot |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/restore` | Restore snapshot |
| GET | `/api/v1/kb/collections/:id/snapshot-diff` | Diff snapshots |
| GET | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/embedding-status` | Get embedding status |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/embedding-jobs/retry-failed` | Retry failed embedding jobs |
| POST | `/api/v1/kb/collections/:id/install` | Install/subscribe to collection |
| DELETE | `/api/v1/kb/collections/:id/install` | Cancel subscription |
| GET | `/api/v1/kb/subscriptions` | List subscriptions |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/manifest-download-url` | Installed manifest download URL |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/content-download-url` | Installed entry content download URL |
| POST | `/api/v1/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/fulltext` | Fetch installed entry full text |

Admin KB APIs:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/admin/kb/collections/review` | List collection review queue |
| POST | `/api/v1/admin/kb/collections/:id/review` | Review / reject / takedown collection |
| GET | `/api/v1/admin/kb/moderation/reports` | List moderation reports |
| POST | `/api/v1/admin/kb/moderation/reports/:report_id/resolve` | Resolve moderation report |
| POST | `/api/v1/admin/kb/subscriptions/expire` | Expire overdue subscriptions |

### Billing Foundations

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/billing/account` | Get billing account |
| GET | `/api/v1/billing/transactions` | List billing transactions |
| GET | `/api/v1/billing/invoices` | List invoices |
| GET | `/api/v1/billing/invoices/:invoice_id` | Get invoice detail |
| POST | `/api/v1/billing/refunds` | Request refund |
| GET | `/api/v1/billing/refunds` | List refunds |
| POST | `/api/v1/billing/disputes` | Open billing dispute |
| GET | `/api/v1/billing/disputes` | List billing disputes |
| GET | `/api/v1/billing/payout-periods` | List contributor payout periods |
| POST | `/api/v1/admin/kb/billing/invoices` | Admin create invoice |
| POST | `/api/v1/admin/kb/billing/invoices/:invoice_id/pay` | Admin mark invoice paid |
| POST | `/api/v1/admin/kb/billing/refunds/:refund_id/resolve` | Admin resolve refund |
| POST | `/api/v1/admin/kb/billing/disputes/:dispute_id/resolve` | Admin resolve dispute |
| POST | `/api/v1/admin/kb/billing/payout-periods/:payout_id/pay` | Admin mark payout paid |

### SAGE Plugin Open Platform

Backend is the control plane. It validates, reviews, catalogs, installs, grants permissions, produces policy bundles, records invocations, accepts execution reports, and tracks usage/metrics. Backend does not execute third-party plugin Flow definitions and does not host arbitrary plugin code.

Public catalog:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/sage/catalog/plugins` | Search public plugin catalog |
| GET | `/api/v1/sage/catalog/plugins/:plugin_key` | Get public plugin detail |

Developer/user APIs:

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/sage/plugins` | Create plugin |
| POST | `/api/v1/sage/plugins/:plugin_id/versions` | Submit manifest version |
| GET | `/api/v1/sage/plugins/:plugin_id/versions/:version_id/validation` | Get validation result |
| POST | `/api/v1/sage/catalog/plugins/:plugin_key/install` | Install plugin |
| GET | `/api/v1/sage/installations` | List installations |
| DELETE | `/api/v1/sage/installations/:installation_id` | Uninstall plugin |
| POST | `/api/v1/sage/installations/:installation_id/disable` | Disable plugin |
| POST | `/api/v1/sage/installations/:installation_id/enable` | Enable plugin |
| POST | `/api/v1/sage/installations/:installation_id/grants` | Grant permission |
| DELETE | `/api/v1/sage/installations/:installation_id/grants/:grant_id` | Revoke grant |
| GET | `/api/v1/sage/installations/:installation_id/policy-bundle` | Fetch client runtime policy bundle |
| POST | `/api/v1/sage/invocations` | Create invocation record |
| POST | `/api/v1/sage/invocations/:invocation_id/reports` | Submit execution report |
| GET | `/api/v1/developer/sage/plugins/:plugin_id/metrics` | Developer metrics |

Admin APIs:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/admin/sage/plugins/review-queue` | List plugin review queue |
| POST | `/api/v1/admin/sage/plugins/:plugin_id/versions/:version_id/review` | Review plugin version |
| POST | `/api/v1/admin/sage/plugins/:plugin_id/suspend` | Suspend plugin |

### Governance / Audit / Ops

Governance enforcement modes:

```bash
GOVERNANCE_ENFORCEMENT_MODE=observe
GOVERNANCE_ENFORCEMENT_SAGE_MODE=enforce
GOVERNANCE_ENFORCEMENT_OBJECT_MODE=observe
GOVERNANCE_ENFORCEMENT_KB_MODE=disabled
```

Supported modes:

- `disabled` — skip governance evaluation and allow execution
- `observe` — evaluate and persist policy decisions without blocking business execution
- `enforce` — block denied operations and require valid approval receipts for approval-gated operations

Stable governance error codes:

- `governance_denied`
- `governance_approval_required`
- `governance_approval_invalid`

Admin APIs:

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/admin/audit/events` | List audit events |
| GET | `/api/v1/admin/audit/verify-chain` | Verify audit hash chain |
| POST | `/api/v1/admin/governance/capabilities` | Create capability definition |
| GET | `/api/v1/admin/governance/capabilities` | List capability definitions |
| POST | `/api/v1/admin/governance/policy-rules` | Create policy rule |
| GET | `/api/v1/admin/governance/policy-rules` | List policy rules |
| POST | `/api/v1/admin/governance/kill-switches` | Create kill switch |
| POST | `/api/v1/admin/governance/evaluate` | Evaluate policy |
| GET | `/api/v1/admin/governance/summary` | Governance summary |
| GET | `/api/v1/admin/governance/policy-decisions` | List policy decisions |
| GET | `/api/v1/admin/governance/policy-decisions/:id` | Get policy decision |
| POST | `/api/v1/admin/governance/approval-receipts` | Create approval receipt |
| GET | `/api/v1/admin/governance/approval-receipts` | List approval receipts without raw tokens |
| POST | `/api/v1/admin/governance/approval-receipts/:id/revoke` | Revoke approval receipt |
| POST | `/api/v1/admin/governance/approval-receipts/:id/reissue-token` | Reissue one-time raw token |
| POST | `/api/v1/governance/approval-receipts/consume` | Consume approval receipt |
| GET | `/api/v1/admin/governance/scan-results` | List scanner findings |
| POST | `/api/v1/admin/governance/scan-results/:id/resolve` | Resolve scanner finding |
| GET | `/api/v1/admin/ops/background-job-runs` | List background job runs |
| GET | `/api/v1/admin/ops/background-job-runs/:id` | Get background job run |
| POST | `/api/v1/admin/ops/background-jobs/run-once` | Run background jobs once |

## Semantic Search

KB Hub semantic search uses PostgreSQL + pgvector and an asynchronous embedding queue.

Defaults:

```text
Model: BAAI/bge-m3
Dimensions: 1024
Provider: local HTTP embedding worker or deterministic test provider
```

Local HTTP setup:

```bash
EMBEDDING_PROVIDER=local_http
EMBEDDING_ENDPOINT=http://localhost:8091
EMBEDDING_MODEL=BAAI/bge-m3
EMBEDDING_DIMENSIONS=1024

docker compose up -d postgres redis minio embedding-worker embedding-job-worker
```

Deterministic smoke without downloading BGE-M3:

```bash
APP_PORT=18080 \
EMBEDDING_PROVIDER=deterministic \
EMBEDDING_MODEL=deterministic-test \
EMBEDDING_DIMENSIONS=1024 \
go run ./cmd/server

BASE_URL=http://localhost:18080 ./scripts/smoke-kb-semantic-deterministic.sh
```

Real local HTTP semantic smoke:

```bash
docker compose up -d postgres redis minio embedding-worker

APP_PORT=18081 \
EMBEDDING_PROVIDER=local_http \
EMBEDDING_ENDPOINT=http://localhost:8091 \
EMBEDDING_MODEL=BAAI/bge-m3 \
EMBEDDING_DIMENSIONS=1024 \
go run ./cmd/server

BASE_URL=http://localhost:18081 \
EMBEDDING_ENDPOINT=http://localhost:8091 \
./scripts/smoke-kb-semantic-local-http.sh
```

Search modes:

- `mode=lexical` — keyword search
- `mode=metadata` — metadata search
- `mode=semantic` — ready embeddings only; unavailable state is explicit
- `mode=hybrid` — semantic + lexical when available; lexical fallback when semantic is unavailable

## Verification Commands

Default local verification:

```bash
go test ./...
./scripts/release-gate-local.sh
```

Targeted SDK bridge/FFI verification from the SDK repository:

```bash
cd /Users/yakii/code/agent-os/Infrastructure/connor-agent-core
cargo test -p agentos-client-bridge -p agentos-ffi --locked
```

Optional backend gates:

```bash
# Apply migrations against disposable local PostgreSQL database.
RUN_MIGRATION_GATE=1 ./scripts/release-gate-local.sh

# Run live smokes against a running local stack.
RUN_LIVE_SMOKES=1 BASE_URL=http://localhost:8080 ./scripts/release-gate-local.sh

# Run real local_http semantic smoke.
RUN_LOCAL_HTTP_SEMANTIC=1 EMBEDDING_ENDPOINT=http://localhost:8091 ./scripts/release-gate-local.sh

# Run Stage 5B SDK/runtime consumption contract checks.
RUN_RUNTIME_LOOP_GATES=1 SDK_REPO=/Users/yakii/code/agent-os/Infrastructure/connor-agent-core ./scripts/release-gate-local.sh
```

Useful smoke scripts:

```bash
./scripts/smoke-sync.sh
./scripts/smoke-sync-settings.sh
./scripts/smoke-stage5a-sync-bridge.sh
./scripts/smoke-object-storage.sh
./scripts/smoke-kb-snapshot.sh
./scripts/smoke-kb-install.sh
./scripts/smoke-kb-access.sh
./scripts/smoke-kb-semantic-deterministic.sh
./scripts/smoke-kb-embedding-worker.sh
./scripts/smoke-kb-semantic-local-http.sh
./scripts/smoke-sage-plugin-runtime.sh
./scripts/smoke-governance-enforcement.sh
./scripts/smoke-governance-enforce-readiness.sh
```

## Project Structure

```text
agent-os-backend/
├── cmd/
│   ├── server/                 # API server entrypoint
│   └── worker/                 # embedding/background worker entrypoint
├── internal/
│   ├── config/                 # configuration, database, Redis
│   ├── handler/                # HTTP handlers
│   ├── middleware/             # CORS, JWT, admin, rate limit, request ID
│   ├── model/                  # GORM models
│   ├── pkg/response/           # response helpers
│   ├── repository/             # data access layer
│   ├── router/                 # route registration
│   ├── runtime/                # bundled Rust FFI artifacts
│   ├── service/                # business services and FFI verifier
│   └── ws/                     # WebSocket hub and dispatcher
├── migrations/                 # SQL migrations
├── scripts/                    # release gates and smoke scripts
├── services/embedding-worker/  # Python local embedding worker
├── examples/                   # sample SAGE plugin server etc.
├── docker-compose.yml
├── Dockerfile
└── README.md
```

## Architecture

```text
AgentOS Client
  │
  ├── HTTP REST
  ├── WebSocket
  │
  ▼
Go Backend
  ├── Identity / Admission / Device
  ├── Conversation / Message / WebSocket
  ├── Sync Mediator
  ├── Personal Knowledge / Contacts / Server List
  ├── Object Storage API
  ├── KB Hub
  ├── Skill Hub
  ├── SAGE Plugin Open Platform
  ├── Governance / Audit
  ├── Billing Foundations
  └── Background Jobs
  │
  ├── Purego / FFI ──► Rust SDK dynamic library
  │                   ├── identity-core verification
  │                   └── client sync projection bridge
  │
  └── Storage
      ├── PostgreSQL + pgvector
      ├── Redis
      └── MinIO
```

## Explicit Non-goals

- No Federation / multi-server networking in the current roadmap.
- No server discovery or `.well-known/agentos-server.json`.
- No signed server handshake or cross-server identity trust.
- No remote plugin discovery or remote KB discovery.
- No cross-server data replication.
- Backend does not execute third-party SAGE Flow definitions.
- Backend does not host arbitrary plugin code.
- Backend does not execute Skill package code.
- Skill execution belongs to AgentOS Client / SDK / Agent Runtime.

## Known Gaps

- AgentOS Client integration is not complete.
- SDK / Agent Runtime does not yet fully consume installed Skill Hub packages as executable agent behavior.
- SAGE Plugin backend contract exists, but real client-side plugin runtime execution still needs product integration.
- Semantic search needs chunk-level passage retrieval, reranking, logs, feedback, and evaluation fixtures.
- Governance needs richer scanners, policy conditions, response inspection, incident workflows, and admin UX.
- Production observability needs structured logs, metrics, dashboards, and runbooks.
- Commercialization needs real payment provider integration, renewal collection, reconciliation, tax/export, entitlement revocation, and payout operations.
