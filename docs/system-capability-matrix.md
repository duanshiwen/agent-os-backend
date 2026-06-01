# AgentOS Backend System Capability Matrix

Updated: 2026-06-01
Status source: code inspection, docs cleanup scan, `go test ./...`, `./scripts/release-gate-local.sh`, and targeted Rust SDK bridge/FFI test evidence.

Legend:

- ✅ Implemented and covered by tests or smoke docs.
- 🟡 Partial implementation or foundation exists, but not platform-complete.
- ⬜ Planned / not implemented.
- 🚫 Explicit non-goal for current architecture.

## 1. Verification Baseline

```text
Backend branch: main
Backend: go test ./...
Result: 189 passed in 11 packages

Backend release gate: ./scripts/release-gate-local.sh
Result: passed

Rust SDK targeted bridge/FFI check: cargo test -p agentos-client-bridge -p agentos-ffi --locked
Result: 18 passed in 6 suites
```

Obsolete Phase/M2/M3 progress-history docs were retired; this matrix is the current implementation status index.

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
| Conversation | Private/group/agent conversation creation and participant lifecycle | ✅ | `ConversationService`; routes; emits `conversation.created`, `conversation.updated`, `participant.added`, `participant.updated`, `participant.removed` | moderation/reporting remains future work |
| Messaging | Message persistence, lifecycle, reactions, and read cursors | ✅ | `MessageService`; `messages`; sender edit/delete; reactions; actor-owned `conversation.read`; `reply_to` / `thread_id` / `visibility`; message-level `client_event_id` | social read receipts and moderation remain product choices |
| Messaging | Offline fetch/ack foundation | ✅ | `offline_messages`; WS dispatcher | retention policy and delivery observability; ack naming still needs product cleanup |
| Messaging | Rich message types | ✅ | `messages.type` supports text/system plus image/file/audio/video/card metadata contract hooks | Object/Asset service still owns binary lifecycle |
| WebSocket | Runtime hub and dispatcher | ✅ | `internal/ws`; `message.send` supports reply/thread/visibility/client_event_id; `typing.start/stop`; `offline.fetch`, `message.ack`, `sync.event`, `ping` | broader presence scoping can be refined later |
| Sync | Event stream | ✅ | `sync_events`; `SyncService` | retention/compaction |
| Sync | Per-user monotonic sequence | ✅ | `sync_sequences` | migration/recovery tooling |
| Sync | Pull and ack APIs | ✅ | `/sync/events`, `/sync/ack`; tests | sync snapshot/repair API |
| Sync | Idempotency | ✅ | `client_event_id` partial uniqueness | cross-object conflict UX |
| Sync | Profile sync | ✅ | `profile.updated` | client fixtures for all platforms |
| Sync | Conversation and participant sync | ✅ | `conversation.created`, `conversation.updated`, `conversation.read`, `participant.added`, `participant.updated`, `participant.removed` with self-contained payloads | richer client reconstruction fixtures |
| Sync | Message sync | ✅ | `message.created`, `message.updated`, `message.deleted`, `message.reaction_added`, `message.reaction_removed` with self-contained payloads | richer conversation reconstruction fixtures |
| Sync | Skill settings sync | ✅ | settings routes and `skill.updated/enabled/disabled` events remain for compatibility | schema version evolution |
| Sync | Agent settings sync | ✅ | settings routes and events | schema version evolution |
| Skill Hub | Registry / package versions / catalog | ✅ | `skills`, `skill_versions`, `skill_installations`, `skill_publisher_restrictions`; public `/skills/catalog`; manifest validation | richer package scanning, quality signals, client runtime consumption |
| Skill Hub | User install library / admin takedown | ✅ | install/pin/latest/update config/enable/disable/uninstall, takedown, publisher restriction APIs; emits `skill.installed/uninstalled/enabled/disabled/updated` | ratings/download metrics, monetization, complex moderation/review remain out of MVP |
| Sync | Server list and plugin sync | ✅ | `/servers` routes and `plugin.installed/uninstalled/enabled/disabled/permission_granted/permission_revoked` events | not federation |
| Sync | Personal knowledge sync | ✅ | entry CRUD, tombstone, version conflict, content hash | restore operation and full client merge engine |
| Sync | Plugin sync taxonomy | ✅ | SAGE install/uninstall/enable/disable and permission grant/revoke emit `plugin.*` sync events; Stage 5A fixture and tests cover them | schema version negotiation and full client merge UX |
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
| Governance | Capability taxonomy | ✅ | `capability_definitions`; admin create/list APIs; tests | ongoing taxonomy quality and product naming |
| Governance | Policy engine | ✅ | `policy_rules`, `policy_decisions`; allow/approval/deny decisions; enforcer modes disabled/observe/enforce | policy DSL and richer condition evaluation deferred |
| Governance | Tool definition scanning | 🟡 | `governance_scan_results`; SAGE manifest scanner persists findings; admin list/resolve APIs | richer scanner coverage, typosquatting quality, and LLM-assisted review deferred |
| Governance | Response inspection | ⬜ | none | unsafe output / secret detection |
| Governance | Approval receipts | ✅ | one-time expiring receipts; token hash storage; Stage 4C actor/subject/capability-bound consume path | frontend approval UX and admin approval workflow polish |
| Audit | Queryable audit event log | ✅ | `audit_events`; admin list endpoint; admission/device/sensitive-operation events | hash chain / tamper-evidence |
| Security | Sensitive operation confirmation | ✅ | password-backed one-time confirmation tokens; device/admission enforcement | broaden to plugin grants, billing, and high-risk admin operations |
| Security | Kill switch | ✅ | `kill_switches`; admin create API; policy evaluation override | richer admin revoke/expire/list operations |
| Server config | Local server list sync | ✅ | `user_server_connections` and `/servers` routes | local configuration only; not Federation |
| Deferred scope | Federation / multi-server networking | 🚫 | intentionally removed from current roadmap | no server discovery, signed server handshake, remote plugin discovery, remote KB discovery, or cross-server sync |
| Deferred scope | Cross-server DB replication | 🚫 | intentionally not part of design | preserve single-server product boundary |
| Admin | Admission admin | ✅ | admin admission routes | broader admin console API |
| Admin | KB admin/moderation | ✅ | review list, review/takedown, report list/resolve, subscription expiry API | reviewer roles and moderation dashboard |
| Admin | Plugin admin/review | ✅ | SAGE review queue, approve/reject/request_changes, suspend APIs | richer reviewer roles and policy dashboards |
| Admin | Security/admin ops | 🟡 | audit listing, governance admin read/summary/scan-resolve APIs, unified background runner foundation, and background job admin APIs | dashboards and richer role policy |
| Observability | `/health` | ✅ | database/redis/verifier checks | metrics and structured status |
| Observability | `/ready` | ✅ | database readiness endpoint | broader dependency readiness policy |
| Observability | Request IDs | ✅ | `X-Request-ID` middleware; generated or propagated | structured log integration |
| Observability | Structured logging | ⬜ | standard log today | slog and correlation-aware logs |
| Ops | Background job runner | ✅ | offline cleanup, sync cleanup, sensitive confirmation cleanup, KB subscription expiry, optional embedding worker, persisted run history, admin run-once trigger | embedding worker per-batch telemetry |
| Release | Go unit/integration tests | ✅ | 189 passed in 11 packages | CI automation wrapper |
| Release | Rust SDK bridge/FFI tests | ✅ | targeted `agentos-client-bridge` + `agentos-ffi`: 18 passed in 6 suites | full SDK workspace gate remains useful before SDK releases |
| Release | Smoke scripts | ✅ | `scripts/release-gate-local.sh` runs tests/syntax checks plus optional migration apply, object storage, SAGE, governance enforcement, governance enforce-readiness, and real local_http semantic smokes | CI full platform release gate |

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
- SAGE plugin review queue, review decision, and suspend APIs;
- governance capabilities, policy rules, kill switches, evaluate, summary, policy decisions, approval receipts, scan result list/resolve APIs.

## 4. Stage 0 Decisions from Matrix

1. The next platform phase must not duplicate KB Hub basics; it should harden and govern them.
2. Plugin/SAGE should be built as a complete subsystem with governance and audit dependencies, not as ungoverned tool execution.
3. Federation / multi-server networking is removed from the current roadmap; keep `/servers` as local user configuration sync only.
4. The existing plugin model placeholders should be treated as provisional and can be replaced by proper migrations/models if needed.
5. Real BGE-M3 operational path is now verified locally; search quality next step is chunk-level passage retrieval plus retrieval-quality fixtures, not provider-architecture churn.
