# Gateway Repository Guide

## Repository Role

This repository contains the Go control plane for Nantian Gateway. It watches Kubernetes Gateway API resources, translates them into internal routing state, serves admin APIs, and publishes runtime snapshots to data planes over gRPC/xDS.

Do not use this repository for Rust data plane, Dashboard, Helm chart, Website, or Proto source-of-truth changes. Those live in sibling repositories.

## Git Workflow

The workspace root is not a Git repository. This component directory is its own Git repository.

Make changes in an isolated worktree under `~/.config/superpowers/worktrees/`, not directly in the checked-out `gateway/` main checkout. Do not merge a worktree branch back to `main` until the user explicitly approves. Do not push `main` until the user explicitly asks after merge approval.

Root `docs/` files are workspace notes and do not need to be committed with gateway changes unless the user explicitly asks for archival handling.

## Commands

Run commands from the gateway repository root.

- `make build` builds all Go packages.
- `make test` runs `go test -count=1 -timeout 5m ./...`.
- `go test ./internal/translator` runs focused translator tests.
- `go test ./internal/controller` runs focused controller tests.
- `go test ./internal/admin` runs focused admin API tests.
- `make e2e-smoke` runs the Kind smoke test.
- `make conformance` creates a Kind cluster and runs Gateway API conformance tests.
- No local protobuf generation target is currently defined here; protobuf source and generation workflow live in the sibling Proto repository.

Use focused `go test ./path` checks while iterating, then run the broader relevant target before committing.

## Project Map

- `cmd/manager/` starts the controller manager and wires runtime services.
- `internal/controller/` watches Kubernetes resources and coordinates full or partial rebuilds.
- `internal/translator/` converts Gateway API resources, policies, services, workloads, and extension objects into internal IR snapshots.
- `internal/grpcserver/` publishes snapshots and status over gRPC/xDS to data planes.
- `internal/admin/` serves operational, topology, metrics, and management APIs for the Dashboard and operators.
- `internal/gatewayapi/` contains Gateway API helper logic, validation, encoding, and supported feature declarations.
- `internal/ir/` defines the internal routing and runtime model shared by translator and gRPC publication code.
- `deploy/` contains Kubernetes manifests and overlays.
- `gen/` contains generated protobuf code.

## Generated Code

Do not edit generated files under `gen/` by hand. Change the source `.proto` definitions in the sibling Proto repository and bring generated output into this repository only when the source change and generation command are clear.

## Translator Maintenance

The translator package is the highest-risk package in this repository. When changing it:

- Preserve Gateway API semantics for parent refs, route attachment, listener validity, backend refs, filters, and status conditions.
- Preserve ReferenceGrant and namespace scoping rules for cross-namespace references.
- Preserve BackendTLSPolicy, BackendLBPolicy, session persistence, AIService, TokenPolicy, and WasmPlugin precedence rules.
- Prefer shared indexes and support-object loaders over ad hoc list scans.
- Keep full rebuild and partial rebuild behavior aligned.
- Add or update focused tests for route semantics, backend policy precedence, ReferenceGrant behavior, status summaries, partial rebuild paths, and IR shape changes.

## Documentation And Comments

Use English by default for documentation and code comments. Add localized text only when editing existing localized user-facing content.

## Naming Conventions

### Getters

This codebase follows the Go convention that getter methods do not use a `Get` prefix:

```go
// Correct — no Get prefix
func (c *Config) SyncPeriodDuration() time.Duration { ... }
func (s *SnapshotStore) Current() *Snapshot           { ... }
func (t *Translator) ControllerName() string          { ... }

// Avoid — Get prefix is non-idiomatic in Go
func (c *Config) GetSyncPeriodDuration() time.Duration { ... }
```

The `Get` prefix is reserved for methods that perform non-trivial work beyond returning a field value (e.g., I/O, computation, or error handling). When the method simply returns a stored or derived value, omit the prefix.

### Boolean Functions

Functions and methods returning `bool` should use an `Is`, `Has`, `Can`, `Should`, or `Does` prefix that makes the caller read as a natural English question:

```go
// Correct
func (c *Config) DashboardEnabled() bool     { ... }
func IsAuthConfigured(opts Options) bool     { ... }
func HasAnyManagedParent(...) bool           { ... }
func IsSameResourceKind(left, right string) bool  { ... }

// Avoid — returns bool but name doesn't indicate a boolean result
func authConfigured(opts Options) bool       { ... }
func anyManagedParent(...) bool              { ... }
func sameResourceKind(left, right string) bool    { ... }
```

## Experimental Packages (`internal/gwexp`)

The `internal/gwexp/` directory contains experimental Gateway API extension types that are not yet stable. Each package defines a CRD with an alpha API version:

| Package | CRD | API Group | API Version |
|---------|-----|-----------|-------------|
| `aiservice` | AIService | `gateway.nantian.dev` | `v1alpha1` |
| `backendlb` | BackendLBPolicy | `gateway.networking.k8s.io` | `v1alpha2` |
| `tokenpolicy` | TokenPolicy | `gateway.nantian.dev` | `v1alpha1` |
| `wasmplugin` | WasmPlugin | `gateway.nantian.dev` | `v1alpha1` |
| `routepolicy` | RoutePolicy | `gateway.nantian.dev` | `v1alpha1` |

The `v1alpha1` / `v1alpha2` suffix lives in the `GroupVersion.Version` string within each package — there are no version suffixes in Go package import paths. Package paths remain flat (e.g., `internal/gwexp/aiservice`).

### Version Stability Plan

When a package stabilizes to v1:

1. **If the API is unchanged**: update `GroupVersion.Version` to `"v1"`.
2. **If the API changed**: create a new canonical package under `internal/gwexp/<name>_v1/` with the `v1` GroupVersion, keep the alpha package for backward compatibility during a deprecation window, then remove the alpha package once all consumers have migrated.
3. Each package is promoted independently — no single flag-day for all five.

The `backendlb` package uses `v1alpha2` from `gateway.networking.k8s.io` (the upstream Gateway API group), following upstream stability. Promotion to `v1` depends on the upstream Gateway API specification.

## Acceptance

Every change needs a spec, plan, and strict acceptance criteria. Record exact verification commands and results before marking work complete.

For documentation-only changes in this repository, run at least:

- `go test ./internal/translator` when touching translator documentation.
- `make test` unless the plan explicitly scopes a smaller command and records why.
- A local README link/path check when rewriting `README.md`.
- `git diff --check origin/main...HEAD`.

For behavior changes, add focused tests first and then run all affected package checks.
