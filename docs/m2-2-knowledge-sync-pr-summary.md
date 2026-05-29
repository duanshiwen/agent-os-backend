# M2.2 Knowledge Sync PR Summary

Branch: `feat/knowledge-sync-m2-2`
Base: `main`
Date: 2026-05-30

## Purpose

Implement personal knowledge entry cross-device sync before starting KB Hub, marketplace, billing, or plugin work.

This PR is intentionally scoped to one user's own knowledge entries and uses Go backend services only. Rust FFI remains limited to identity verification.

## User-visible API Additions

### Baseline

- `GET /api/v1/knowledge/entries`
- `GET /api/v1/knowledge/entries?include_deleted=true`
- `GET /api/v1/knowledge/entries/*entry_id`
- `GET /api/v1/knowledge/entries/*entry_id?include_deleted=true`

### Mutations

- `POST /api/v1/knowledge/entries`
- `PUT /api/v1/knowledge/entries/*entry_id`
- `DELETE /api/v1/knowledge/entries/*entry_id`

`entry_id` may contain slash-separated local paths such as `notes/alpha`.

## Sync Events

- `knowledge.created`
- `knowledge.updated`
- `knowledge.deleted`

All events use:

- `object_type = knowledge`
- `object_id = entry_id`
- `operation = created | updated | deleted`

Payloads are full snapshots for simple client reducers.

## Semantics

Implemented:

- stable `entry_id` object identity;
- tombstone deletion instead of physical deletion;
- active baseline hides tombstones by default;
- `include_deleted=true` exposes tombstones for reconciliation;
- deleted entries cannot be updated through normal `PUT`;
- server-side `version` increment per successful mutation;
- SHA-256 `content_hash`;
- optional `client_event_id` idempotency for create/update/delete;
- reused `client_event_id` conflict detection;
- knowledge mutation and sync event record in one DB transaction;
- websocket notification after commit;
- optional `base_version` optimistic concurrency for update/delete;
- stale `base_version` returns `409 conflict`, does not mutate entry, and does not record a sync event.

## Main Files Changed

- `docs/sync-contract.md`
- `docs/phase1-status.md`
- `internal/model/models.go`
- `internal/repository/knowledge_entries_repo.go`
- `internal/service/knowledge_entries.go`
- `internal/handler/knowledge_entries.go`
- `internal/router/router.go`
- `internal/service/knowledge_entries_test.go`
- `internal/handler/phase1_router_smoke_test.go`
- `migrations/011_user_knowledge_entries.sql`
- `scripts/smoke-knowledge-sync.sh`

## Commit Summary

```text
754a411 test(sync): cover knowledge stale version conflicts
3e67886 feat(sync): guard knowledge writes by base version
59640d8 refactor(sync): make knowledge mutations atomic
b6df9f0 test(sync): harden knowledge sync idempotency
1ce0973 feat(sync): add knowledge entry sync
```

## Diff Stat vs main

```text
docs/phase1-status.md                         |   8 +-
docs/sync-contract.md                         | 110 +++++++++
internal/handler/knowledge_entries.go         | 119 ++++++++++
internal/handler/phase1_router_smoke_test.go  | 133 ++++++++++-
internal/model/models.go                      |  19 +-
internal/repository/knowledge_entries_repo.go |  95 ++++++++
internal/repository/sync_repo.go              |   4 +
internal/router/router.go                     |   9 +
internal/service/knowledge_entries.go         | 319 ++++++++++++++++++++++++++
internal/service/knowledge_entries_test.go    | 313 +++++++++++++++++++++++++
internal/service/sync.go                      |  20 +-
migrations/011_user_knowledge_entries.sql     |  28 +++
scripts/smoke-knowledge-sync.sh               |  15 ++
13 files changed, 1182 insertions(+), 10 deletions(-)
```

## Verification

Run:

```bash
./scripts/smoke-knowledge-sync.sh
go test ./...
```

Latest result:

```text
M2.2 knowledge sync smoke passed.
Go test: 107 passed in 10 packages
```

## Pending Manual Verification

Postgres migration smoke is pending because Docker Desktop was not running locally during this session.

Suggested once Docker is available:

```bash
docker compose up -d postgres
# apply migrations to a temporary database or start the app with AUTO_MIGRATE=true against a dev DB
./scripts/smoke-knowledge-sync.sh
go test ./...
```

## Out of Scope

- KB Hub publishing
- KB snapshot and subscription semantics
- marketplace discovery
- billing
- semantic indexing / embeddings
- Rust knowledge FFI
- generic sync write endpoint
