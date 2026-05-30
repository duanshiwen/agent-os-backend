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
Result: 135 passed in 11 packages

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
| Sync | Server list sync | ✅ | `/servers` routes and events | not federation |
| Sync | Personal knowledge sync | ✅ | entry CRUD, tombstone, version conflict, content hash | restore operation and full client merge engine |
| Sync | Plugin sync taxonomy | ⬜ | central contract includes `plugin` type but no plugin subsystem | plugin install/grant events and tests |
| Sync | Schema version negotiation | ⬜ | schema_version stored | `/sync/capabilities`, compatibility strategy |
| Object storage | Object record metadata | ✅ | `object_records` migration/model | lifecycle cleanup job |
| Object storage | Upload intent / complete / download / delete APIs | ✅ | `ObjectService`; authenticated routes | malware/content scanning and quota policy |
| Object storage | MinIO backend | ✅ | `MinIOStorageService`; compose MinIO | production lifecycle policy and bucket validation gate |
| KB personal | User knowledge entries | ✅ | `user_knowledge_entries` | restore and local client reducer beyond fixture |
| KB Hub | Collection create/list/detail | ✅ | `KBCollection`; routes | moderation/status workflow |
| KB Hub | Immutable snapshot publish | ✅ | `KBSnapshot`; manifest and content objects | snapshot diff and lifecycle |
| KB Hub | Public collection discovery | ✅ | public KB routes | ranking/curation/moderation |
| KB Hub | Install latest/pinned | ✅ | `KBSubscription`; install/list/cancel | renewal/expiry job |
| KB Hub | Installed content access gate | ✅ | installed manifest/content/fulltext endpoints | per-plan entitlement model |
| KB Hub | Usage metering foundation | ✅ | `KBUsageRecord` | pricing rules, invoices, payouts |
| KB Hub | Billing account/transaction foundation | ✅ | `BillingAccount`, `BillingTransaction` | full ledger invariants and external payment integration |
| KB Hub | Contributor earnings foundation | ✅ | `ContributorEarning` | payout cycles, disputes, tax/export docs |
| Search | Public lexical search | ✅ | `KBSearchService`; search docs | ranking tuning and language-specific lexical strategy |
| Search | Metadata search | ✅ | `mode=metadata` | marketplace relevance |
| Search | Semantic search deterministic path | ✅ | deterministic provider, pgvector model, smoke script | real BGE-M3 live gate and quality eval |
| Search | Real BGE-M3 worker | 🟡 | Python worker exists; compose service exists | live operational verification and resource constraints |
| Search | Hybrid fallback | ✅ | semantic unavailable is explicit | quality scoring / rerank |
| Search | Chunk-level passage embeddings | ⬜ | entry-level documents only | chunk tables, chunk jobs, passage retrieval |
| Queue | Embedding durable queue | ✅ | `kb_embedding_jobs`; `FOR UPDATE SKIP LOCKED` | unified background job system |
| Plugin | Go model placeholders | 🟡 | `Plugin`, `PluginVersion`, `PluginUsageRecord` in models | no migration coverage beyond AutoMigrate expectations, no routes/services |
| Plugin | Marketplace registry | ⬜ | none | models, migrations, repository, service, handlers, tests |
| Plugin | Plugin package storage | ⬜ | object storage could support it | package object records and review workflow |
| Plugin | Tool Manifest | ⬜ | none | SAGE schema validation |
| Plugin | Flow Definition | ⬜ | none | SAGE engine |
| Plugin | Execution Report | ⬜ | none | report ledger, idempotency, billing, audit |
| Plugin | Installation / grants | ⬜ | none | capability grants and sync events |
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
| Admin | KB admin/moderation | ⬜ | none | review/takedown/report APIs |
| Admin | Plugin admin/review | ⬜ | none | review queue and revoke APIs |
| Admin | Security/admin ops | ⬜ | none | audit/security/job dashboards |
| Observability | `/health` | ✅ | database/redis/verifier checks | metrics and structured status |
| Observability | `/ready` | ✅ | database readiness endpoint | broader dependency readiness policy |
| Observability | Request IDs | ✅ | `X-Request-ID` middleware; generated or propagated | structured log integration |
| Observability | Structured logging | ⬜ | standard log today | slog and correlation-aware logs |
| Release | Go unit/integration tests | ✅ | 135 passed | release-gate script |
| Release | Rust SDK tests | ✅ | 1560 passed | backend-pinned FFI artifact gate |
| Release | Smoke scripts | 🟡 | many M2/M3 smoke scripts exist | unified full platform release gate |

## 3. Existing API Surface Summary

Public / unauthenticated:

- `POST /api/v1/auth/challenge`
- `POST /api/v1/auth/verify`
- `POST /api/v1/auth/register` — Gone
- `GET /api/v1/ws?token=...`
- `POST /api/v1/devices/pairing/claim`
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
- embedding status and retry failed jobs.

Admin:

- admission policy;
- invitation code;
- admission request list/approve/reject.

## 4. Stage 0 Decisions from Matrix

1. The next platform phase must not duplicate KB Hub basics; it should harden and govern them.
2. Plugin/SAGE should be built as a complete subsystem with governance and audit dependencies, not as ungoverned tool execution.
3. Multi-server federation should reuse identity, sync references, governance, and trust records; it should not introduce cross-server database replication.
4. The existing plugin model placeholders should be treated as provisional and can be replaced by proper migrations/models if needed.
5. Search quality next step is chunk-level passage retrieval only after real BGE-M3 operational path is verified.
