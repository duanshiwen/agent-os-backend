# SAGE Plugin Runtime Contract v1

Updated: 2026-06-01  
Status: Stage 5A client runtime contract baseline

This contract is between AgentOS Client and third-party SAGE Plugin Servers. AgentOS Backend is the control plane: it stores plugin manifests, permissions, policy bundles, invocation records, execution reports, audit evidence, usage ledger records, and governance decisions. The backend does **not** execute third-party SAGE Flow definitions.

## 1. Control-Plane Boundaries

AgentOS Backend owns:

- plugin registry and review state;
- manifest validation and approved manifest snapshots;
- plugin installation and permission grants;
- policy bundle generation;
- governance enforcement for grant and invocation operations;
- invocation and execution report ingestion;
- developer metrics and usage ledger foundation.

AgentOS Client owns:

- fetching the policy bundle;
- calling the third-party Plugin Server flow endpoint;
- enforcing runtime guards and user confirmations locally;
- creating backend invocation records before or during runtime execution;
- submitting execution reports after execution;
- presenting governance denied / approval-required / invalid-approval errors to the user.

Third-party Plugin Server owns:

- its manifest endpoint;
- flow generation endpoint;
- callback / health endpoints;
- any external API orchestration it is authorized to perform.

## 2. Policy Bundle API

Endpoint:

```http
GET /api/v1/sage/installations/{installation_id}/policy-bundle
Authorization: Bearer <token>
```

Stable response body under `data`:

```json
{
  "plugin_key": "com.example.hotel-booking",
  "version": "1.0.0",
  "policy_bundle_version": "2026-06-01.1",
  "manifest_snapshot": {},
  "granted_permissions": [
    {"key": "plugin.api.call", "risk": "low", "scope": {}}
  ],
  "denied_permissions": ["transaction.booking.create"],
  "runtime_guards": [
    {
      "id": "guard-transaction-booking-create",
      "match": {"permission": "transaction.booking.create"},
      "decision": "require_user_confirmation"
    }
  ],
  "reporting": {
    "required": true,
    "endpoint": "/api/v1/sage/invocations/{id}/reports"
  }
}
```

Client rules:

1. Treat `manifest_snapshot` as the approved manifest source for runtime execution.
2. Treat `granted_permissions` as currently active grants.
3. Treat `denied_permissions` as unavailable even if the manifest requests them.
4. Enforce `runtime_guards` before executing guarded steps.
5. Submit reports to `reporting.endpoint` when `reporting.required=true`.
6. Refetch policy bundle after plugin sync events or before sensitive runtime operations.

## 3. Flow Request

AgentOS Client calls the Plugin Server flow endpoint from the approved manifest snapshot.

Example request to Plugin Server:

```json
{
  "request_id": "req_123",
  "user_intent": "帮我找东京酒店",
  "locale": "zh-CN",
  "timezone": "Asia/Shanghai",
  "user_constraints": {
    "destination": "Tokyo"
  },
  "available_agent_capabilities": [
    "llm.generate",
    "user.ask",
    "user.confirm",
    "http.call"
  ],
  "granted_permissions": ["plugin.api.call"],
  "privacy_context": {
    "allow_profile_sharing": false,
    "allow_precise_location": false
  },
  "policy_bundle_version": "2026-06-01.1"
}
```

## 4. Flow Response

Example response from Plugin Server:

```json
{
  "flow_id": "flow_123",
  "plugin_key": "com.example.hotel-booking",
  "title": "Search Tokyo Hotels",
  "risk_level": "medium",
  "requires_permissions": ["plugin.api.call"],
  "steps": [
    {
      "id": "search_hotels",
      "type": "plugin_api",
      "method": "POST",
      "path": "/api/hotels/search",
      "input": {"destination": "Tokyo"},
      "output_key": "hotel_candidates"
    },
    {
      "id": "present_options",
      "type": "present_to_user",
      "input": {"items": "{{steps.search_hotels.output}}"}
    }
  ],
  "reporting": {
    "callback_required": true
  }
}
```

## 5. Step Types v1

Supported client runtime step type vocabulary:

- `plugin_api`
- `llm_generate`
- `ask_user`
- `present_to_user`
- `user_confirm`
- `callback_plugin`
- `stop`

Backend stores reports about execution but does not interpret or execute these steps in Stage 5A.

## 6. Invocation API

Endpoint:

```http
POST /api/v1/sage/invocations
Authorization: Bearer <token>
Content-Type: application/json
```

Request:

```json
{
  "plugin_key": "com.example.hotel-booking",
  "installation_id": "installation-uuid",
  "client_request_id": "client-stable-request-id",
  "user_intent": "find hotels in Hangzhou",
  "flow_id": "flow_123",
  "flow_hash": "sha256-or-client-flow-hash",
  "risk_level": "low",
  "permissions_used": ["plugin.api.call"],
  "policy_decision": {"decision": "allow"}
}
```

Client rules:

1. `client_request_id` is required and idempotent per user + plugin.
2. Retrying the same logical invocation with the same `client_request_id` returns the existing invocation.
3. `permissions_used[0]` is used as the governance capability key when present; otherwise backend uses `sage.invocation.create`.
4. `policy_decision` may include client-side runtime policy evidence and is augmented by backend governance evidence when governance is configured.

## 7. Execution Report API

Endpoint:

```http
POST /api/v1/sage/invocations/{invocation_id}/reports
Authorization: Bearer <token>
Content-Type: application/json
```

Request:

```json
{
  "client_report_id": "client-stable-report-id",
  "status": "completed",
  "flow_id": "flow_123",
  "steps_completed": ["search_hotels", "present_options"],
  "step_summaries": {
    "search_hotels": "mock candidates returned",
    "present_options": "options presented"
  },
  "tokens_used": 42,
  "metering": {"unit": "tokens"},
  "errors": [],
  "user_confirmations": [],
  "plugin_callbacks": []
}
```

Client rules:

1. `client_report_id` is required and idempotent per invocation.
2. Retrying the same logical report with the same `client_report_id` returns the existing report.
3. `status=completed` marks the invocation completed.
4. `status=failed` marks the invocation failed.
5. Reports feed usage ledger and developer metrics.

## 8. Governance Error Contract

SAGE permission grants and invocation creation can be governed in enforce mode.

Stable error codes:

| HTTP | Code | Meaning |
|---:|---|---|
| 403 | `governance_denied` | Operation is denied by policy. |
| 428 | `governance_approval_required` | Operation requires approval before retry. |
| 403 | `governance_approval_invalid` | Supplied approval token is invalid, mismatched, expired, revoked, or already consumed. |

Stable error body:

```json
{
  "error": {
    "code": "governance_approval_required",
    "message": "governance approval required",
    "details": {
      "policy_decision_id": "decision-uuid",
      "subject_type": "sage_permission_grant",
      "subject_id": "installation-uuid",
      "capability_key": "third_party.api.call",
      "risk_level": "medium",
      "decision": "require_user_approval",
      "reason": "matched policy rule",
      "approval_receipt_id": "receipt-uuid",
      "approval_expires_at": "2026-06-01T08:00:00Z"
    }
  }
}
```

Client rules:

1. For `governance_denied`, stop execution and show the stable policy reason.
2. For `governance_approval_required`, drive the approval UX, then retry with `approval_token` when available.
3. For `governance_approval_invalid`, discard the token and request a new approval path.
4. Do not reuse consumed approval tokens.

## 9. Stage 5A Evidence Path

Fast service-level gate:

```bash
./scripts/check-sage-client-runtime-contract.sh
```

It runs:

```bash
go test ./internal/service -run 'TestSAGEPluginService(ClientRuntimeContract|GovernanceEnforceClientRuntimeOutcomes)$' -count=1 -v
```

This verifies:

- policy bundle identity fields;
- granted / denied permission split;
- runtime guard emission;
- stable reporting endpoint;
- invocation idempotency;
- execution report idempotency;
- developer metrics update;
- governance denied outcome;
- governance approval-required outcome with receipt/token;
- governance invalid-approval outcome;
- approval-token retry success path.

Live control-plane smoke remains available when a local stack is running:

```bash
RUN_LIVE_SMOKES=1 ./scripts/release-gate-local.sh
```

The live smoke covers object upload, plugin asset binding, developer submit, admin review, user install/grant, sync events, policy bundle fetch, mock Plugin Server flow call, invocation/report, uninstall event, and developer metrics.
