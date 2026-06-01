# Governance Client Error Contract v1

Updated: 2026-06-01  
Status: Stage 5A client-facing contract baseline

Stage 5A handoff: [stage5a-client-integration-handoff.md](./stage5a-client-integration-handoff.md)

This document defines the stable HTTP error shape that AgentOS Client can rely on when backend governance is in enforce mode. It applies to governance-protected operations such as SAGE permission grants, SAGE invocations, and object operations.

## 1. Error Envelope

Governance errors use the structured error envelope:

```json
{
  "error": {
    "code": "governance_denied",
    "message": "governance denied",
    "details": {
      "policy_decision_id": "decision-uuid",
      "subject_type": "sage_invocation",
      "subject_id": "plugin-or-installation-id",
      "capability_key": "plugin.api.call",
      "risk_level": "low",
      "decision": "deny",
      "reason": "matched policy rule"
    }
  }
}
```

The envelope intentionally differs from the generic `{code,message,data}` success envelope so clients can branch on `error.code` without parsing prose.

## 2. Stable Error Codes

| HTTP | `error.code` | Client meaning | Recommended client action |
|---:|---|---|---|
| 403 | `governance_denied` | The operation is blocked by active policy. | Stop the operation and present `details.reason`. |
| 428 | `governance_approval_required` | Policy requires explicit approval before retry. | Trigger approval UX, then retry with a fresh approval token. |
| 403 | `governance_approval_invalid` | Supplied approval token is invalid, mismatched, expired, revoked, or consumed. | Discard token and restart approval UX if still needed. |

## 3. Details Fields

`error.details` is stable for client UX and telemetry:

| Field | Type | Required when available | Meaning |
|---|---|---:|---|
| `policy_decision_id` | string | yes | Backend policy decision ID for audit and support. |
| `subject_type` | string | yes | Governed subject type, e.g. `sage_permission_grant`, `sage_invocation`, `object_operation`. |
| `subject_id` | string | yes | Governed subject ID, e.g. installation ID, plugin ID, or object scope. |
| `capability_key` | string | yes | Capability or permission key that triggered governance. |
| `risk_level` | string | yes | Governance risk level: `low`, `medium`, `high`, or `critical`. |
| `decision` | string | yes | Policy decision: `allow`, `deny`, or `require_user_approval`. |
| `reason` | string | yes | Human-readable policy reason. |
| `approval_receipt_id` | string | approval-required only | Receipt ID created by backend for the approval flow. |
| `approval_expires_at` | string | approval-required only | RFC3339 expiration timestamp for the receipt/token. |

The backend does **not** return the approval token inside error details. Token delivery remains part of the approval receipt flow.

## 4. Denied Example

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
```

```json
{
  "error": {
    "code": "governance_denied",
    "message": "governance denied",
    "details": {
      "policy_decision_id": "44444444-4444-4444-4444-444444444444",
      "subject_type": "object_operation",
      "subject_id": "scope-a",
      "capability_key": "object.upload.scope_a",
      "risk_level": "medium",
      "decision": "deny",
      "reason": "matched policy rule"
    }
  }
}
```

## 5. Approval Required Example

```http
HTTP/1.1 428 Precondition Required
Content-Type: application/json
```

```json
{
  "error": {
    "code": "governance_approval_required",
    "message": "governance approval required",
    "details": {
      "policy_decision_id": "44444444-4444-4444-4444-444444444444",
      "subject_type": "sage_permission_grant",
      "subject_id": "installation-1",
      "capability_key": "third_party.api.call",
      "risk_level": "medium",
      "decision": "require_user_approval",
      "reason": "requires human approval",
      "approval_receipt_id": "55555555-5555-5555-5555-555555555555",
      "approval_expires_at": "2026-06-01T08:30:00Z"
    }
  }
}
```

Client behavior:

1. Show the approval prompt with capability, risk, subject, and reason.
2. Route the user/operator through the approval receipt flow.
3. Retry the original operation with a fresh approval token.
4. Do not cache approval tokens beyond their specific operation context.

## 6. Invalid Approval Example

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
```

```json
{
  "error": {
    "code": "governance_approval_invalid",
    "message": "governance approval invalid",
    "details": {
      "policy_decision_id": "44444444-4444-4444-4444-444444444444",
      "subject_type": "sage_permission_grant",
      "subject_id": "installation-1",
      "capability_key": "third_party.api.call",
      "risk_level": "medium",
      "decision": "require_user_approval",
      "reason": "approval token invalid"
    }
  }
}
```

Client behavior:

1. Discard the token.
2. Do not retry in a tight loop.
3. Restart approval UX only if the user still wants to perform the operation.

## 7. Stage 5A Evidence Path

Fast HTTP-level contract gate:

```bash
./scripts/check-governance-client-error-contract.sh
```

It runs:

```bash
go test ./internal/handler -run 'TestGovernanceHandlerClientErrorContract$' -count=1 -v
```

The gate verifies:

- `403 governance_denied` body shape;
- `428 governance_approval_required` body shape;
- `403 governance_approval_invalid` body shape;
- stable details fields for decision, subject, capability, risk, and reason;
- approval receipt details only on approval-required responses.

The default local release gate runs this check after the SAGE client runtime contract gate.
