# AgentOS Backend Phase 1 / Full M3 KB Hub Status

Updated: 2026-05-30
Branch: `feat/kb-hub-m3-full`

## Summary

Phase 1 is the deployability-hardened AgentOS backend foundation. It includes identity, admission, QR-only device pairing, conversation, offline message, WebSocket, and the stable sync foundation.

M2.1 Sync Object Coverage is implemented for the low-risk configuration objects that sit on top of that foundation:

- profile
- message
- skill settings
- agent settings
- server list

M2.2 Knowledge Sync Semantics is implemented for a user's personal knowledge entries. M2.3 documents and verifies the client-facing sync consumer contract with router-level coverage for the two-device knowledge pull/apply/ack flow. M2.4 adds the live integration gate: the backend has been verified against running PostgreSQL/Redis with a real two-device QR pairing + knowledge create/update/conflict/delete/ack smoke. This remains intentionally scoped to cross-device sync of the user's own local knowledge objects. It does not implement KB Hub publishing, marketplace discovery, subscription state, billing, or semantic indexing.

Rust FFI has two verified surfaces: identity verification is in the backend runtime path, while knowledge sync bridge reducer exports are covered as a contract/integration surface for SDK/client consumption and are not part of the backend business write path.

## Implemented and Verified

### Identity and Admission

- `POST /api/v1/auth/challenge`
- `POST /api/v1/auth/verify`
- Ed25519 challenge-response verification through the configured `SignatureVerifier`.
- Rust SDK FFI verifier path through `agentos-ffi` / `identity-core`.
- Direct registration is disabled:
  - `POST /api/v1/auth/register` returns `410 Gone`.
- Admission policies:
  - `protocol`
  - `invitation`
  - `approval`
- Admin admission routes:
  - `GET /api/v1/admin/admission/policy`
  - `PUT /api/v1/admin/admission/policy`
  - `PUT /api/v1/admin/admission/invitation-code`
  - `GET /api/v1/admin/admission/requests`
  - `POST /api/v1/admin/admission/requests/:id/approve`
  - `POST /api/v1/admin/admission/requests/:id/reject`

### QR-only Device Pairing

- `POST /api/v1/devices/pairing/start`
- `POST /api/v1/devices/pairing/claim`
- QR payload includes server id, pairing session id, one-time token, and expiry.
- Pairing token is bcrypt-hashed in storage.
- QR payload hash is stored and checked.
- Pairing session is one-time-use.
- Claim path verifies the new device signature.
- Claim path is transaction-safe through `DevicePairingRepo.ClaimPairingSession`.
- Concurrent duplicate QR claims are covered by regression tests.
- Direct authenticated device creation is disabled:
  - `POST /api/v1/users/me/devices` returns `410 Gone`.

### Conversations and Messaging

- Private conversation creation.
- Group and `agent_conversation` conversation creation path.
- Participant access checks.
- Message persistence through `MessageService`.
- Offline message save/fetch/ack foundation.
- WebSocket dispatcher supports:
  - `message.send`
  - `offline.fetch`
  - `message.ack`
  - `ping`

### Sync Foundation

- `sync_events`
- `sync_cursors`
- `sync_sequences`
- `GET /api/v1/sync/events`
- `POST /api/v1/sync/ack`
- Monotonic per-user sequence assignment.
- Cursor-based pull/ack.
- Explicit `after_sequence` pull support.
- Stable M2 pull response envelope documented in `docs/sync-contract.md`.
- Centralized sync object / operation constants and validation.
- Optional `client_event_id` idempotency with non-empty `(user_id, client_event_id)` uniqueness.
- Real-time notification envelope via `sync.event`.
- `message.created` events are recorded for every conversation participant, including sender cross-device sync.
- `profile.updated` is emitted by `PUT /api/v1/users/me`.

### M2.1 Sync Object Coverage

| Object family | Baseline API | Mutating APIs | Events |
|---|---|---|---|
| `profile` | `GET /api/v1/users/me` | `PUT /api/v1/users/me` | `profile.updated` |
| `message` | `GET /api/v1/conversations`, `GET /api/v1/conversations/:id/messages` | conversation message send paths | `message.created` |
| `skill` | `GET /api/v1/skills/settings` | `PUT /api/v1/skills/settings/:skill_id`, `POST /api/v1/skills/settings/:skill_id/enable`, `POST /api/v1/skills/settings/:skill_id/disable` | `skill.updated`, `skill.enabled`, `skill.disabled` |
| `agent` | `GET /api/v1/agents/settings` | `PUT /api/v1/agents/settings/:agent_id` | `agent.updated` |
| `server` | `GET /api/v1/servers` | `POST /api/v1/servers`, `PUT /api/v1/servers/:id`, `DELETE /api/v1/servers/:id` | `server.added`, `server.updated`, `server.removed` |

M2.1 intentionally uses domain-specific write APIs. It does not expose a generic external sync write endpoint.

### M2.2 Knowledge Sync Semantics

Personal knowledge entry sync is implemented through domain-specific APIs:

| Object family | Baseline API | Mutating APIs | Events |
|---|---|---|---|
| `knowledge` | `GET /api/v1/knowledge/entries`, `GET /api/v1/knowledge/entries?include_deleted=true`, `GET /api/v1/knowledge/entries/*entry_id` | `POST /api/v1/knowledge/entries`, `PUT /api/v1/knowledge/entries/*entry_id`, `DELETE /api/v1/knowledge/entries/*entry_id` | `knowledge.created`, `knowledge.updated`, `knowledge.deleted` |

Implemented semantics:

- stable cross-device `entry_id` object identity;
- slash-containing entry IDs such as `notes/alpha` through wildcard knowledge routes;
- full snapshot payloads for create/update/delete sync events;
- server-side `version` increments for every successful mutation;
- SHA-256 `content_hash` in stored entries and sync payloads;
- tombstone deletes through `status = deleted` and `deleted_at`, not physical deletion;
- active baseline APIs hide tombstones by default;
- `include_deleted=true` exposes tombstones for reconciliation;
- deleted entries cannot be updated by normal `PUT`;
- optional `client_event_id` idempotency for create/update/delete;
- idempotency conflict detection when a reused `client_event_id` refers to another sync event;
- knowledge domain writes and sync event creation occur in one database transaction;
- WebSocket sync notification is sent after commit;
- optional `base_version` optimistic concurrency for `PUT` / `DELETE`;
- stale `base_version` returns `409 conflict`, leaves the entry unchanged, and records no sync event.

The contract is documented in `docs/sync-contract.md` under **Knowledge Entry Sync**.

### M2.3 Client Sync Consumer Contract

M2.3 has an explicit client-facing sync consumer contract in:

```text
docs/client-sync-consumer-contract.md
```

It defines:

- baseline + incremental sync consumption;
- `/api/v1/sync/events` pull rules;
- `/api/v1/sync/ack` cursor/ack discipline;
- WebSocket `sync.event` as a hint, not the canonical event stream;
- required sync event envelope fields;
- apply rules for profile, message, skill, agent, server, and knowledge events;
- knowledge tombstone and version comparison rules;
- `client_event_id` idempotent retry behavior;
- `base_version` optimistic concurrency behavior;
- offline catch-up behavior;
- SDK/client reducer test requirements;
- the gate before KB Hub design/implementation.

Router-level M2.3 coverage is in:

```text
internal/handler/m2_3_client_sync_contract_test.go
```

It verifies a minimal client consumer loop for personal knowledge sync:

1. Device A and Device B authenticate under one user;
2. Device A creates a knowledge entry;
3. Device B pulls `knowledge.created`;
4. Device B applies the event into a local projection and acks the sequence;
5. repeated create with the same `client_event_id` is idempotent and emits no new sync event;
6. Device A updates the entry with `base_version`;
7. Device B pulls/applies/acks `knowledge.updated`;
8. stale update with old `base_version` returns `409` and emits no sync event;
9. Device A deletes with `base_version`;
10. Device B pulls/applies/acks the tombstone;
11. final pull after ack returns no remaining events.

### M2.4 Live Integration Gate

The live two-device sync gate is verified through:

```text
scripts/smoke-live-two-device-knowledge-sync.sh
```

It verifies against a running backend with PostgreSQL/Redis dependencies:

1. `/health` reports database, Redis, and FFI identity verifier as healthy;
2. Device A authenticates through Ed25519 challenge-response;
3. Device A starts QR-only pairing;
4. Device B claims the QR pairing session;
5. Device B authenticates as the same user identity;
6. Device A creates a personal knowledge entry;
7. Device B pulls and acks `knowledge.created`;
8. Device A updates the entry with `base_version`;
9. Device B pulls and acks `knowledge.updated`;
10. Device B attempts a stale update and receives `409` without a new sync event;
11. Device A deletes the entry;
12. Device B pulls and acks the `knowledge.deleted` tombstone;
13. final pull returns no remaining events.

Latest verified result:

```text
Live two-device knowledge sync smoke passed.
Device A: live-knowledge-a-1780121029
Device B: live-knowledge-b-1780121029
Entry:    notes/live-two-device-1780121029
```

### Rust FFI Bridge Contract Surface

The bundled `agentos-ffi` dynamic library exposes and tests:

- `agentos_identity_verify_ed25519_challenge` — backend runtime identity verification path;
- `agentos_apply_knowledge_sync_events_json` — SDK/client knowledge reducer contract surface;
- `agentos_apply_knowledge_sync_pull_response_json` — SDK/client backend pull-response reducer contract surface.

Backend-side integration coverage exists in:

```text
internal/service/ffi_verifier_test.go
```

`TestFFIKnowledgeSyncBridgeIntegration` loads the bundled dynamic library, registers the knowledge sync bridge export, applies `testdata/m2_3_knowledge_sync_pull_response.json`, and verifies that tombstone/version semantics advance the projection cursor correctly.

### Production Migrations

Current migrations:

- `001_init.sql` — users, devices, auth challenges, admission requests, conversations, messages, offline messages.
- `002_server_admission.sql` — persistent server admission policy.
- `003_device_pairing_sessions.sql` — QR-only device pairing sessions.
- `004_phase1_hardening.sql` — sync events/cursors and Phase 1 DB safeguards.
- `005_sync_sequence_foundation.sql` — sync sequence foundation.
- `006_profile_sync.sql` — profile sync support.
- `007_sync_sequences.sql` — per-user monotonic sync sequence storage.
- `008_user_skill_settings.sql` — user skill settings.
- `009_user_agent_settings.sql` — user agent settings.
- `010_user_server_connections.sql` — user server connections.
- `011_user_knowledge_entries.sql` — user-owned personal knowledge entries and tombstone metadata.

## Verification Commands

Run the M2.1 sync settings smoke suite:

```bash
./scripts/smoke-sync-settings.sh
```

Run the M2.2 knowledge sync smoke suite:

```bash
./scripts/smoke-knowledge-sync.sh
```

Run the M2.3 client sync consumer smoke suite:

```bash
./scripts/smoke-m2-3-client-sync.sh
```

Run all Go tests:

```bash
go test ./...
```

Latest verified result:

```text
M2.1 sync settings smoke passed.
M2.2 knowledge sync smoke passed.
M2.3 client sync consumer smoke passed.
Live two-device knowledge sync smoke passed.
Go test: 109 passed in 10 packages
```

Run Rust FFI integration test:

```bash
./scripts/test-ffi-integration.sh
```

Latest verified result:

```text
TestFFIVerifierIntegration: passed
TestFFIKnowledgeSyncBridgeIntegration: passed
cargo test -p agentos-ffi --locked: 6 passed
```

## Smoke Coverage

The router-level smoke tests are in:

```text
internal/handler/phase1_router_smoke_test.go
```

They verify real Gin router wiring for:

1. auth challenge;
2. auth verify;
3. private conversation creation;
4. message-created sync event pull;
5. sync event ack;
6. QR pairing start;
7. QR pairing claim;
8. skill setting mutation and cross-device sync pull;
9. agent setting mutation and cross-device sync pull;
10. server list mutation and cross-device sync pull;
11. knowledge create/update/delete and cross-device sync pull;
12. knowledge tombstone baseline behavior;
13. knowledge stale `base_version` conflicts returning `409` without sync events;
14. M2.3 client-side projection semantics for pull/apply/ack, idempotent retry, stale conflict, and tombstone application.

The service-level smoke and focused sync tests are in:

```text
internal/service/phase1_smoke_test.go
internal/service/skill_settings_test.go
internal/service/agent_settings_test.go
internal/service/server_connections_test.go
internal/service/knowledge_entries_test.go
internal/service/sync_contract_test.go
```

Use this script for focused M2.1 verification:

```bash
./scripts/smoke-sync-settings.sh
```

Use this script for focused M2.2 verification:

```bash
./scripts/smoke-knowledge-sync.sh
```

Use this script for focused M2.3 verification:

```bash
./scripts/smoke-m2-3-client-sync.sh
```

## Run Locally

Start dependencies:

```bash
docker compose up -d
```

Run the server:

```bash
go run ./cmd/server
```

Health check:

```bash
curl http://localhost:8080/health
```

`/health` includes the identity verifier backend and version. The server is expected to fail startup if the configured FFI verifier cannot be loaded.

For live sync pull smoke against a running server, use:

```bash
TOKEN="<jwt>" ./scripts/smoke-sync.sh
```

For the full live two-device QR pairing + knowledge sync gate, start dependencies and the backend, then run:

```bash
docker compose up -d postgres redis minio
AUTO_MIGRATE=true go run ./cmd/server
./scripts/smoke-live-two-device-knowledge-sync.sh
```

## M3.0 Object Storage Foundation

Initial object storage foundation is implemented for the backend-owned MinIO path:

- `object_records` model and migration;
- object storage configuration in `.env.example` and `internal/config`;
- `ObjectStorageBackend` interface;
- `MinIOStorageService` implementation;
- `ObjectService` upload/complete/download/delete lifecycle;
- authenticated object APIs:
  - `POST /api/v1/objects/upload-intents`
  - `POST /api/v1/objects/uploads/:id/complete`
  - `GET /api/v1/objects/:id`
  - `POST /api/v1/objects/:id/download-url`
  - `DELETE /api/v1/objects/:id`
- unit tests with fake object storage backend;
- live MinIO smoke script:

```text
scripts/smoke-object-storage.sh
```

Latest verified result:

```text
Object storage smoke passed.
Object ID:  5a249dd9-79ae-4c74-81aa-06748eba8217
Object URI: minio://agentos-objects/objects/d132733f-0f70-49ed-83d5-34b6dd878239/smoke-object-storage/5a249dd9-79ae-4c74-81aa-06748eba8217/object-smoke-1780121652.txt
SHA-256:    cbb723fef74720a3f6a6d3fe2942cb15eeb47070a79b7dd3a8bd1b8cfeacf588
```

This foundation is intentionally still storage-only. KB Hub snapshot publishing should use this service rather than calling MinIO directly.

## M3.1 KB Hub Snapshot Vertical Slice

The first KB Hub vertical slice is now implemented:

- `kb_collections`, `kb_snapshots`, and `kb_snapshot_entries` migration;
- KB collection create/list/detail APIs;
- immutable snapshot publish API;
- snapshot manifest and per-entry Markdown content written through `ObjectService` / `ObjectStorageBackend`;
- snapshot metadata persisted in PostgreSQL;
- snapshot entries denormalized at publish time to preserve immutability;
- unit tests covering publish and source-entry mutation immutability;
- live smoke script:

```text
scripts/smoke-kb-snapshot.sh
```

Authenticated KB Hub APIs:

- `POST /api/v1/kb/collections`
- `GET /api/v1/kb/collections`
- `GET /api/v1/kb/collections/:id`
- `POST /api/v1/kb/collections/:id/snapshots`
- `GET /api/v1/kb/collections/:id/snapshots`
- `GET /api/v1/kb/collections/:id/snapshots/:snapshot_id`

Latest verified result:

```text
KB snapshot smoke passed.
Collection ID: 6d32bb2e-5a97-4017-8de3-6c8b3c9cec0e
Snapshot v1:   88be81fd-d1c5-4ceb-9c56-c56417c9dc55
Manifest URI:  minio://agentos-objects/objects/bb8ef35c-c414-400f-922f-e5199431206f/kb-snapshots-6d32bb2e-5a97-4017-8de3-6c8b3c9cec0e-v1/27173474-23b1-408f-bfc4-5c38b4747957/manifest.json
Content URI:   minio://agentos-objects/objects/bb8ef35c-c414-400f-922f-e5199431206f/kb-snapshots-6d32bb2e-5a97-4017-8de3-6c8b3c9cec0e-v1-entries/d6314c68-9dec-4b4d-84de-d8318345a684/notes__kb-snapshot-1780124548.md
```

## M3.2 KB Hub Read/Install Consumer Slice

The first KB Hub consumer slice is now implemented:

- public/read-side collection discovery and detail APIs;
- public snapshot detail API;
- public manifest presigned download URL endpoint;
- authenticated install/subscription API;
- `latest` and `pinned` subscription track modes;
- consumer subscription listing;
- subscription schema extension with snapshot reference and active status;
- unit tests covering public read, manifest URL, latest install, pinned install, and subscription upsert;
- live smoke script:

```text
scripts/smoke-kb-install.sh
```

Public/read APIs:

- `GET /api/v1/kb/public/collections`
- `GET /api/v1/kb/public/collections/:id`
- `GET /api/v1/kb/public/collections/:id/snapshots/:snapshot_id`
- `POST /api/v1/kb/public/collections/:id/snapshots/:snapshot_id/manifest-download-url`

Authenticated consumer APIs:

- `POST /api/v1/kb/collections/:id/install`
- `GET /api/v1/kb/subscriptions`

Latest verified result:

```text
KB install smoke passed.
Collection ID: 23c121ae-2123-41a9-85f6-d0718a210b41
Snapshot ID:   83377496-b9d6-4124-a811-b6d870d03c72
Consumer:      kb-consumer-1780125366
```

## M3.3 KB Hub Access/Download Consumer Slice

The installed-consumer access slice is now implemented:

- authenticated installed snapshot manifest download URL endpoint;
- authenticated installed snapshot entry content download URL endpoint;
- active subscription gate for private content URLs;
- public metadata remains readable without installation;
- non-installed users receive `403` for installed content endpoints;
- subscription cancel endpoint;
- cancelled users lose installed content access;
- unit tests covering installed access, stranger denial, content URL retrieval, cancel, and post-cancel denial;
- live smoke script:

```text
scripts/smoke-kb-access.sh
```

Authenticated installed access APIs:

- `POST /api/v1/kb/collections/:id/snapshots/:snapshot_id/manifest-download-url`
- `POST /api/v1/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/content-download-url`
- `DELETE /api/v1/kb/collections/:id/install`

Latest verified result:

```text
KB access smoke passed.
Collection ID: 167ec914-285f-4fb8-bb2d-22acc5e9f77d
Snapshot ID:   48d8ceaa-fbe5-449e-91e8-72e9a1b128d3
Entry Record:  1b2b0c10-816f-468f-a7a2-f07f38ed9ea3
```

## M3 Full KB Hub Completion

Full M3 now implements the KB Hub as a coherent publish / storage / index / discovery / subscription / access / usage / billing / contributor-reporting subsystem.

Implemented full M3 additions:

- public marketplace search/filter over published collections:
  - `q`
  - `owner_id`
  - `is_free`
  - `limit`
  - `offset`
- marketplace collection card metadata:
  - latest snapshot id/version;
  - entry count;
  - total tokens;
  - pricing fields;
  - active install count;
  - manifest/content URL issuance counts;
- collection pricing update and validation:
  - `PUT /api/v1/kb/collections/:id/pricing`;
- owner stats and contributor reporting:
  - `GET /api/v1/kb/collections/:id/stats`;
  - `GET /api/v1/kb/collections/:id/earnings`;
- KB search/index foundation:
  - `kb_search_documents` index rows are created on snapshot publish;
  - public lexical search API: `GET /api/v1/kb/public/search`;
  - metadata search mode;
  - hybrid mode explicitly reports semantic unavailability;
  - semantic mode returns explicit `501 semantic search unavailable` rather than fake results;
- usage metering:
  - public manifest URL issuance;
  - installed manifest URL issuance;
  - installed entry content URL issuance;
  - installed entry fulltext fetch;
- billing ledger foundation:
  - `kb_usage_records`;
  - `billing_accounts`;
  - `billing_transactions`;
  - `contributor_earnings`;
- billing APIs:
  - `GET /api/v1/billing/account`;
  - `GET /api/v1/billing/transactions`;
- installed fulltext fetch endpoint:
  - `POST /api/v1/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/fulltext`;
- full M3 live smoke script:
  - `scripts/smoke-kb-full-m3.sh`.

New full M3 migration:

```text
migrations/015_kb_usage_billing_search.sql
```

Latest local verification:

```text
go test ./...: 119 passed in 10 packages
```

Expected full live verification once the backend and dependencies are running:

```bash
docker compose up -d postgres redis minio
AUTO_MIGRATE=true go run ./cmd/server
./scripts/smoke-object-storage.sh
./scripts/smoke-kb-snapshot.sh
./scripts/smoke-kb-install.sh
./scripts/smoke-kb-access.sh
./scripts/smoke-kb-full-m3.sh
./scripts/smoke-live-two-device-knowledge-sync.sh
```

## M3.5 Async Semantic Search Pipeline

M3.5 adds the first real semantic search pipeline for KB Hub without blocking snapshot publish on model inference.

Implemented M3.5 additions:

- PostgreSQL + pgvector semantic storage:
  - `CREATE EXTENSION IF NOT EXISTS vector` is prepared before AutoMigrate in PostgreSQL mode;
  - local Docker Compose uses `pgvector/pgvector:pg16`;
  - `kb_search_embeddings.embedding` uses `vector(1024)` for BGE-M3 dense embeddings;
- durable PostgreSQL embedding queue:
  - `kb_embedding_jobs` is the task source of truth;
  - job claiming uses `FOR UPDATE SKIP LOCKED` on PostgreSQL;
  - statuses include `pending`, `processing`, `ready`, `retrying`, `failed`, `cancelled`, and `skipped`;
  - retry/failure metadata is persisted with attempts, max attempts, backoff, worker lock, and last error;
- snapshot publish integration:
  - `kb_search_documents` remain the lexical/index source;
  - snapshot indexing enqueues embedding jobs asynchronously when an embedding provider is configured;
  - publish does not call model inference synchronously;
- embedding providers:
  - `disabled` for default safe startup;
  - `deterministic` for ordinary Go tests and local contract coverage;
  - `local_http` for the local Python model worker;
- local Python embedding model worker:
  - path: `services/embedding-worker`;
  - model: `BAAI/bge-m3`;
  - dimensions: `1024`;
  - endpoints: `GET /health`, `GET /metadata`, `POST /embed`;
- Go embedding job worker:
  - command: `go run ./cmd/worker`;
  - claims durable jobs, calls the provider, writes pgvector embeddings, and handles retry/backoff;
- search behavior:
  - `mode=semantic` uses query embedding + pgvector cosine distance when available;
  - `mode=hybrid` merges lexical and semantic results;
  - semantic unavailability is explicit and hybrid falls back to lexical rather than faking semantic results;
  - responses can include embedding coverage and unavailable reason;
- operational endpoints:
  - `GET /api/v1/kb/collections/:id/snapshots/:snapshot_id/embedding-status`;
  - `POST /api/v1/kb/collections/:id/snapshots/:snapshot_id/embedding-jobs/retry-failed`;
- smoke helper:
  - `scripts/smoke-kb-embedding-worker.sh` validates the local Python worker metadata and sample embedding shape.

New M3.5 migration:

```text
migrations/016_kb_semantic_search_pgvector.sql
```

Latest local verification:

```text
go test ./...: 122 passed in 11 packages
```

## Current Known Limitations

Full M3 / M3.5 intentionally still does **not** include:

- plugin marketplace / SAGE service routes;
- multi-server federation or remote server authentication;
- a full client-side merge engine;
- full conversation history reconstruction solely from sync events;
- production observability stack;
- generic external sync write APIs;
- chunk-level passage embeddings;
- BGE-M3 sparse / multi-vector retrieval;
- OpenAI/Cohere/Voyage or other paid/closed embedding providers as defaults.

Semantic search is no longer faked: it is available only when a configured embedding provider and ready pgvector rows exist. Hybrid search degrades to lexical results and reports semantic unavailable reason / coverage when semantic indexing is not ready.

## Recommended Next Milestone

Recommended next milestone is to live-smoke M3.5 with Docker Compose (`embedding-worker` + `embedding-job-worker`) and then proceed to **M4 Plugin Marketplace + SAGE**. If search quality becomes the priority before M4, the next search slice should add chunk-level passage embeddings and retrieval-quality evaluation rather than changing the provider architecture.
