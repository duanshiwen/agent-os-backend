# AgentOS Backend Phase 1 / M2.2 Status

Updated: 2026-05-30
Branch: `feat/knowledge-sync-m2-2`

## Summary

Phase 1 is the deployability-hardened AgentOS backend foundation. It includes identity, admission, QR-only device pairing, conversation, offline message, WebSocket, and the stable sync foundation.

M2.1 Sync Object Coverage is implemented for the low-risk configuration objects that sit on top of that foundation:

- profile
- message
- skill settings
- agent settings
- server list

M2.2 Knowledge Sync Semantics is now implemented for a user's personal knowledge entries. This is intentionally scoped to cross-device sync of the user's own local knowledge objects. It does not implement KB Hub publishing, marketplace discovery, subscription state, billing, semantic indexing, or Rust knowledge FFI.

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

Run all Go tests:

```bash
go test ./...
```

Latest verified result:

```text
M2.2 knowledge sync smoke passed.
Go test: 107 passed in 10 packages
```

Run Rust FFI integration test:

```bash
./scripts/test-ffi-integration.sh
```

Latest previously verified result:

```text
=== RUN   TestFFIVerifierIntegration
--- PASS: TestFFIVerifierIntegration (0.00s)
PASS
ok  github.com/agent-os/backend/internal/service
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
13. knowledge stale `base_version` conflicts returning `409` without sync events.

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

## Current Known Limitations

Phase 1 / M2.2 intentionally does **not** include:

- KB Hub service routes;
- KB publishing / snapshot / subscription semantics;
- semantic indexing, embeddings, or content storage pipeline;
- Rust FFI knowledge operations beyond identity verification;
- plugin marketplace / SAGE service routes;
- billing business logic;
- multi-server federation or remote server authentication;
- a full client-side merge engine;
- full conversation history reconstruction solely from sync events;
- production observability stack;
- generic external sync write APIs.

Model structs for KB, plugin, and billing already exist, but they should be treated as future-phase placeholders until the corresponding service, repository, handler, migration, and sync semantics are designed.

Local Postgres migration smoke is still pending on this machine because Docker Desktop was not running during verification.

## Recommended Next Milestone

M2.2 personal knowledge sync is ready for review. Recommended next steps:

1. run Postgres migration smoke once Docker Desktop is available;
2. review and merge `feat/knowledge-sync-m2-2`;
3. update SDK/client code to consume the knowledge baseline APIs and sync events;
4. keep KB Hub service implementation blocked until personal knowledge sync is integrated by clients;
5. after client integration feedback, design the separate KB Hub contract for publishing, snapshots, subscriptions, and marketplace behavior.
