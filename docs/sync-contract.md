# AgentOS Sync Contract

Updated: 2026-05-29

## 1. Scope

This document defines the AgentOS backend cross-device sync contract. The sync layer is an event-sourced catch-up mechanism for devices under the same user identity.

M2 covers the stable contract for:

- sync event envelope;
- event taxonomy;
- per-user sequence semantics;
- pull API;
- ack API;
- WebSocket notification envelope;
- client event idempotency;
- conversation incremental sync;
- profile sync;
- compatibility and error semantics.

M2 does not implement KB Hub, Plugin Marketplace, Billing, multi-server federation, or a full merge engine.

## 2. Event Envelope

Each persisted sync event has these contract fields:

| Field | Meaning |
|---|---|
| `id` | server-generated event UUID |
| `user_id` | owner of the sync stream |
| `device_id` | legacy/source device field; mirrors source device for now |
| `event_type` | stable shorthand: `<object_type>.<operation>` |
| `schema_version` | event schema version, currently `1` |
| `object_type` | sync object family, e.g. `message`, `profile` |
| `object_id` | object identifier inside its family |
| `operation` | state transition, e.g. `created`, `updated` |
| `source_device_id` | device that caused the event |
| `client_event_id` | optional client-generated idempotency key |
| `payload` | self-contained JSON payload for the event |
| `timestamp` | server event timestamp |
| `sequence` | per-user monotonic sequence |
| `created_at` / `updated_at` | persistence timestamps |

## 3. Event Taxonomy

Supported object types:

- `message`
- `knowledge`
- `skill`
- `agent`
- `server`
- `plugin`
- `profile`

Supported operations:

- `created`
- `updated`
- `deleted`
- `added`
- `removed`
- `enabled`
- `disabled`

Allowed combinations in M2:

| Object type | Operations |
|---|---|
| `message` | `created`, `updated`, `deleted` |
| `knowledge` | `created`, `updated`, `deleted` |
| `skill` | `enabled`, `disabled`, `updated` |
| `agent` | `updated` |
| `server` | `added`, `updated`, `removed` |
| `plugin` | `added`, `updated`, `removed` |
| `profile` | `updated` |

`event_type` is derived from `object_type + "." + operation`, for example `profile.updated`.

## 4. Sequence Semantics

Sequences are monotonic per user.

- First event for a user starts at sequence `1`.
- Each new event increments that user's sequence.
- Different users have independent sequence spaces.
- Idempotent replay with the same `client_event_id` must return the existing event and must not allocate a new sequence.

## 5. Pull API

Endpoint:

```http
GET /api/v1/sync/events?after_sequence=0&limit=100
```

If `after_sequence` is provided, events are returned after that explicit sequence without reading or mutating the device cursor.

If `after_sequence` is omitted, the backend reads the authenticated device's durable cursor and returns events after `sync_cursors.last_synced_sequence`.

M2 response envelope:

```json
{
  "events": [],
  "next_after_sequence": 123,
  "has_more": false,
  "server_time": 1780054321000,
  "schema_version": 1
}
```

`limit` defaults to `100` when `<= 0` or `> 500`.

## 6. Ack API

Endpoint:

```http
POST /api/v1/sync/ack
Content-Type: application/json

{"last_sequence": 123}
```

Ack semantics:

- Ack advances the authenticated device's durable sync cursor.
- Ack is monotonic. A lower `last_sequence` than the current cursor is a no-op and still succeeds.
- Clients should ack only after they have durably applied events.

## 7. WebSocket Notification

When a sync event is recorded, the server notifies the user's other connected devices with:

```json
{
  "type": "sync.event",
  "payload": {
    "event_type": "profile.updated",
    "schema_version": 1,
    "object_type": "profile",
    "object_id": "...",
    "operation": "updated",
    "source_device_id": "device-a",
    "client_event_id": "optional",
    "sequence": 12,
    "timestamp": 1780054321000,
    "payload": {}
  }
}
```

The source device is excluded from this notification path.

## 8. Client Event Idempotency

`client_event_id` is optional but recommended for client-originated writes.

For the same user:

| Case | Behavior |
|---|---|
| same `client_event_id` + same object / operation / source | return existing event, do not allocate new sequence |
| same `client_event_id` + different object / operation / source | reject as conflict |
| empty `client_event_id` | allow write, no idempotency guarantee |

The database enforces uniqueness for non-empty `(user_id, client_event_id)` through a partial unique index.

## 9. Conversation Sync

M2 uses a baseline + incremental model.

### Baseline

Clients load conversation list and message history through existing REST APIs:

- `GET /api/v1/conversations`
- `GET /api/v1/conversations/:id/messages`

### Incremental changes

New messages are represented as `message.created` sync events.

Minimum payload:

```json
{
  "object_id": "message uuid",
  "message_id": "message uuid",
  "conversation_id": "conversation uuid",
  "sender_id": "user uuid",
  "type": "text",
  "content": "...",
  "metadata": {},
  "created_at": "..."
}
```

### Real-time path

Connected devices may receive immediate WebSocket message delivery and/or `sync.event` notification.

### Catch-up path

After reconnect, clients call `/sync/events?after_sequence=<last_seen>` or cursor-based `/sync/events`.

## 10. Profile Sync

`profile.updated` is the first non-message sync event.

Minimum payload:

```json
{
  "object_id": "user uuid",
  "user_id": "user uuid",
  "display_name": "Alice",
  "avatar_url": "https://...",
  "updated_at": "..."
}
```

This event is emitted by `PUT /api/v1/users/me`.

## 11. Skill Settings Sync

Skill settings use a baseline + incremental model.

### Baseline

Clients load the current user's skill settings through:

- `GET /api/v1/skills/settings`

### Incremental changes

Skill setting writes emit these events:

| Endpoint | Event |
|---|---|
| `POST /api/v1/skills/settings/:skill_id/enable` | `skill.enabled` |
| `POST /api/v1/skills/settings/:skill_id/disable` | `skill.disabled` |
| `PUT /api/v1/skills/settings/:skill_id` | `skill.updated` |

Mutating requests accept optional `client_event_id` for idempotency.

Minimum payload:

```json
{
  "object_id": "skill id",
  "skill_id": "skill id",
  "enabled": true,
  "config": {},
  "updated_by_device_id": "device-a",
  "updated_at": "..."
}
```

The source device is recorded in `source_device_id`; other devices can pull the event through `/sync/events` and receive real-time `sync.event` notification when connected.

## 12. Error Semantics

| Condition | HTTP status / behavior |
|---|---:|
| malformed query / JSON | 400 |
| unsupported event type | 400 when externally exposed |
| unsupported operation | 400 when externally exposed |
| duplicate `client_event_id` conflict | 409 when externally exposed |
| ack sequence lower than current cursor | 200 no-op |
| unauthorized / invalid token | 401 |
| authenticated but not allowed | 403 |
| database / internal failure | 500 |

## 13. Compatibility Rules

- `schema_version` must increase for breaking payload changes.
- Existing fields should remain additive whenever possible.
- New object types and operations must be added to the central sync contract and covered by tests.
- Clients should ignore unknown payload fields.
- Clients should not assume global sequence ordering across users; sequences are per-user.
