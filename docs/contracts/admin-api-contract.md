# Admin API Contract

This document defines the formal contract entry point for the Nantian Gateway administration surface. The current stable contract only covers the control plane and data plane admin APIs; the [`dashboard/`](../../dashboard/) directory in the repository is a consumer of these APIs, and the Node proxy is merely a same-origin access layer, not a new public aggregate API. The old frontend-specific aggregate API has been removed. Any future web console or SDK should establish dependencies based on this document and the machine-readable surface.

## Contract Files

- Machine-readable surface inventory: [admin-api-surface.json](admin-api-surface.json)
- External contract versioning strategy: [versioning.md](versioning.md)
- Operations and user-facing documentation: [admin-api.md](../user/admin-api.md)

`admin-api-surface.json` is the normative inventory, suitable for scripts, tests, SDKs, the dashboard, or future UIs to perform path, method, authentication, and content type handshaking first. `docs/user/admin-api.md` covers field semantics, filter parameters, examples, and troubleshooting context; it does not serve as a drift gate.

## Stable Rules

### 1. Authentication

- `/livez` and `/readyz` for both controlplane admin and dataplane admin are fixed to `none`, intended for health probes.
- `/v1/*` for controlplane is classified as `bearer-when-configured`.
- `/metrics` and `/v1/*` for dataplane are classified as `bearer-when-configured`.
- `bearer-when-configured` means that requests must explicitly carry an `Authorization: Bearer <token>` only after `adminAuth.bearerToken` or `adminAuth.bearerTokenFile` has been enabled.

### 2. Content-Type

- Probes return `text/plain`.
- Prometheus metrics return `text/plain; version=0.0.4; charset=utf-8`.
- All other admin interfaces return `application/json`.

### 3. Path Stability

- The stable path prefix for controlplane admin is `/v1/*`, along with `/livez` and `/readyz`.
- The stable path prefixes for dataplane admin are `/metrics` and `/v1/*`, along with `/livez` and `/readyz`.
- The repository currently does not provide an old frontend-specific admin surface contract; the dashboard proxy path is not part of the stable surface.

If these paths are added, removed, or renamed in the future, the following must be updated simultaneously:

- The route contract in the code.
- [admin-api-surface.json](admin-api-surface.json).
- Related tests.
- Update [admin-api.md](../user/admin-api.md) when necessary.

### 4. Explicit Version Handshake

`admin-api-surface.json` currently requires each surface to explicitly declare:

- `stability=stable-v1`
- `versionPolicy=additive-compatible`

See [versioning.md](versioning.md) for the meaning of these two fields. In short: paths, methods, authentication modes, and primary content types are the stable contract; JSON payloads may continue to have compatible fields appended, but existing stable field semantics must not be deleted or changed without a migration path.

The current dataplane summary already provides stronger version signals than a plain JSON document:

- Top-level `summarySurface=dataplane-summary`
- Top-level `summarySchemaVersion=1`
- `schemaVersion=1` inside multiple overview bundles

The controlplane summary currently does not have an independent schema document version number, so field-level evolution is still governed by [admin-api.md](../user/admin-api.md).

## Verification

The repository has incorporated this contract into automated verification:

- `controlplane/internal/admin/route_contract_test.go`
- `dataplane/crates/aeg-app/src/admin/tests/contract.rs`

Minimum verification commands:

```bash
cd controlplane && go test ./internal/admin
cargo test --manifest-path dataplane/Cargo.toml -p aeg-app
```

## Future Work

The current contract has elevated the documentation into a checkable surface contract, but it has not yet covered the full field-level JSON Schema / OpenAPI.

If more granular auto-generation is pursued in the future, the following should be prioritized:

1. Field-level schema for the controlplane summary / topology / resource admin surface.
2. Field-level JSON Schema for the dataplane summary overview bundle.
3. Machine-readable enumerations and value ranges for query parameters.
4. Complete OpenAPI documentation and corresponding generation / drift gate.
