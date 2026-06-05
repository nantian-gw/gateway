# Gateway API Next-Version Upgrade Proposal

Status: proposed
Date: 2026-05-13
Owner: maintainers

## Summary

Nantian Gateway currently pins Gateway API `v1.5.1` across the Go control plane,
conformance harness, Kind CRD bundle, release scripts, and support matrix. This
proposal defines how to move to the next stable Gateway API release after
`v1.5.1` without silently expanding the public support surface or reusing stale
conformance evidence.

The upgrade is intentionally evidence-gated. A future version bump must align
code dependencies, CRDs, conformance, support docs, status semantics, and release
evidence in the same change set or in a tightly sequenced upgrade branch.

## Current Position

- The repository default Gateway API version is `v1.5.1`.
- The current declared support surface is defined by
  `controlplane/internal/gatewayapi/supported_features.go` and
  `docs/gateway-api-support.md`.
- `docs/backlog/gateway-api-experimental.md` already records that new
  experimental support must not be added by default.
- `docs/gateway-api-version-audit.md` defines the status-surface audit required
  when Gateway API versions change.

As of this proposal, the upgrade target is not a moving monthly snapshot. The
target must be a named upstream stable Gateway API release newer than `v1.5.1`,
unless maintainers explicitly approve a canary branch that is not used for
release support claims.

## Goals

1. Upgrade all repository version pins to the same stable Gateway API release.
2. Re-run and archive conformance evidence for the upgraded version.
3. Re-audit `GatewayClass.status.supportedFeatures` against upstream feature
   metadata.
4. Keep unsupported features unsupported unless they have their own design,
   implementation, tests, and support-matrix updates.
5. Preserve rollback to the current `v1.5.1` baseline if the upgrade exposes
   incompatible CRD, status, or conformance behavior.

## Non-Goals

- Do not implement HTTP/3 / QUIC as part of a version bump.
- Do not implement ListenerSet as part of a version bump.
- Do not expand TLSRoute terminate or mixed mode as part of a version bump.
- Do not declare full BackendLBPolicy support as part of a version bump.
- Do not treat an archived `v1.5.1` conformance report as evidence for the new
  Gateway API version.

## Upgrade Scope

The version bump must inspect and update these surfaces together:

| Surface | Required action |
| --- | --- |
| `controlplane/go.mod` and `go.sum` | Upgrade `sigs.k8s.io/gateway-api` and `sigs.k8s.io/gateway-api/conformance` together. |
| `tests/conformance-harness/go.mod` and `go.sum` | Match the same Gateway API module version. |
| `tests/e2e/run-kind.sh` and `tests/conformance/run.sh` | Update default CRD and conformance clone/cache version knobs. |
| `scripts/check-gateway-api-version-alignment.sh` | Ensure the checker recognizes the new canonical version. |
| `controlplane/internal/status/*` | Re-audit CRD bundle version checks and status conditions. |
| `controlplane/internal/gatewayapi/supported_features.go` | Reconcile upstream feature metadata with declared support. |
| `docs/gateway-api-support.md` | Regenerate or manually update declared / implemented / tested / production-validated boundaries. |
| `docs/status-matrix.md` | Update condition semantics if upstream changed them. |
| `docs/test/latest-baseline.md` and `reports/conformance/README.md` | Point to the new conformance evidence after it exists. |
| Release notes and compatibility docs | Record the Gateway API version change, known unsupported areas, and rollback path. |

## Implementation Plan

### Phase 1: Upstream Version Audit

Run a read-only audit before editing dependencies:

```bash
./scripts/check-gateway-api-version-alignment.sh
./scripts/update-gateway-api-support.sh --check
```

Review upstream release notes and compare:

- CRD schema changes
- conformance test additions, removals, and renamed features
- feature channel changes
- status condition and reason changes
- resource version promotions or deprecations

Record the audit result in `docs/gateway-api-version-audit.md` or a dated
appendix if the change is non-trivial.

### Phase 2: Dependency and CRD Alignment

Update every repository pin in one branch. The minimum local checks are:

```bash
go test ./...
./scripts/check-gateway-api-version-alignment.sh
./scripts/audit-gateway-api-bundle.sh
./scripts/update-gateway-api-support.sh --check
```

If generated CRD or support-matrix output changes, update the generated docs
through the repository scripts rather than editing generated sections by hand.

### Phase 3: Feature Declaration Review

Treat upstream features as candidates, not automatic declarations.

For every upstream feature that is new, renamed, promoted, or removed, record
one of these outcomes:

- `declared`: implementation and tests exist, and conformance or equivalent
  evidence is archived for the upgraded version.
- `implemented-not-declared`: repository behavior exists but upstream
  conformance or support metadata is absent or incomplete.
- `unsupported`: control plane rejects or ignores it with documented status
  behavior.
- `deferred`: requires a separate proposal, such as ListenerSet or HTTP/3.

Update `docs/backlog/gateway-api-experimental.md` if any experimental boundary
changes.

### Phase 4: Runtime and Status Fixes

Compile failures and status test failures are expected during a version bump.
Keep fixes scoped to compatibility and status semantics unless a separate
feature proposal is approved.

Minimum checks:

```bash
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

Run narrower targeted tests first while fixing, then the full commands before
publishing the upgrade branch.

### Phase 5: Kind and Conformance Evidence

After unit and contract checks pass, rebuild images and run Kind smoke:

```bash
./tests/e2e/run-kind.sh
```

Then run the upgraded conformance suite:

```bash
ALL_FEATURES=true ./tests/conformance/run.sh
```

Archive the report:

```bash
RESULT_STATUS=passed \
SOURCE_COMMAND='ALL_FEATURES=true ./tests/conformance/run.sh' \
./scripts/archive-conformance-report.sh \
  <YYYY-MM-DD-gateway-api-version-full-suite> \
  tmp/conformance/report-<version>.yaml \
  tmp/conformance/report-<version>.log
```

The new support claim is not complete until `reports/conformance/`,
`docs/gateway-api-support.md`, `docs/test/latest-baseline.md`, and
`docs/community-readiness.md` all point to evidence for the upgraded version.

## Test Matrix

| Layer | Command | Required before merge |
| --- | --- | --- |
| Version alignment | `./scripts/check-gateway-api-version-alignment.sh` | yes |
| Gateway API support docs | `./scripts/update-gateway-api-support.sh --check` | yes |
| Control plane | `cd controlplane && go test ./...` | yes |
| Data plane | `cargo test --manifest-path dataplane/Cargo.toml --workspace` | yes |
| Script inventory, if scripts changed | `./scripts/check-script-inventory.sh` | yes |
| Kind smoke | `./tests/e2e/run-kind.sh` | yes |
| Full conformance | `ALL_FEATURES=true ./tests/conformance/run.sh` | yes |
| Release evidence refresh | `./scripts/refresh-release-evidence.sh --check-only ...` | before release |

## Rollback Plan

The rollback is a normal revert of the version bump and all support-matrix
changes. If CRDs were applied to a live cluster during testing, rollback also
requires reinstalling the prior Gateway API `v1.5.1` CRD bundle and confirming:

```bash
EXPECTED_BUNDLE_VERSION=v1.5.1 ./scripts/audit-gateway-api-bundle.sh
./tests/e2e/run-kind.sh
```

Do not rollback only the Go modules while leaving newer CRDs, conformance
scripts, or support docs behind.

## Exit Criteria

This proposal is considered implemented for a future target version only when:

- all repository version pins match the same stable Gateway API release;
- version alignment and support-matrix checks pass;
- Kind smoke passes on images built from the upgraded branch;
- full conformance for the upgraded Gateway API version is archived;
- `docs/gateway-api-support.md` accurately separates declared, implemented,
  tested, and production-validated support;
- unsupported or deferred upstream features are explicitly listed; and
- release notes document upgrade impact and rollback.
