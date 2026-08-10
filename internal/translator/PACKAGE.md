# Package `translator` — Boundary Guide

## Overview

The `translator` package converts Kubernetes Gateway API resources and Nantian
extension resources into the internal routing model (IR — Intermediate
Representation) used by the control plane. The controller layer calls this
package after Kubernetes object changes.

A full rebuild loads Gateways, routes, Services, ServiceImports, EndpointSlices,
Secrets, ConfigMaps, ReferenceGrants, policies, workloads, and extension
resources. The translator builds shared lookup indexes, resolves listener
attachment, route rules, backend references, policy precedence, filters,
workload metadata, and extension behavior, and emits an IR snapshot plus status
summaries for the controller and gRPC publication layers.

### Package Size (2026-07-18 Split)

The package was split from a single monolithic package into 8 sub-packages
to improve maintainability, testability, and developer ergonomics. The root
package retains the `Translator` struct, full/partial build orchestration,
support object loaders, and workload translation.

| Sub-package | Pure LOC (non-test) | Pure LOC (test) | Primary responsibility |
|---|---|---|---|
| `shared/` | 409 | 242 | Indexes, helpers, limits, metrics, status summaries |
| `backends/` | 1240 | 1688 | Backend cluster, TLS, LB policy, session translation |
| `routes/` | 899 | 83 | Route and filter translation, timeout parsing |
| `listeners/` | 140 | 990 | Frontend validation, mesh service translation |
| `policies/` | 989 | 3476 | Route attachment, gateway queries, Wasm/Token translation |
| `routepolicy/` | 391 | 719 | RoutePolicy translation |
| `aiservice/` | 36 | 121 | AIService translation |
| `testutil/` | 335 | 0 | Shared test helpers (no test files) |
| **Root** | 4579 | 8081 | Translator struct, Build(), partial rebuilds, support loaders, workloads |

---

## Sub-package Responsibilities

### `shared/` — Leaf utility package

**Responsibility**: Common utilities, indexes, limits, metrics, timeouts, and
status helpers used by the translator package and all sub-packages.

**Key types**:
- `TranslatorIndexes` — Pre-built lookup maps for Services, ServiceImports,
  TLS Secrets, ConfigMap CA PEMs, EndpointSlices, and ReferenceGrants.
- `Options` / `Limits` — Configuration for resource limits and timeouts.
- `Limits.ValidateInputObjects()` / `Limits.ValidateSnapshot()` — Guard rails
  against resource exhaustion.

**Exports**: `BackendObjectKey`, `Hostnames`, `StringValue`,
`NamespaceOrDefault`, `PortValue`, `WeightValue`, `GatewayParents`,
`SelectSlicePort`, `HTTPRouteTimeouts`, `ParseGatewayDuration`,
`ListenerStatusSummary`, `RouteStatusSummary`, `ConvertConditions`,
`FindConditionSummary`, `RecordTranslationError`, `ObserveTranslationDuration`.

**Dependency rule**: Imports only `ir`, `prometheus`, and standard library.
Does NOT import any translator internals. This is a **leaf package** — safe to
use from any sub-package.

### `backends/` — Backend resource translation

**Responsibility**: Translates backend resources (Services, ServiceImports,
BackendTLSPolicies, BackendLBPolicies) into IR backend clusters with associated
TLS, load balancing, and session persistence configuration.

**Key types**:
- `BackendService` — Bundles a Kubernetes Service with its logical name and
  namespace for backend cluster construction.
- `BackendRefTranslator` — Resolves backend references on routes, checking
  ReferenceGrant scoping, service existence, and cross-namespace permissions.
- `BackendLBPolicyIndexes` — Pre-computed maps of session persistence, load
  balancing, and circuit breaker configs keyed by backend cluster key.
- `RouteKind` — Enum for route kind in backend ref validation.
- `HTTPFilterFunc` / `GRPCFilterFunc` — Function types for resolving route
  filters to IR filters.

**Exports**: `EffectiveBackendServices`, `TranslateBackends`,
`TranslateBackendsWithIndexes`, `TranslateEffectiveBackends`,
`TranslateServiceImportBackends`, `TranslateSecrets`,
`NewBackendRefTranslator`, `AnnotateHTTPRoute`, `AnnotateGRPCRoute`,
`AnnotateTCPRoute`, `AnnotateUDPRoute`, `AnnotateTLSRoute`,
`BackendRefMetadata`, `BackendKindForRef`, `ReferenceGranted`,
`RouteUsesOnlyServiceParents`, `IsServiceParentRef`, `ObjectNamePtr`,
`RuleSessionPersistence`, `BackendSessionPersistence`,
`DefaultRouteSessionName`, `BackendLoadBalancing`,
`BackendTLSForGatewayWithIndexes`, `RefGroup`, `RefKind`,
`BackendTLSValidationIndexWithIndexes`, `BuildBackendLBPolicyIndexesWithIndexes`.

**Imports**: `shared`, `ir`, `mesh`, `extfilter`, `gatewayapi`, `backendtls`,
`loadbalancing`, `backend` (gatewayexp).

### `routes/` — Route and filter translation

**Responsibility**: Translates Gateway API route resources (HTTPRoute, GRPCRoute,
TCPRoute, UDPRoute, TLSRoute) into IR objects, including filter resolution and
timeout parsing.

**Key types**:
- `RawHTTPRouteFilterConfigs` — Raw filter configs loaded from unstructured
  Kubernetes objects for filter types (e.g., CORS) that need access to the
  full object shape.

**Exports**: `TranslateHTTPRoute`, `TranslateHTTPRouteWithResolver`,
`TranslateHTTPRouteWithDefaultGateways`, `TranslateGRPCRoute`,
`TranslateGRPCRouteWithResolver`, `TranslateGRPCRouteWithDefaultGateways`,
`TranslateTCPRoute`, `TranslateTCPRouteWithDefaultGateways`,
`TranslateUDPRoute`, `TranslateUDPRouteWithDefaultGateways`,
`TranslateTLSRoute`, `TranslateTLSRouteWithDefaultGateways`,
`FiltersFromHTTP`, `FiltersFromHTTPWithResolver`, `FiltersFromGRPC`,
`FiltersFromGRPCWithResolver`, `BackendRefsFromHTTP`, `BackendRefsFromGRPC`,
`BackendRefsFromRouteRule`, `HTTPRouteRetry`,
`LoadHTTPRouteRawFilterConfigs`, `RawHTTPFilterConfig`.

**Imports**: `shared`, `backends`, `ir`, `extfilter`, `gatewayapi`.

### `listeners/` — Frontend validation and mesh services

**Responsibility**: Translates Gateway listener and frontend validation
configuration into the IR model, including mesh service frontend discovery
and listener set merging.

**Exports**: `FrontendValidationForListener`,
`FrontendValidationForListenerWithIndexes`, `CollectMeshServiceFrontends`,
`TranslateMeshServiceListeners`.

**Imports**: `shared`, `backends`, `ir`, `mesh`, `gatewayapi`.

### `policies/` — Policy attachment and translation

**Responsibility**: Provides translation helpers for Gateway API policy resources
and Nantian extension policy resources.

Sub-responsibilities:
- **Route attachment** (`attachments.go`): Resolves which routes attach to which
  listeners based on Gateway, ListenerSet, and Service mesh frontend semantics.
- **Gateway-scoped queries** (`gateway_scoped.go`): Lists GatewayClasses and
  Gateways using controller-runtime field indexes.
- **Policy target indexing** (`target_refs.go`): Sets up field indexes for
  BackendTLSPolicy, BackendLBPolicy, and RoutePolicy target references.
- **Token policy translation** (`token.go`): Converts TokenPolicy resources into
  IR token policy configs mapped to backend keys.
- **Wasm plugin translation** (`wasm.go`): Converts WasmPlugin resources into
  IR Wasm plugin configs, resolving inline, URL, and ConfigMap sources.

**Key types**:
- `ListenerSetMergeFunc` / `ListenerSetGateFunc` — Callback types for
  ListenerSet resolution during route attachment.
- `RouteKind` — Enum for route kinds in attachment logic.
- `attachmentPolicy` — Internal type bundling supported kinds, namespace mode,
  and label selector for a listener's allowed routes policy.

**Exports**: `AttachRoutes`, `RecordRouteAttachments`, `IsServiceParentRef`,
`IsListenerSetParentRef`, `RouteKindForStreamRoute`, `StreamRouteHostnames`,
`ListGatewayClassesForController`, `ListGatewaysForGatewayClass`,
`IsMissingFieldIndexError`, `SetupIndexes`,
`BackendTLSPolicyTargetRefIndexKeys`, `BackendLBPolicyTargetRefIndexKeys`,
`RoutePolicyTargetRefIndexKeys`, `BackendPolicyTargetRefIndexValuesByNamespace`,
`BackendPolicyTargetRefIndexValue`, `TranslateTokenPolicy`,
`TranslateTokenPolicies`, `ServiceKeySet`, `ServiceImportKeySet`,
`BuildRouteBackendServices`, `ReferencedConfigMapKeysForWasmPlugins`,
`TranslateWasmPlugin`, `TranslateWasmPlugins`.

**Imports**: `shared`, `ir`, `mesh`, `gatewayapi`, `extfilter`, `backend`
(gatewayexp), `routepolicy` (gatewayexp), `tokenpolicy` (gatewayexp),
`wasmplugin` (gatewayexp).

### `routepolicy/` — RoutePolicy translation

**Responsibility**: Translates RoutePolicy CRD resources into IR route policy
configs, resolving target references, scoping (namespace/gateway/route), and
policy precedence.

**Key types**:
- `translatedRoutePolicy` — Internal struct bundling route keys, config, and
  scope for a single RoutePolicy.
- `routePolicyScope` — Enum for scope resolution (namespace, gateway, route).

**Exports**: `BuildRoutePolicyIndexes`, `GrpcRoutesToHTTP`.

**Imports**: `ir`, `routepolicy` (gatewayexp).

### `aiservice/` — AIService translation

**Responsibility**: Translates AIService CRD resources into IR AI service configs.

**Exports**: `Translate`, `TranslateAll`.

**Imports**: `ir`, `aiservice` (gatewayexp).

### `testutil/` — Shared test helpers

**Responsibility**: Shared test fixtures, fake Kubernetes client builders, and
validating client wrappers used across translator test files. Imported
exclusively by `_test.go` files.

**Key types**:
- `fakeValidatingTranslatorClient` — Validates that certain List calls are
  not made during tests.
- `fakeScopedBuildDependencyValidatingClient` — Validates namespace-scoped
  lists for pods, ReferenceGrants, BackendLBPolicies, BackendTLSPolicies.
- `fakeScopedPolicyListValidatingClient` — Validates policy list scope.
- `fakeScopedReferenceGrantValidatingClient` — Validates ReferenceGrant scope.
- `fakeIndexedPolicyListValidatingClient` — Validates field-selector usage.
- `fakeFieldSelectorRejectingClient` — Simulates a cluster that doesn't
  support field selectors for BackendTLSPolicy.

**Exports**: `Ptr`, `Must`, `BuildSupportScheme`,
`NewTranslatorClientBuilder`, `NewFakeValidatingTranslatorClient`,
`NewFakeScopedBuildDependencyValidatingClient`,
`NewFakeScopedPolicyListValidatingClient`,
`NewFakeScopedReferenceGrantValidatingClient`,
`NewFakeIndexedPolicyListValidatingClient`,
`NewFakeFieldSelectorRejectingClient`, `RequireMatchingAnyField`,
`ListNamespace`, `BackendLBPolicyTargetRefIndexKeys`,
`ReadTestTLSAsset`, `SectionNamePtr`.

---

## Dependency Graph

```
                 ┌──────────────────────────────────────────────┐
                 │              translator (root)                │
                 │  Translator struct, Build(), partial rebuilds,│
                 │  support loaders, workloads, compat           │
                 └─────┬──────┬──────┬──────┬──────┬──────┬──────┘
                       │      │      │      │      │      │
         ┌─────────────┘      │      │      │      │      └─────────────┐
         ▼                    ▼      ▼      ▼      ▼                    ▼
   ┌──────────┐     ┌──────────┐  ┌─────┐  ┌──────┐  ┌──────────┐  ┌──────────┐
   │ backends │     │  routes  │  │list.│  │policies│ │routepol. │ │aiservice │
   │          │     │          │  │     │  │        │ │          │ │          │
   └────┬─────┘     └────┬─────┘  └──┬──┘  └───┬────┘ └────┬─────┘ └────┬─────┘
        │                │           │          │           │           │
        └────────┬───────┘           │          └──────┬────┘           │
                 │                   │                 │                │
                 ▼                   ▼                 ▼                ▼
          ┌─────────────────────────────────────────────────────────────────┐
          │                        shared (leaf)                            │
          │  indexes, helpers, limits, metrics, status summaries, timeouts │
          └──────────────────────────────┬──────────────────────────────────┘
                                         │
                                         ▼
                                   ┌──────────┐
                                   │    ir    │
                                   │ (types)  │
                                   └──────────┘
```

### Dependency Rules

| Sub-package | Imports | Imported by |
|---|---|---|
| `shared` | `ir`, standard library, prometheus | All sub-packages, root |
| `backends` | `shared`, `ir`, `mesh`, `extfilter`, `gatewayapi`, `backendtls`, `loadbalancing`, `backend` | `routes`, `listeners`, root |
| `routes` | `shared`, `backends`, `ir`, `extfilter`, `gatewayapi` | Root |
| `listeners` | `shared`, `backends`, `ir`, `mesh`, `gatewayapi` | Root |
| `policies` | `shared`, `ir`, `mesh`, `gatewayapi`, `extfilter`, `backend`, `routepolicy`, `tokenpolicy`, `wasmplugin` | Root, `cmd/manager` |
| `routepolicy` | `ir`, `routepolicy` (gatewayexp) | Root |
| `aiservice` | `ir`, `aiservice` (gatewayexp) | Root |
| `testutil` | standard library, controller-runtime, Gateway API, `gatewayapi`, `backend` | Test files only |

**Key observations**:
- **No circular dependencies found** — verified by `go vet ./internal/translator/...`.
- `shared` is a true leaf package — it does not import any translator sub-package.
- `backends` is a shared dependency between `routes`, `listeners`, and root.
- `policies` is the heaviest importer, pulling in 4 gatewayexp packages.
- `testutil` is an island — imported only by `_test.go` files.

---

## Key Interfaces and Types

### `Translator` (root package)

```go
type Translator struct {
    controllerName string
    namespace      string
    logger         *slog.Logger
    limits         shared.Limits
    fallbackCerts  *gwtls.FallbackCertManager
}
```

Entry point: `New(controllerName, logger)` or `NewWithOptions(...)`.
Main method: `Build(ctx, cl) (*ir.Snapshot, error)`.

### `shared.TranslatorIndexes`

```go
type TranslatorIndexes struct {
    servicesByKey                    map[string]corev1.Service
    serviceImportsByKey              map[string]mcsv1alpha1.ServiceImport
    tlsSecretsByKey                  map[string]corev1.Secret
    configMapCAPEMsByKey             map[string]string
    endpointSlicesByServiceKey       map[string][]discoveryv1.EndpointSlice
    endpointSlicesByServiceImportKey map[string][]discoveryv1.EndpointSlice
    ReferenceGrantsByNamespace       map[string][]gatewayv1beta1.ReferenceGrant
}
```

Single source of truth for all shared lookup indexes. Built once during `Build()`
and passed to sub-packages to avoid ad-hoc scans.

### `backends.BackendRefTranslator`

```go
type BackendRefTranslator struct {
    servicePorts               map[string]map[uint32]struct{}
    serviceImportPorts         map[string]map[uint32]struct{}
    referenceGrantsByNamespace map[string][]gatewayv1beta1.ReferenceGrant
    extensionResolver          extfilter.Resolver
    httpFilter                 HTTPFilterFunc
    grpcFilter                 GRPCFilterFunc
}
```

Resolves backend references with ReferenceGrant checking, service existence
validation, and cross-namespace permission enforcement.

### `backends.HTTPFilterFunc` / `backends.GRPCFilterFunc`

```go
type HTTPFilterFunc func(
    filters []gatewayv1.HTTPRouteFilter,
    defaultNamespace string,
    resolver extfilter.Resolver,
    target extfilter.Target,
) []ir.Filter

type GRPCFilterFunc func(
    filters []gatewayv1.GRPCRouteFilter,
    defaultNamespace string,
    resolver extfilter.Resolver,
    target extfilter.Target,
) []ir.Filter
```

Callback types that allow the root package to inject `routes.FiltersFromHTTPWithResolver`
and `routes.FiltersFromGRPCWithResolver` into `NewBackendRefTranslator`, avoiding
a direct import of `routes` from `backends`.

### `policies.ListenerSetMergeFunc` / `policies.ListenerSetGateFunc`

```go
type ListenerSetMergeFunc func(
    gateway gatewayv1.Gateway,
    base []gatewayv1.Listener,
    sets []gatewayv1.ListenerSet,
    namespaces map[string]corev1.Namespace,
) []gatewayv1.Listener

type ListenerSetGateFunc func(
    gateway gatewayv1.Gateway,
    ls gatewayv1.ListenerSet,
    namespaces map[string]corev1.Namespace,
) bool
```

Callback types injected by the root package to avoid circular imports between
the `policies` sub-package and the root's listener set resolution logic.

---

## Testing Guidelines

### Test Structure

Tests use Go's standard `testing` package. Test files are placed alongside
production code in each sub-package, plus the root package.

### Fake Clients

The `testutil` package provides a rich set of fake Kubernetes clients for
testing:

- `NewTranslatorClientBuilder` — Pre-configured `fake.ClientBuilder` with
  Gateway API scheme and field indexes.
- `NewFakeValidatingTranslatorClient` — Validates that unexpected List calls
  are not made.
- `NewFakeScopedBuildDependencyValidatingClient` — Validates namespace-scoped
  lists for pods, ReferenceGrants, policies.
- `NewFakeScopedPolicyListValidatingClient` — Validates policy list scope.
- `NewFakeScopedReferenceGrantValidatingClient` — Validates ReferenceGrant scope.
- `NewFakeIndexedPolicyListValidatingClient` — Validates field-selector usage.
- `NewFakeFieldSelectorRejectingClient` — Simulates a cluster without field
  selector support.

### What to Test

- **Route semantics**: Parent refs, hostname matching, filter resolution, timeout
  parsing, retry policy, session persistence.
- **Backend policy precedence**: BackendTLSPolicy and BackendLBPolicy precedence
  when multiple policies target the same backend.
- **ReferenceGrant behavior**: Cross-namespace refs with and without grants.
- **Status summaries**: Listener and route status condition translation.
- **Partial rebuild paths**: Incremental updates to routes, backends, and
  listeners maintain consistency.
- **IR shape**: Ensure the output IR matches expectations for all resource types.
- **Edge cases**: Empty lists, nil pointers, invalid durations, missing secrets,
  unmatched hostnames, cross-namespace refs without grants.

### Running Tests

```bash
# All translator tests
go test ./internal/translator/... -count=1 -timeout 120s

# Specific package
go test ./internal/translator/backends/... -count=1 -timeout 30s

# With race detector
go test -race ./internal/translator/... -count=1 -timeout 5m
```

---

## Verification Results (2026-08-10)

| Check | Result |
|---|---|
| `go vet ./internal/translator/...` | ✅ PASS (no circular dependencies) |
| `go build ./internal/...` | ✅ PASS |
| `go test ./internal/translator/... -count=1 -timeout 120s` | ✅ PASS (all 8 packages) |

### Per-package test results

| Package | Status | Duration |
|---|---|---|
| `github.com/nantian-gw/gateway/internal/translator` | ok | 0.200s |
| `github.com/nantian-gw/gateway/internal/translator/aiservice` | ok | 0.003s |
| `github.com/nantian-gw/gateway/internal/translator/backends` | ok | 0.121s |
| `github.com/nantian-gw/gateway/internal/translator/listeners` | ok | 0.096s |
| `github.com/nantian-gw/gateway/internal/translator/policies` | ok | 0.120s |
| `github.com/nantian-gw/gateway/internal/translator/routepolicy` | ok | 0.007s |
| `github.com/nantian-gw/gateway/internal/translator/routes` | ok | 0.010s |
| `github.com/nantian-gw/gateway/internal/translator/shared` | ok | 0.014s |
| `github.com/nantian-gw/gateway/internal/translator/testutil` | ? | no test files |

### LOC Summary

| Sub-package | Non-test LOC | Test LOC | Files | Notes |
|---|---|---|---|---|
| `shared/` | 409 | 242 | 7 | Healthy; leaf package |
| `backends/` | 1240 | 1688 | 12 | Largest sub-package; 8 production files, well-split by concern |
| `routes/` | 899 | 83 | 7 | 5 production files; test coverage is relatively low |
| `listeners/` | 140 | 990 | 3 | Small, focused; good test coverage |
| `policies/` | 989 | 3476 | 10 | 5 production files, 5 test files; heavy test coverage |
| `routepolicy/` | 391 | 719 | 2 | Focused, good test coverage |
| `aiservice/` | 36 | 121 | 2 | Smallest; good test coverage |
| `testutil/` | 335 | 0 | 2 | Test-only utility; no test files needed |
| Root | 4579 | 8081 | 13 | Largest package; 7 production files + 6 test files |

### Observations

1. **No circular dependencies** — `go vet` confirms all import graphs are acyclic.
2. **`shared` is a true leaf** — only imports `ir`, prometheus, and stdlib.
3. **`backends` is the most complex sub-package** — 1240 non-test LOC across 8
   files, but each file has a single responsibility (translate.go, refs.go,
   tls.go, tls_policy.go, lb_policy.go, load_balancing.go, session.go).
4. **Callback pattern avoids circular imports** — `BackendRefTranslator` takes
   `HTTPFilterFunc`/`GRPCFilterFunc` callbacks instead of importing `routes`
   directly. Same pattern with `ListenerSetMergeFunc`/`ListenerSetGateFunc` in
   `policies`.
5. **Root package is large** — 4579 non-test LOC. The `doc.go` acknowledges this
   and the split moved route/backend/listener logic out, but the root retains
   `Build()`, support loaders, partial rebuilds, and workloads. Future
   consideration: extract support loaders and workload translation into
   dedicated sub-packages.
6. **`routes/` test coverage is thin** — 83 test LOC vs 899 production LOC.
   Consider adding more unit tests for filter edge cases, stream route
   hostname intersection, and raw filter config parsing.