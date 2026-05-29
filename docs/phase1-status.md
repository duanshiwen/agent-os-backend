# AgentOS Backend Phase 1 / M2.1 Status

Updated: 2026-05-29
Branch: `feat/skill-settings-sync`

## Summary

Phase 1 is the deployability-hardened AgentOS backend foundation. It includes identity, admission, QR-only device pairing, conversation, offline message, WebSocket, and the stable sync foundation.

M2.1 Sync Object Coverage is now implemented for the low-risk configuration objects that sit on top of that foundation:

- profile
- message
- skill settings
- agent settings
- server list

This means a device can mutate these object families through domain-specific APIs, and another device for the same user can catch up through `/api/v1/sync/events`.

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

## Verification Commands

Run the M2.1 sync settings smoke suite:

```bash
./scripts/smoke-sync-settings.sh
```

Run all Go tests:

```bash
go test ./...
```

Latest verified result:

```text
94 passed in 10 packages
```

Run Rust FFI integration test:

```bash
./scripts/test-ffi-integration.sh
```

Latest verified result:

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
10. server list mutation and cross-device sync pull.

The service-level smoke and focused sync tests are in:

```text
internal/service/phase1_smoke_test.go
internal/service/skill_settings_test.go
internal/service/agent_settings_test.go
internal/service/server_connections_test.go
internal/service/sync_contract_test.go
```

Use this script for focused M2.1 verification:

```bash
./scripts/smoke-sync-settings.sh
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

Phase 1 / M2.1 intentionally does **not** include:

- knowledge entry sync semantics;
- KB Hub service routes;
- plugin marketplace / SAGE service routes;
- billing business logic;
- multi-server federation or remote server authentication;
- a full client-side merge engine;
- full conversation history reconstruction solely from sync events;
- production observability stack;
- generic external sync write APIs.

Model structs for KB, plugin, and billing already exist, but they should be treated as future-phase placeholders until the corresponding service, repository, handler, migration, and sync semantics are designed.

## Recommended Next Milestone

Proceed from M2.1 Sync Object Coverage to M2.2 Knowledge Sync Semantics implementation. The first implementation slice adds personal knowledge entry baseline APIs, tombstone-safe mutations, and `knowledge.created` / `knowledge.updated` / `knowledge.deleted` sync events.

Suggested next tasks:

1. implement local knowledge object identity and tombstone semantics;
2. implement baseline APIs and incremental sync events for knowledge entries;
3. decide how KB sync relates to future KB Hub subscription state;
4. keep knowledge sync aligned with the documented contract;
5. keep KB Hub service implementation blocked until sync contract is stable enough for knowledge and subscription state.
