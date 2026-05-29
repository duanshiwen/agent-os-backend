# AgentOS Backend Phase 1 Status

Updated: 2026-05-29
Branch: `harden-phase1-deployability`

## Summary

Phase 1 is now a deployability-hardening milestone for the AgentOS backend foundation. The current backend has a working identity, admission, QR-only device pairing, conversation, offline message, WebSocket, and minimal sync foundation. Phase 1 should be treated as a stable base for Phase 2 sync contract work, not as a complete product surface.

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
- `GET /api/v1/sync/events`
- `POST /api/v1/sync/ack`
- Monotonic per-user sequence assignment.
- Cursor-based pull/ack.
- Real-time notification envelope via `sync.event`.
- `message.created` events are recorded for every conversation participant, including sender cross-device sync.

### Production Migrations

Current migrations:

- `001_init.sql` — users, devices, auth challenges, admission requests, conversations, messages, offline messages.
- `002_server_admission.sql` — persistent server admission policy.
- `003_device_pairing_sessions.sql` — QR-only device pairing sessions.
- `004_phase1_hardening.sql` — sync events/cursors and Phase 1 DB safeguards.

`004_phase1_hardening.sql` adds:

- `sync_events`
- `sync_cursors`
- `idx_sync_events_user_sequence`
- `idx_sync_events_user_device`
- `idx_sync_events_user_event_type`
- `idx_sync_events_timestamp`
- unique `devices(device_id)`
- unique `device_pairing_sessions(qr_payload_hash)`
- `auth_challenges(device_id, nonce)` index

## Verification Commands

Run all Go tests:

```bash
go test ./...
```

Latest verified result:

```text
59 passed in 10 packages
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

The router-level Phase 1 smoke test is:

```text
internal/handler/phase1_router_smoke_test.go
```

It verifies the real Gin router wiring for:

1. auth challenge;
2. auth verify;
3. private conversation creation;
4. message-created sync event pull;
5. sync event ack;
6. QR pairing start;
7. QR pairing claim.

The service-level Phase 1 smoke test is:

```text
internal/service/phase1_smoke_test.go
```

It verifies service-level auth, conversation, offline message, QR pairing, and sync behavior.

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

## Current Known Limitations

Phase 1 intentionally does **not** include:

- full Phase 2 cross-device sync contract;
- explicit sync event schema versioning;
- client event idempotency;
- `after_sequence` pull API;
- full conversation history sync reconstruction contract;
- KB Hub service routes;
- plugin marketplace / SAGE service routes;
- billing business logic;
- multi-server connection management;
- production observability stack.

Model structs for KB, plugin, and billing already exist, but they should be treated as future-phase placeholders until the corresponding service, repository, handler, and migration work is designed.

## Recommended Next Milestone

Proceed to M2: Sync Contract Foundation.

Suggested next tasks:

1. define a stable sync event envelope;
2. centralize sync event type constants;
3. add event validation before write;
4. add optional client event idempotency;
5. add explicit `after_sequence` pull support;
6. use `profile.updated` as the first non-message sync event.
