# Contract Versioning And Compatibility

This document defines the compatibility rules for externally consumed Nantian Gateway contracts. It is the shared policy for release review, code review and future schema changes.

The goal is not to freeze every internal field. The goal is to make each public contract explicit enough that operators, automation clients and mixed-version data planes can reason about upgrades before they deploy them.

## Contract Classes

| Contract | Stable anchor | Compatibility policy | Drift gate |
| --- | --- | --- | --- |
| Gateway API support declaration | `GatewayClass.status.supportedFeatures`, [Gateway API support matrix](../gateway-api-support.md) | Feature declarations are explicit and must not be treated as production validation. Additions require implementation and test evidence; removals require release notes. | `scripts/update-gateway-api-support.sh --check`, conformance reports |
| xDS / proto snapshot | `proto/gateway/control/v1/control.proto` | `gateway.control.v1` is additive-compatible across adjacent releases. Breaking wire changes require a new proto package or a documented migration. | `scripts/run-skew-validation.sh`, `make proto`, unit tests |
| Controlplane admin API | `/livez`, `/readyz`, `/v1/*` | `stable-v1`, `additive-compatible`: stable method/path/auth/content type; JSON response fields may be added, but existing stable fields keep type and meaning. | [admin-api-surface.json](admin-api-surface.json), `controlplane/internal/admin/route_contract_test.go` |
| Dataplane admin API | `/livez`, `/readyz`, `/metrics`, `/v1/*` | `stable-v1`, `additive-compatible`: stable method/path/auth/content type; metrics names and summary schema versions require explicit review. | [admin-api-surface.json](admin-api-surface.json), `dataplane/crates/aeg-app/src/admin/tests/contract.rs` |
| Prometheus metrics and Grafana golden signals | `/metrics`, `deploy/observability/grafana/*`, `deploy/observability/prometheus/*` | `stable-v1`, `additive-compatible`: metric additions must keep existing names, units, types and label meanings stable, and must document label cardinality before default exposure. | [metrics-cardinality.md](metrics-cardinality.md), `scripts/check-metrics-cardinality-contract.sh` |

## Version Labels

`stable-v1` means the surface is suitable for operators and repo-owned clients to consume within the documented boundaries. It does not mean the payload has a complete OpenAPI schema yet.

`additive-compatible` means these changes are allowed without a breaking version:

- Adding optional JSON fields.
- Adding optional protobuf fields with safe zero-value behavior.
- Adding new enum values only when old consumers can safely treat unknown or unrecognized values as `UNSPECIFIED`, `UNKNOWN`, `unsupported`, or an equivalent fallback.
- Adding new endpoints when existing endpoints are unchanged and the surface inventory is updated.
- Adding new Prometheus metrics when existing metric names, label meanings and units remain stable.
- Adding new metric labels only when [metrics-cardinality.md](metrics-cardinality.md) documents the cardinality class and default scrape impact.

These changes are breaking unless they are explicitly handled through a new version, migration or release-note compatibility exception:

- Removing or renaming an endpoint, metric, protobuf message, protobuf field, enum value or stable JSON field.
- Reusing a protobuf field number.
- Changing a protobuf field wire type, JSON field type, metric type or metric unit.
- Changing auth requirements in a way that weakens production security defaults or breaks existing configured-token clients without a migration.
- Changing path parameters, pagination semantics, filter names or error object fields in a non-additive way.

## xDS / Proto Snapshot Rules

`gateway.control.v1` is the release-to-release wire contract between the Go control plane and Rust data plane.

All changes to `proto/gateway/control/v1/control.proto` must follow this checklist:

- State whether the change is additive or breaking in the PR / commit context.
- For every new field, document the safe default when an older control plane does not send it.
- For every new field, document how an older data plane behaves when it receives and ignores it.
- For every new enum value, define the fallback behavior for unknown values.
- Do not delete fields, reuse field numbers or change wire types inside `gateway.control.v1`.
- If a data plane cannot safely apply a snapshot, it must `NACK` and continue serving last-good configuration when possible.
- If a control plane receives `NACK`, it must preserve node-level error detail for admin API and summary visibility.
- Run `make proto` after proto edits and include generated code in the same commit.
- Run `./scripts/run-skew-validation.sh` for proto or mixed-version changes.

The adjacent-release skew policy remains documented in [skew-compatibility.md](../skew-compatibility.md). This document defines the field-level review checklist; the skew document defines the supported runtime combinations.

## Admin API Rules

The normative route inventory is [admin-api-surface.json](admin-api-surface.json). Each surface must declare:

- `name`
- `displayName`
- `basePath`
- `defaultAuth`
- `stability`
- `versionPolicy`
- `endpoints`

For `stable-v1` admin surfaces, the following are stable unless a breaking change is explicitly planned:

- HTTP method.
- Path template.
- Auth mode.
- Primary content type.
- Query parameter names and basic pagination semantics.
- JSON error object fields that are already documented for that surface.

For controlplane JSON errors, the compatible baseline is:

```json
{
  "code": "machine_readable_code",
  "error": "human readable message"
}
```

For dataplane JSON errors, the compatible baseline is the `ApiError` response shape used by the current Axum handlers. The route contract fixes the endpoint surface; field-level dataplane error schema should be promoted to a JSON schema before it is treated as fully frozen.

Pagination and filtering rules:

- Existing `limit` / `offset` pagination semantics are additive-compatible only when default behavior remains unchanged.
- New filters must be optional.
- Invalid filter values must keep returning a 4xx JSON error rather than silently broadening the query.
- List responses may add fields, but existing item identity fields must keep their type and meaning.

## Gateway API Support Declaration Rules

Gateway API support uses four separate levels:

- `declared`
- `implemented`
- `tested`
- `production-validated`

Do not use `supported` as a standalone release claim. The detailed policy and current matrix are in [gateway-api-support.md](../gateway-api-support.md).

When a Gateway API feature is added to `GatewayClass.status.supportedFeatures`, the change must identify:

- The controlplane translation and status path.
- The dataplane runtime path or a clear explanation that the feature is controlplane-only.
- The conformance, e2e, unit or smoke evidence.
- Any production-validation gap that remains open.

## Release Review Checklist

Before a release or externally referenced compatibility claim, verify the relevant subset:

- `scripts/update-gateway-api-support.sh --check` for Gateway API declaration drift.
- `./scripts/run-skew-validation.sh` for proto / mixed-version contract changes.
- `cd controlplane && go test ./internal/admin` for controlplane admin surface drift.
- `cargo test --manifest-path dataplane/Cargo.toml -p aeg-app` for dataplane admin surface drift.
- `./scripts/run-targeted-validation.sh` when the changed files span multiple areas and the cheapest validation path is not obvious.

If a change intentionally breaks compatibility, the same commit or release PR must include:

- The reason the additive-compatible path is insufficient.
- The migration path.
- The rollback behavior.
- The release note entry.
- The documentation update for this file and any affected contract file.
