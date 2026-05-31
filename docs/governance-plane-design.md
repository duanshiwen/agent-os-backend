# AgentOS Governance Plane Design

Updated: 2026-05-31
Status: Stage 4A foundation

## 1. Purpose

The governance plane is the shared policy, approval, kill switch, scan, and audit-adjacent control plane for high-impact AgentOS backend actions.

It is designed to be reused by:

- SAGE Plugin Open Platform;
- KB Hub governance and billing operations;
- Object Storage operations;
- admin operations;
- future Agent/tool execution surfaces.

Federation / multi-server networking is explicitly out of current scope.

## 2. Current Stage 4A Foundation

Stage 4A introduces six durable concepts:

1. `capability_definitions` — stable capability taxonomy entries.
2. `policy_rules` — deterministic rules that map actor/subject/capability/risk context to a decision.
3. `policy_decisions` — persisted evaluation result and reason.
4. `approval_receipts` — one-time, expiring receipts for approval-gated decisions.
5. `kill_switches` — emergency deny controls for plugin/user/capability/server-local/global scopes.
6. `governance_scan_results` — persistent scanner findings for manifest/tool/policy review.

## 3. Policy Decision Contract

Allowed decision values:

- `allow`
- `require_user_approval`
- `require_admin_approval`
- `deny`

The first implementation is intentionally simple and deterministic:

1. validate subject type and capability key;
2. check active, unexpired kill switches;
3. find active matching policy rules ordered by `priority ASC, created_at ASC`;
4. persist the decision;
5. default to `allow` only when no matching active rule or kill switch exists.

A kill switch always overrides an allow rule.

## 4. Subject Types

Initial subject types:

- `sage_plugin`
- `sage_permission_grant`
- `sage_invocation`
- `object_operation`
- `kb_operation`
- `admin_operation`

## 5. Kill Switch Scopes

Initial kill switch scopes:

- `global`
- `plugin`
- `user`
- `capability`
- `server`

`server` is server-local only. It is not a Federation primitive.

## 6. Approval Receipts

Approval receipts are opaque-token based. The backend persists only `token_hash`, never the raw token.

Rules:

- receipt starts as `pending`;
- receipt must expire;
- consume is one-time;
- consume after expiry fails;
- token reuse fails;
- `consumed_by` records the consuming device or actor marker.

## 7. Admin APIs

The first admin API slice exposes:

- `POST /api/v1/admin/governance/capabilities`
- `GET /api/v1/admin/governance/capabilities`
- `POST /api/v1/admin/governance/policy-rules`
- `GET /api/v1/admin/governance/policy-rules`
- `POST /api/v1/admin/governance/kill-switches`
- `POST /api/v1/admin/governance/evaluate`

These routes are mounted under the existing admin route group and therefore inherit JWT authentication and admin authorization.

## 8. Current Non-Goals

- No policy DSL.
- No remote/federated policy evaluation.
- No backend execution of third-party SAGE Flow definitions.
- No LLM-based scanner in the first foundation.
- No external payment provider enforcement in this slice.

## 9. Verification

Focused tests:

```bash
go test ./internal/service -run Governance -count=1
```

Full backend tests:

```bash
go test ./...
```
