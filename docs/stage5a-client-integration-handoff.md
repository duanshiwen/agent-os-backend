# Stage 5A Client Integration Handoff

Updated: 2026-06-01  
Status: client-ready contract handoff baseline

This handoff summarizes what AgentOS Client / SDK can rely on after the Stage 5A contract gates. The backend remains a single-server control plane. Rust remains integrated through narrow FFI contracts only; no Rust backend sidecar is introduced.

## 1. Stable Contract Surfaces

### 1.1 Sync Consumer Contract

Document: [client-sync-consumer-contract.md](./client-sync-consumer-contract.md)

Client-facing contract:

- `/sync/events` style pull envelope with cursor progression;
- profile, message, skill, agent, server, plugin, plugin permission, and knowledge event families;
- SDK projection compatibility through `ClientReadySyncProjection`;
- FFI reducer entrypoint: `agentos_apply_sync_pull_response_json`.

Evidence:

```bash
./scripts/smoke-stage5a-sync-bridge.sh
```

Primary backend fixture:

```text
internal/service/testdata/stage5a_client_ready_sync_pull_response.json
```

### 1.2 FFI Artifact Contract

Bundled runtime artifacts:

```text
internal/runtime/darwin-arm64/libagentos_ffi.dylib
internal/runtime/linux-arm64/libagentos_ffi.so
```

Required exported symbols:

- `agentos_ffi_version`
- `agentos_ffi_free_string`
- `agentos_identity_verify_ed25519_challenge`
- `agentos_apply_knowledge_sync_events_json`
- `agentos_apply_knowledge_sync_pull_response_json`
- `agentos_apply_sync_pull_response_json`

Current FFI version expected by backend tests:

```text
0.1.0
```

Evidence:

```bash
./scripts/check-ffi-artifacts.sh
```

### 1.3 SAGE Client Runtime Contract

Document: [sage-plugin-runtime-contract.md](./sage-plugin-runtime-contract.md)

Client-facing contract:

- fetch policy bundle for an installation;
- read approved manifest snapshot, granted permissions, denied permissions, runtime guards, and reporting endpoint;
- call third-party Plugin Server flow endpoint from the approved manifest snapshot;
- create invocation records with idempotent `client_request_id`;
- submit execution reports with idempotent `client_report_id`;
- preserve flow metadata, step summaries, tokens, and metering evidence;
- surface governance outcomes during permission grant and invocation creation.

Evidence:

```bash
./scripts/check-sage-client-runtime-contract.sh
```

Optional live stack smoke:

```bash
RUN_LIVE_SMOKES=1 ./scripts/release-gate-local.sh
```

### 1.4 Governance Client Error Contract

Document: [governance-client-error-contract.md](./governance-client-error-contract.md)

Stable HTTP error codes:

| HTTP | Code | Meaning |
|---:|---|---|
| 403 | `governance_denied` | Operation blocked by active policy. |
| 428 | `governance_approval_required` | Operation requires explicit approval before retry. |
| 403 | `governance_approval_invalid` | Supplied approval token is invalid, mismatched, expired, revoked, or consumed. |

Stable details fields include:

- `policy_decision_id`
- `subject_type`
- `subject_id`
- `capability_key`
- `risk_level`
- `decision`
- `reason`
- `approval_receipt_id` for approval-required responses
- `approval_expires_at` for approval-required responses

Evidence:

```bash
./scripts/check-governance-client-error-contract.sh
```

## 2. Release Evidence Commands

Default local release gate:

```bash
./scripts/release-gate-local.sh
```

Stage 5A evidence report:

```bash
./scripts/stage5a-release-evidence.sh
```

By default, the evidence script writes:

```text
tmp/stage5a-release-evidence.md
```

To include targeted SDK tests:

```bash
RUN_SDK_TESTS=1 ./scripts/stage5a-release-evidence.sh
```

To include environment-backed live gates, use the local release gate with a running stack:

```bash
RUN_MIGRATION_GATE=1 RUN_LIVE_SMOKES=1 RUN_LOCAL_HTTP_SEMANTIC=1 ./scripts/release-gate-local.sh
```

## 3. Client Integration Order

Recommended order for AgentOS Client integration:

1. Load / verify bundled FFI artifact version and required symbols.
2. Consume sync pull responses through `agentos_apply_sync_pull_response_json`.
3. Build local client projection for profile, message, skill, agent, server, plugin, plugin permission, and knowledge families.
4. Implement SAGE installation policy bundle fetch.
5. Execute third-party SAGE flow client-side using approved manifest snapshot and runtime guards.
6. Create SAGE invocation records with stable `client_request_id`.
7. Submit execution reports with stable `client_report_id`.
8. Implement governance UX for denied, approval-required, and invalid-approval responses.
9. Use Stage 5A release evidence as the regression baseline for backend/client compatibility.

## 4. Non-goals for This Handoff

This handoff does not require:

- Federation / multi-server networking;
- backend execution of third-party SAGE Flow steps;
- Rust sidecar deployment;
- real external payment provider integration;
- production governance operations beyond the stable client error contract.

## 5. Open Follow-ups After Client Contract Handoff

- KB Hub search quality improvements: chunk-level search, passage embeddings, hybrid reranking, retrieval evaluation fixtures.
- Governance 4E production hardening: richer scanner coverage, policy conditions, response inspection, incident workflow, approval operator authorization.
- Live-stack release evidence with PostgreSQL migrations, object storage, SAGE runtime smoke, governance enforcement smoke, and local HTTP semantic search.
