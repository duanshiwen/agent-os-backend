# Audit Hash Chain

Updated: 2026-05-31
Status: Stage 4A foundation

## Purpose

AgentOS audit events are now recorded with a tamper-evident hash chain. This does not make the database immutable, but it makes post-write mutation detectable by recomputing the canonical event hash and verifying each event's link to the previous event.

This is the audit substrate for the Stage 4A governance plane.

## Audit Event Hash Fields

`audit_events` now carries:

- `sequence` — monotonic audit chain sequence, starting at `1`;
- `previous_hash` — previous event hash, or `GENESIS` for the first event;
- `event_hash` — canonical SHA-256 hash for this event;
- `hash_algorithm` — currently `sha256`.

## Canonical Hash Payload

The event hash is computed over stable audit fields:

- `id`
- `sequence`
- `previous_hash`
- `actor_user_id`
- `actor_device_id`
- `action`
- `resource_type`
- `resource_id`
- `outcome`
- `ip_address`
- `user_agent`
- `metadata`
- `occurred_at`

`event_hash` itself is excluded from the payload.

## Verification

`AuditService.VerifyHashChain` checks:

1. sequence continuity;
2. previous-hash linkage;
3. non-empty hash fields;
4. supported hash algorithm;
5. recomputed event hash equality.

Break reasons:

- `sequence_gap`
- `link_mismatch`
- `missing_hash`
- `unsupported_hash_algorithm`
- `hash_mismatch`

## Current Scope

Implemented:

- hash-chain append in `AuditService.Record`;
- repository append transaction;
- full-chain verification service method;
- migration `026_audit_hash_chain.sql`;
- tests for valid chain and tamper detection.

Not yet implemented:

- admin HTTP endpoint for verification;
- periodic background verification job;
- external anchoring / notarization;
- partitioned per-tenant chains.

## Verification Commands

```bash
go test ./internal/service -run AuditService -count=1
go test ./...
```
