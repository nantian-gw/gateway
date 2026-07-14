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

## Package Naming Conventions

This codebase follows the [Go package naming conventions](https://go.dev/blog/package-names): lowercase, single-word names with no underscores or mixedCaps. The following packages use non-standard multi-word names that should be migrated.

### Audit Results (2026-07-14)

| # | Package | Path | Importers | Pkg Files | Severity | Proposed | Rationale |
|---|---------|------|-----------|-----------|----------|----------|-----------|
| 1 | `gwapi` | `internal/gwapi` | 43 | 13 | **CRITICAL** | → `gatewayapi` | "gw" is a project-specific abbreviation, not a standard Go abbreviation (cf. `http`, `json`, `tls`). AGENTS.md docs already refer to it as `gatewayapi` (line 39). |
| 2 | `gwexp` | `internal/gwexp` | 55† | 10‡ | **HIGH** | → `gatewayexp` | Same "gw" abbreviation issue. Cascading rename affects all sub-packages (`aiservice`, `backendlb`, `routepolicy`, `tokenpolicy`, `wasmplugin`). |
| 3 | `backendlb` | `internal/gwexp/backendlb` | 46 | 2 | **LOW** | → `backend` | "backend"+"lb" concatenation is hard to parse. Already aliased everywhere (`backendlb ".../backendlb"`). Renaming to `backend` gives idiomatic `backend.BackendLBPolicy{}`. Deferred until after `gwexp`→`gatewayexp` rename. |
| 4 | `lbpolicy` | `internal/lbpolicy` | 3 | 6 | **LOW** | → `loadbalancing` | "lb" is a standard abbreviation but "policy" suffix makes a 4-letter compound. Renaming to `loadbalancing` describes what the package does (evaluates load balancing policies). |
| 5 | `tlspolicy` | `internal/tlspolicy` | 2 | 6 | **LOW** | → `backendtls` | "tls" is a standard Go abbreviation (`crypto/tls`). Renaming to `backendtls` mirrors the CRD name (BackendTLSPolicy) and distinguishes from frontend TLS. |
| 6 | `nodeinfo` | `internal/nodeinfo` | 15 | 4 | **LOW** | → `noderegistry` | "node"+"info" concatenation is valid Go but vague. `noderegistry` better describes the package's function (node registration/status registry). |

**†** `gwexp` has no standalone package — all 55 imports go to its sub-packages. Renaming the parent directory touches all 55 import paths.

**‡** Package files across the 5 sub-packages under `gwexp/`.

### Import Reference Details

#### 1. `gwapi` (43 importers across 11 packages)

| Calling Package | Files |
|-----------------|-------|
| `internal/translator` | 13 |
| `internal/status` | 14 |
| `internal/controller` | 5 |
| `internal/admin` | 1 |
| `internal/infrastructure` | 2 |
| `cmd/gateway-api-support` | 1 |
| `conformance` | 1 |
| `internal/gwapi` (self) | 13 |

#### 2. `gwexp` (55 importers across 9 packages)

| Sub-package | Importers |
|-------------|-----------|
| `gwexp/backendlb` | 46 |
| `gwexp/aiservice` | 3 |
| `gwexp/tokenpolicy` | 2 |
| `gwexp/routepolicy` | 2 |
| `gwexp/wasmplugin` | 2 |

All 55 imports would need path updates when `gwexp`→`gatewayexp`.

#### 3. `backendlb` (46 importers across 10 packages)

| Calling Package | Files |
|-----------------|-------|
| `internal/translator` | 12 |
| `internal/status` | 11 |
| `internal/controller` | 9 |
| `internal/admin` | 6 (+2 chatbot subdir) |
| `internal/lbpolicy` | 4 |
| `cmd/manager` | 3 |
| `internal/infrastructure` | 1 |

Import alias `backendlb` is used in **all 46 files**. If renamed to `backend`, the alias must become `backend` and all `backendlb.X` references updated.

#### 4-6. Low-Impact Packages

| Package | Callers | Files |
|---------|---------|-------|
| `lbpolicy` | `internal/translator` (2), `internal/status` (1) | 3 |
| `tlspolicy` | `internal/translator` (1), `internal/status` (1) | 2 |
| `nodeinfo` | `internal/xds` (5), `internal/admin` (6), `internal/infrastructure` (3), `cmd/manager` (1) | 15 |

### Migration Plan

#### Phase 1: `gwapi` → `gatewayapi` (CRITICAL — docs/import mismatch)

1. Rename directory `internal/gwapi` → `internal/gatewayapi`
2. Update the `package gwapi` declaration to `package gatewayapi` in all 13 source files
3. Update all 43 import paths: `"github.com/nantian-gw/gateway/internal/gwapi"` → `"github.com/nantian-gw/gateway/internal/gatewayapi"`
4. Update all `gwapi.X` → `gatewayapi.X` references
5. Update any import aliases from `gwapi` to `gatewayapi`

**Affected: 56 files.** Build check: `go build ./...` and `go test ./...`.

#### Phase 2: `gwexp` → `gatewayexp` (HIGH — cascading rename)

1. Rename directory `internal/gwexp` → `internal/gatewayexp`
2. Update all 55 import paths — all sub-package paths change:
   - `internal/gwexp/backendlb` → `internal/gatewayexp/backendlb`
   - `internal/gwexp/aiservice` → `internal/gatewayexp/aiservice`
   - `internal/gwexp/tokenpolicy` → `internal/gatewayexp/tokenpolicy`
   - `internal/gwexp/routepolicy` → `internal/gatewayexp/routepolicy`
   - `internal/gwexp/wasmplugin` → `internal/gatewayexp/wasmplugin`
3. Update AGENTS.md "Experimental Packages" section header and all references
4. Update the Version Stability Plan paths

**Affected: 65 files.** Build check: `go build ./...` and `go test ./...`.

#### Phase 3: `backendlb` → `backend` (LOW — after gwexp rename)

1. Rename directory `internal/gatewayexp/backendlb` → `internal/gatewayexp/backend`
2. Update `package backendlb` → `package backend` in `types.go` and `types_test.go`
3. Update all 46 import paths and aliases:
   - `backendlb ".../gatewayexp/backendlb"` → `backend ".../gatewayexp/backend"`
4. Replace all `backendlb.X` → `backend.X` references (e.g., `backendlb.BackendLBPolicy` → `backend.BackendLBPolicy`)

**Affected: 48 files.** Build check: `go build ./...` and `go test ./...`.

#### Phases 4-6: Low-Impact Packages (can be done independently)

| Phase | Rename | Files | Order |
|-------|--------|-------|-------|
| 4 | `lbpolicy` → `loadbalancing` | 9 | Any time |
| 5 | `tlspolicy` → `backendtls` | 8 | Any time |
| 6 | `nodeinfo` → `noderegistry` | 19 | Any time |

**Total migration scope: ~205 files across all 6 renames.**

### Execution Order (Recommended)

```
Phase 1 (gwapi) → Phase 2 (gwexp) → Phase 3 (backendlb)
Phases 4-6 can run in parallel with any phase.
```

Phase 1 and 2 are independent of each other (gwapi and gwexp have no mutual imports). Phase 3 depends on Phase 2 (backendlb lives under gwexp). Phases 4-6 have no dependencies on Phases 1-3.

### Verification

After each phase:
```bash
go build ./...
go test -count=1 -timeout 5m ./...
golangci-lint run ./...
```

## Acceptance

Every change needs a spec, plan, and strict acceptance criteria. Record exact verification commands and results before marking work complete.

For documentation-only changes in this repository, run at least:

- `go test ./internal/translator` when touching translator documentation.
- `make test` unless the plan explicitly scopes a smaller command and records why.
- A local README link/path check when rewriting `README.md`.
- `git diff --check origin/main...HEAD`.

For behavior changes, add focused tests first and then run all affected package checks.

---

## Pending Improvements & Audit Findings

Last audited: 2026-07-14

### Item 1: Control Plane Memory Optimization (P2)

**Current State:**
- **pprof enabled**: Separate HTTP server at configurable `pprof.addr` (default `127.0.0.1:6060`). Exposes `/debug/pprof/` endpoints with optional bearer-token auth. Defined in `cmd/manager/pprof.go`, wired in `cmd/manager/app.go` as a managed component (graceful serve/shutdown).
- **No custom memory metrics**: No `nantian_gw_controlplane_mem_*` or Go `runtime.MemStats` metrics are registered. Only admin API request metrics exist (`AdminAPIRequestsTotal`, `AdminAPIRequestDurationSeconds`).
- **In-memory indexing structures**:
  - `internal/admin/detail_index.go`: `snapshotDetailIndex` holds hash maps for listeners (by name), backends (by namespace+name), and routes (by kind+namespace+name). Rebuilt from scratch on every snapshot version change.
  - `internal/admin/list_cache.go`: TTL-based response cache (default 1s) for resource lists and service catalogs to avoid repeated Kubernetes API calls.
  - `internal/infrastructure/route_indexes.go`: Kubernetes field indexes (GatewayClass by controllerName, Gateway by gatewayClassName, Route by service parents) used by the infrastructure reconciler. These are server-side indexes on the API server, not in-process.
  - `internal/ir/types.go`: Full `Snapshot` materialized in memory with all Listeners, HTTPRoutes, GRPCRoutes, StreamRoutes, Backends, Secrets, Workloads as in-process slices.
  - `internal/infrastructure/inspector.go`: Infrastructure report uses maps (`serviceIndex`, `sliceIndex`) for observed vs. expected comparison during inspections.
- **Memory threats identified**:
  - `snapshotDetailIndex` rebuilds all three lookup maps (listeners, backends, routes) per snapshot — old snapshots are GC'd but intermediate copies exist during rebuild.
  - List endpoints (`/v1/listeners`, `/v1/routes`, `/v1/backends`) return full unfiltered slice copies with no pagination enforcement at the snapshot layer.
  - No buffer pool or `sync.Pool` usage for the frequent slice allocations in `filterListeners`/`filterRoutes`/`filterBackends`.
  - `Snapshot.Clone()` (in `internal/ir/clone.go`) performs deep copies for xDS distribution but doesn't use arena allocators.

**Recommendations:**
1. **Add Go runtime memory metrics**: Expose `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`, `go_memstats_gc_cpu_fraction` as Prometheus metrics to establish baseline and detect regressions.
2. **Add snapshot memory metrics**: Track `snapshot_size_bytes` (approximate JSON serialized size) and `detail_index_build_duration_seconds`.
3. **Profile snapshot lifecycle**: Use existing pprof endpoints to capture heap profiles during steady state and full-rebuild cycles.
4. **Consider `sync.Pool` for slice buffers** in admin filter functions that allocate new slices per request.
5. **Evaluate arena allocator** for `Snapshot` clone paths (requires Go 1.22+ `arena` package).

### Item 2: API Aggregation — Control Plane Aggregated Endpoint (P1)

**Current State:**
- `/v1/summary` exists as an aggregated overview: returns `Summary` struct with counts (listeners, routes, backends, secrets), node status distribution, listener health, and snapshot sync state. It computes route/backend/listener counts from the snapshot but doesn't return object details.
- `/v1/dashboard/capabilities` returns feature-flag toggles (AI overview, services, etc.).
- **Individual list endpoints** exist for each resource type:
  - `GET /v1/listeners` — returns full listener list with optional filter (name, protocol, hostname, attachedRoute, sort, pagination).
  - `GET /v1/listeners/{name}` — single listener detail.
  - `GET /v1/routes` — returns HTTP+GRPC+Stream routes with optional kind filter, sort, pagination.
  - `GET /v1/routes/{kind}/{namespace}/{name}` — single route detail.
  - `GET /v1/backends` — returns backends with pagination.
  - `GET /v1/backends/{namespace}/{name}` — single backend detail.
  - `GET /v1/nodes` — returns node list with pagination.
  - `GET /v1/nodes/{nodeId}` — single node detail.
  - `GET /v1/infrastructure` — infrastructure report with filtering (state, role, kind, namespace, name, sort, pagination).
  - `GET /v1/service-catalog` — service catalog with filtering.
- **No `?include=` parameter exists** on any endpoint. Each resource type requires a separate API call.
- **No composite/aggregated endpoint** that returns multiple resource types in a single response.

**What's Needed:**
1. **`GET /v1/gateways`** — list all managed gateways with optional `?include=routes,listeners,backends,summary` parameter.
2. **`include=summary` mode**: Return the existing `/v1/summary` data embedded in each gateway resource (gateway-level summary, not cluster-wide).
3. **`include=routes` mode**: Embed route lists filtered by gateway parentRef.
4. **`include=listeners` mode**: Embed listener config with status.
5. **`include=backends` mode**: Embed backend references used by this gateway's routes.

**Design considerations:**
- Gateway-to-route parentRef mapping already exists in translator (routes reference gateways via parentRefs). Need a reverse index for filtering.
- Per-gateway summary would be a new subset of the existing `Summary` struct scoped to one gateway.
- Response size could be large — needs pagination and gzip support.
- Consider `include=counts` as a lightweight option (just counts, not full objects).

### Item 3: Multi-Word Package Rename Audit (P1) — COMPLETE

**Status: Audit complete.** Full audit findings and migration plan are documented above in the "Package Naming Conventions" section (lines 122-253).

Summary: 6 packages identified for rename across ~205 files in 6 phases:
- Phase 1: `gwapi` → `gatewayapi` (CRITICAL, 56 files)
- Phase 2: `gwexp` → `gatewayexp` (HIGH, 65 files)
- Phase 3: `backendlb` → `backend` (LOW, 48 files)
- Phase 4-6: `lbpolicy`, `tlspolicy`, `nodeinfo` (LOW, 36 files total)

Phases 1 and 2 are independent; Phase 3 depends on Phase 2. Phases 4-6 have no dependencies.

**Next action**: Select and execute Phase 1 (`gwapi` → `gatewayapi`) as the highest-impact rename.

---

### Verification

```bash
go build ./...  # PASSES (no errors)
```
