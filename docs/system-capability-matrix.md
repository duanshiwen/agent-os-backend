# AgentOS Backend System Capability Matrix

Updated: 2026-05-31
Status source: code inspection, docs inspection, `go test ./...`, and Rust SDK workspace test evidence.

Legend:

- ✅ Implemented and covered by tests or smoke docs.
- 🟡 Partial implementation or foundation exists, but not platform-complete.
- ⬜ Planned / not implemented.
- 🚫 Explicit non-goal for current architecture.

## 1. Verification Baseline

```text
Backend: go test ./...
Result: 150 passed in 11 packages

Rust SDK: cargo test --workspace --all-targets --locked
Result: 1560 passed, 2 ignored, 110 suites
```

Backend working tree at Stage 0 start had one existing README documentation diff that narrows Docker Compose startup guidance to avoid accidental BGE-M3 download.

## 2. Capability Matrix

| Domain | Capability | Status | Current implementation / evidence | Gap to platform-complete |
|---|---|---:|---|---|
| Identity | Ed25519 challenge creation | ✅ | `POST /api/v1/auth/challenge`; `AuthChallenge`; tests | Operational monitoring and audit events |
| Identity | Ed25519 signature verification | ✅ | `POST /api/v1/auth/verify`; `SignatureVerifier`; FFI verifier | Version/export symbol release gate |
| Identity | Rust FFI identity verifier | ✅ | `agentos_identity_verify_ed25519_challenge`; backend integration tests | Multi-platform bundled library release checks |
| Identity | Direct registration disabled | ✅ | `/auth/register` returns Gone | None |
| Admission | protocol / invitation / approval policies | ✅ | `AdmissionService`; admin routes; tests; confirmation-gated policy updates | Admin bootstrap hardening |
| Admission | Admin admission APIs | ✅ | policy get/update, invitation code, request approve/reject; invitation code updates require confirmation | Broader fine-grained role policy |
| Device | QR-only pairing start/claim | ✅ | `DevicePairingService`, one-time bcrypt token, QR hash | old-device biometric UX on clients |
| Device | Direct device create disabled | ✅ | `/users/me/devices` returns Gone | None |
| Device | Device revoke / rename / trust | ✅ | rename/revoke APIs, active/revoked status, audit events, confirmation-gated revoke | Active session invalidation and richer trust posture |
| User profile | Get/update self | ✅ | `/users/me`; emits `profile.updated` | Sensitive field policy |
| Password | Password setup/change/confirmation | ✅ | password set/change routes; one-time sensitive operation confirmation tokens | Password reset/recovery policy |
| Conversation | Private/group/agent conversation creation path | ✅ | `ConversationService`; routes | richer lifecycle events and moderation |
| Messaging | Text message persistence | ✅ | `MessageService`; `messages` | edit/delete/reactions not complete |
| Messaging | Offline fetch/ack foundation | ✅ | `offline_messages`; WS dispatcher | retention policy and delivery observability |
| Messaging | Rich message types | 🟡 | `messages.type` and metadata are flexible | image/voice/file/video/link/kb/plugin card contracts |
| WebSocket | Runtime hub and dispatcher | ✅ | `internal/ws`; `message.send`, `offline.fetch`, `message.ack`, `ping` | conversation lifecycle and richer sync hints |
| Sync | Event stream | ✅ | `sync_events`; `SyncService` | retention/compaction |
| Sync | Per-user monotonic sequence | ✅ | `sync_sequences` | migration/recovery tooling |
| Sync | Pull and ack APIs | ✅ | `/sync/events`, `/sync/ack`; tests | sync snapshot/repair API |
| Sync | Idempotency | ✅ | `client_event_id` partial uniqueness | cross-object conflict UX |
| Sync | Profile sync | ✅ | `profile.updated` | client fixtures for all platforms |
| Sync | Message sync | ✅ | `message.created` | full conversation reconstruction solely from sync not complete |
| Sync | Skill settings sync | ✅ | settings routes and events | schema version evolution |
| Sync | Agent settings sync | ✅ | settings routes and events | schema version evolution |
| Sync | Server list and plugin sync | ✅ | `/servers` routes and `plugin.installed/uninstalled/enabled/disabled/permission_granted/permission_revoked` events | not federation |
| Sync | Personal knowledge sync | ✅ | entry CRUD, tombstone, version conflict, content hash | restore operation and full client merge engine |
| Sync | Plugin sync taxonomy | 🟡 | SAGE subsystem exists but install/grant state is not yet emitted as sync events | plugin install/grant/enable/disable events and tests |
| Sync | Schema version negotiation | ⬜ | schema_version stored | `/sync/capabilities`, compatibility strategy |
| Object storage | Object record metadata | ✅ | `object_records` migration/model | lifecycle cleanup job |
| Object storage | Upload intent / complete / download / delete APIs | ✅ | `ObjectService`; authenticated routes | malware/content scanning and quota policy |
| Object storage | MinIO backend | ✅ | `MinIOStorageService`; compose MinIO | production lifecycle policy and bucket validation gate |
| KB personal | User knowledge entries | ✅ | `user_knowledge_entries` | restore and local client reducer beyond fixture |
| KB Hub | Collection create/list/detail | ✅ | `KBCollection`; owner routes; source/copyright declarations | contributor profile and verification |
| KB Hub | Governance and moderation | ✅ | review statuses, admin review/takedown, reports, report resolution | richer moderation queues and reviewer policy |
| KB Hub | Immutable snapshot publish | ✅ | `KBSnapshot`; manifest and content objects | subscriber impact preview |
| KB Hub | Snapshot lifecycle | ✅ | active/archived status, archive/restore, diff | retention/version cleanup policy |
| KB Hub | Public collection discovery | ✅ | public KB routes gated by published + approved state | ranking/curation quality |
| KB Hub | Install latest/pinned | ✅ | `KBSubscription`; install/list/cancel; optional expiry; entitlement type and renewal status | automated renewal job runner |
| KB Hub | Installed content access gate | ✅ | installed manifest/content/fulltext endpoints require active subscription | entitlement revocation policy |
| KB Hub | Usage metering foundation | ✅ | `KBUsageRecord` | pricing quality and usage invoice automation |
| KB Hub | Billing account/transaction foundation | ✅ | `BillingAccount`, `BillingTransaction`, `KBBillingPlan`, `KBInvoice`, `KBInvoiceItem` | external payment integration |
| KB Hub | Contributor earnings foundation | ✅ | `ContributorEarning`, `ContributorPayoutPeriod` | payout holds, disputes, tax/export docs |
| KB Hub | Refund/dispute foundation | ✅ | `KBRefund`, `KBBillingDispute`; user request/open and admin resolve APIs | payment-provider reconciliation |
| Search | Public lexical search | ✅ | `KBSearchService`; search docs | ranking tuning and language-specific lexical strategy |
| Search | Metadata search | ✅ | `mode=metadata` | marketplace relevance |
| Search | Semantic search deterministic path | ✅ | deterministic provider, pgvector model, smoke script | real BGE-M3 live gate and quality eval |
| Search | Real BGE-M3 worker | ✅ | Python worker, compose service, `scripts/smoke-kb-embedding-worker.sh`, `scripts/smoke-kb-semantic-local-http.sh` | resource constraints, eval fixtures, and search-quality tuning |
| Search | Hybrid fallback | ✅ | semantic unavailable is explicit | quality scoring / rerank |
| Search | Chunk-level passage embeddings | ⬜ | entry-level documents only | chunk tables, chunk jobs, passage retrieval |
| Queue | Embedding durable queue | ✅ | `kb_embedding_jobs`; `FOR UPDATE SKIP LOCKED`; optional in-process worker in unified runner | persisted job history / dead-lettering |
| Plugin | SAGE Open Platform models | ✅ | `sage_*` models and migrations `022_sage_plugin_open_platform.sql`, `023_sage_plugin_asset_bindings.sql` | richer lifecycle states |
| Plugin | Marketplace registry | ✅ | developer create plugin, submit versions, public catalog, repository/service/handler tests | developer profile polish and richer catalog ranking |
| Plugin | Plugin package/icon storage | ✅ | ObjectService/MinIO-backed icon and package bindings; active developer-owned object validation; safe catalog asset metadata | review-time asset scanning and signing |
| Plugin | SAGE Manifest | ✅ | `SAGEManifestValidator`, manifest hash, permission/risk summaries, validation tests | stronger schema evolution and compatibility policy |
| Plugin | Runtime Flow contract | ✅ | mock plugin server + `scripts/smoke-sage-plugin-runtime.sh`; Backend intentionally does not execute Flow | real AgentOS Client integration |
| Plugin | Execution Report | ✅ | invocation and execution report APIs, usage ledger / metrics foundation | idempotency hardening and billing integration |
| Plugin | Installation / grants | ✅ | install/uninstall/enable/disable/grant/revoke APIs, high-risk grants use sensitive confirmation, `plugin.*` sync events | richer grant scopes and real AgentOS Client integration |
| Governance | Capability taxonomy | ⬜ | none | platform-wide capability model |
| Governance | Policy engine | ⬜ | none | allow/approval/deny decisions |
| Governance | Tool definition scanning | ⬜ | none | injection/typosquatting/capability mismatch scanner |
| Governance | Response inspection | ⬜ | none | unsafe output / secret detection |
| Governance | Approval receipts | ⬜ | none | user/admin approval record |
| Audit | Queryable audit event log | ✅ | `audit_events`; admin list endpoint; admission/device/sensitive-operation events | hash chain / tamper-evidence |
| Security | Sensitive operation confirmation | ✅ | password-backed one-time confirmation tokens; device/admission enforcement | broaden to plugin grants, billing, federation admin operations |
| Security | Kill switch | ⬜ | none | plugin/user/capability/server kill switches |
| Federation | Local server list | ✅ | `user_server_connections` | local config only |
| Federation | Server identity | ⬜ | none | server public key and trust records |
| Federation | Well-known discovery | ⬜ | none | `.well-known/agentos-server.json` |
| Federation | Federation handshake | ⬜ | none | signed challenge protocol |
| Federation | Remote plugin discovery | ⬜ | none | metadata APIs and trust policy |
| Federation | Federated KB Discovery | 🚫 | explicitly deferred | not part of current implementation plan; KB Hub remains local-server scoped |
| Federation | Cross-server DB replication | 🚫 | intentionally not part of design | preserve server independence |
| Admin | Admission admin | ✅ | admin admission routes | broader admin console API |
| Admin | KB admin/moderation | ✅ | review list, review/takedown, report list/resolve, subscription expiry API | reviewer roles and moderation dashboard |
| Admin | Plugin admin/review | ✅ | SAGE review queue, approve/reject/request_changes, suspend APIs | richer reviewer roles and policy dashboards |
| Admin | Security/admin ops | 🟡 | audit listing plus unified background runner foundation and background job admin APIs | job dashboards and richer job-specific controls |
| Observability | `/health` | ✅ | database/redis/verifier checks | metrics and structured status |
| Observability | `/ready` | ✅ | database readiness endpoint | broader dependency readiness policy |
| Observability | Request IDs | ✅ | `X-Request-ID` middleware; generated or propagated | structured log integration |
| Observability | Structured logging | ⬜ | standard log today | slog and correlation-aware logs |
| Ops | Background job runner | ✅ | offline cleanup, sync cleanup, sensitive confirmation cleanup, KB subscription expiry, optional embedding worker, persisted run history, admin run-once trigger | embedding worker per-batch telemetry |
| Release | Go unit/integration tests | ✅ | 155 passed | CI migration apply gate |
| Release | Rust SDK tests | ✅ | 1560 passed | backend-pinned FFI artifact gate |
| Release | Smoke scripts | ✅ | `scripts/release-gate-local.sh` runs tests/syntax checks plus optional object storage, SAGE, and real local_http semantic smokes | CI full platform release gate |

## 3. Existing API Surface Summary

Public / unauthenticated:

- `POST /api/v1/auth/challenge`
- `POST /api/v1/auth/verify`
- `POST /api/v1/auth/register` — Gone
- `GET /api/v1/ws?token=...`
- `POST /api/v1/devices/pairing/claim`
- SAGE catalog plugin list/detail;
- `GET /api/v1/kb/public/collections`
- `GET /api/v1/kb/public/search`
- `GET /api/v1/kb/public/collections/:id`
- `GET /api/v1/kb/public/collections/:id/snapshots/:snapshot_id`
- `POST /api/v1/kb/public/collections/:id/snapshots/:snapshot_id/manifest-download-url`

Authenticated:

- device pairing start;
- user profile and devices;
- conversations and messages;
- sync events and ack;
- skill/agent/server settings;
- personal knowledge entries;
- object storage lifecycle;
- billing account/transactions;
- KB collection owner, publishing, pricing, stats, earnings;
- KB install/cancel/list subscriptions;
- installed manifest/content/fulltext access;
- embedding status and retry failed jobs;
- SAGE developer plugin registry, manifest submission/validation, install/grants, policy bundle, invocation/report, developer metrics.

Admin:

- admission policy;
- invitation code;
- admission request list/approve/reject;
- SAGE plugin review queue, review decision, and suspend APIs.

## 4. Stage 0 Decisions from Matrix

1. The next platform phase must not duplicate KB Hub basics; it should harden and govern them.
2. Plugin/SAGE should be built as a complete subsystem with governance and audit dependencies, not as ungoverned tool execution.
3. Multi-server federation should reuse identity, sync references, governance, and trust records; it should not introduce cross-server database replication.
4. The existing plugin model placeholders should be treated as provisional and can be replaced by proper migrations/models if needed.
5. Real BGE-M3 operational path is now verified locally; search quality next step is chunk-level passage retrieval plus retrieval-quality fixtures, not provider-architecture churn.
