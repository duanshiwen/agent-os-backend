# AgentOS Governance Plane Design

Updated: 2026-06-01
Status: Stage 4D approval workflow foundation

## 1. Purpose

The governance plane is the shared policy, approval, kill switch, scan, and audit-adjacent control plane for high-impact AgentOS backend actions.

It is designed to be reused by:

- SAGE Plugin Open Platform;
- KB Hub governance and billing operations;
- Object Storage operations;
- admin operations;
- future Agent/tool execution surfaces.

Federation / multi-server networking is explicitly out of current scope.

## 2. Current Stage 4D Foundation

Stage 4A introduced six durable concepts:

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
- `consumed_by` records the consuming device or actor marker;
- Stage 4C consume validates actor, subject type, subject id, and capability key when an approval token is used by `GovernanceEnforcer`;
- an approval token for one subject/capability cannot be reused for another SAGE, KB, or object operation;
- Stage 4D adds admin revoke semantics for pending receipts;
- Stage 4D adds admin token reissue for pending, unexpired receipts;
- token reissue rotates `token_hash`, invalidates the previous raw token, and returns the new raw token only once;
- revoked, consumed, and expired receipts cannot be reissued.

## 7. Admin APIs

The admin API slice exposes:

- `POST /api/v1/admin/governance/capabilities`
- `GET /api/v1/admin/governance/capabilities`
- `POST /api/v1/admin/governance/policy-rules`
- `GET /api/v1/admin/governance/policy-rules`
- `POST /api/v1/admin/governance/kill-switches`
- `POST /api/v1/admin/governance/evaluate`
- `GET /api/v1/admin/governance/summary?window=24`
- `GET /api/v1/admin/governance/policy-decisions`
- `GET /api/v1/admin/governance/policy-decisions/:id`
- `POST /api/v1/admin/governance/approval-receipts`
- `GET /api/v1/admin/governance/approval-receipts`
- `POST /api/v1/admin/governance/approval-receipts/:id/reissue-token`
- `POST /api/v1/admin/governance/approval-receipts/:id/revoke`
- `GET /api/v1/admin/governance/scan-results`
- `POST /api/v1/admin/governance/scan-results/:id/resolve`

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

## Stage 4B Governance Enforcement Integration

Stage 4B connects the governance plane to real backend execution surfaces through `GovernanceEnforcer`. The enforcer supports three modes:

- `disabled`: skip evaluation and allow execution;
- `observe`: evaluate and persist `policy_decisions`, but do not block business execution;
- `enforce`: block `deny`, require approval for `require_user_approval` / `require_admin_approval`, and consume one-time approval receipts.

The default backend configuration is `GOVERNANCE_ENFORCEMENT_MODE=observe`, so Stage 4B is safe to roll out while collecting would-block evidence.

| Surface | Subject Type | Capability Key Pattern | Enforcement Point |
|---|---|---|---|
| SAGE permission grant | `sage_permission_grant` | permission key, e.g. `sage.permission.payments.write` | `SAGEPluginService.GrantPermission` |
| SAGE invocation | `sage_invocation` | `sage.invocation.create` or first permission used | `SAGEPluginService.CreateInvocation` |
| Object upload | `object_operation` | `object.upload.<scope>` | `ObjectService.CreateUploadIntent` |
| Object download | `object_operation` | `object.download.<scope>` | `ObjectService.CreateDownloadURL` |
| Object delete | `object_operation` | `object.delete.<scope>` | `ObjectService.DeleteObject` |
| KB review/takedown | `kb_operation` | `kb.collection.review.<status>` | `KBHubService.ReviewCollection` |
| KB pricing | `kb_operation` | `kb.collection.pricing.update` | `KBHubService.UpdateCollectionPricing` |
| KB snapshot lifecycle | `kb_operation` | `kb.snapshot.archive` / `kb.snapshot.restore` | `KBHubService.ArchiveSnapshot` / `RestoreSnapshot` |

Domain-specific overrides are available via `GOVERNANCE_ENFORCEMENT_SAGE_MODE`, `GOVERNANCE_ENFORCEMENT_OBJECT_MODE`, and `GOVERNANCE_ENFORCEMENT_KB_MODE`.

## Stage 4C Enforce Readiness

Stage 4C makes observe-mode evidence operationally useful and closes the most important approval-token safety gap before broader enforce rollout.

Implemented readiness controls:

1. Approval receipt binding
   - `GovernanceEnforcer` consumes approval receipts through an operation-bound path.
   - The repository validates pending status, expiry, actor, subject type, subject id, and capability key in one transaction before marking the receipt consumed.

2. Stable governance error contract
   - Denied operations return machine-readable error codes:
     - `governance_denied`
     - `governance_approval_required`
     - `governance_approval_invalid`
   - SAGE, Object, and KB handlers route governance errors through the shared governance error helper.
   - Stage 4D wraps enforce-mode governance failures with decision/input context and returns stable `details` fields for policy decision, subject, capability, risk, reason, and approval receipt expiry metadata when available.

3. Admin evidence APIs
   - Admins can list and inspect policy decisions.
   - Admins can list approval receipts without exposing raw approval tokens.
   - Admins can list scanner findings and resolve findings.
   - Admins can query a governance summary window for decisions, approvals, and open findings.

4. Stage 4D approval workflow controls
   - Admins can revoke pending approval receipts.
   - Admins can reissue a token for a pending, unexpired approval receipt.
   - Reissue invalidates the old token by rotating the persisted token hash.
   - Revoked, consumed, and expired receipts cannot be reissued.

5. Release evidence
   - Unit/integration test baseline: `go test ./...` → 183 passed in 11 packages.
   - Local release gate: `./scripts/release-gate-local.sh` passes.
   - Live enforce-readiness smoke: `scripts/smoke-governance-enforce-readiness.sh` verifies enforce-mode denial, stable `governance_denied`, admin policy-decision visibility, summary evidence, approval token reissue/consume, old-token invalidation, and approval receipt revoke semantics.
