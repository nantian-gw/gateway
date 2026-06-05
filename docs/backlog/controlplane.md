# Controlplane Backlog

This document tracks control plane performance, status reconciliation, infrastructure orchestration, Gateway API gaps, and maintainability work.

## P1: Scoped Reconcile

Goal: Evolve `ReconcilerRunner` from undifferentiated full reconcile to scoped reconcile, reducing the frequency of full status / infrastructure recomputation.

Background:

- The current runner runs infrastructure and status reconcilers serially.
- Snapshot publish, node state changes, and periodic tickers easily trigger heavy global reconciliation.
- `status.Reconciler` already has object-level paths, but the runner lacks a scope dimension.

Acceptance:

- Runner trigger distinguishes at minimum `infra`, `gateway-status`, `route-status`, `policy-status`, `full`.
- Snapshot publish, node state changes, Gateway/Route/Policy changes only trigger the affected scope.
- Metrics add a scope label, e.g., `aether_gateway_controlplane_reconciler_runner_duration_seconds{scope=...}`.
- Keep low-frequency full reconcile as a safety net.
- Minimum validation: `cd controlplane && go test ./internal/controller ./internal/status ./internal/infrastructure -count=1`.

## P1: Large File And Test Split

Goal: Continuously split oversized production files and oversized test files to reduce review surface and regression localization cost.

Current targets:

- `controlplane/internal/status/reconciler_core_test.go`
- `controlplane/internal/translator/build_support_test.go`
- `controlplane/internal/controller/syncer_partial_rebuild_test.go`
- `controlplane/internal/infrastructure/reconciler_core_test.go`

Completed background:

- `controlplane/internal/controller/syncer_watch.go` has been split into index, request, backend, watch helper, and other responsibility files.
- Subsequent splits should remain pure refactoring without mixing in behavioral changes.

Acceptance:

- Test splits organized by scenario: gateway status, route status, policy status, infra convergence, conflict retry, generation freshness, TLS assets, shared bind handoff.
- Prioritize splitting when a single file approaches 800 lines.
- Minimum validation: `go test -count=1` for the target package.

## P1: Gateway API Remaining Gaps

Goal: Continue filling the currently undeclared or unimplemented Gateway API subset based on the latest full-suite skip/fail inventory.

Key outstanding items:

- `ListenerSet` is explicitly not entering the current `supportedFeatures` declaration, tracked as a post-v0.4 proposal/implementation; if opened later, it requires independent design of watch, translator, status, attachment, and conformance verification.
- `TLSRouteModeTerminate` / `TLSRouteModeMixed` are explicitly not declared as supported at this time: `TLSRoute` only covers core passthrough, TLS termination is handled by HTTPS listeners, and terminate/passthrough mixed mode on the same listener continues as a future independent design item.
- Additional Gateway API experimental expansions have completed phase assessment; the current decision is not to add new `supportedFeatures` declarations. See [Gateway API Experimental Feature Audit](gateway-api-experimental.md) for details.

Completed:

- `GatewayHTTPSListenerDetectMisdirectedRequests` has been supplemented with HTTPS SNI/Host mismatch behavior verification and `supportedFeatures` declaration restored.
- `GatewayFrontendClientCertificateValidation` / `GatewayFrontendClientCertificateValidationInsecureFallback` `supportedFeatures` declarations restored; `2026-05-09` targeted conformance covers strict, insecure fallback, invalid default frontend validation, and invalid frontend validation status.
- `HTTPRouteCORS` `supportedFeatures` declaration restored; control plane support list test pins the feature name. `2026-05-09` targeted conformance `HTTPRouteCORS` and `HTTPRouteCORSAllowCredentialsBehavior` both `PASS`, evidence at `tmp/conformance/httproute-cors-report.yaml` and `tmp/conformance/httproute-cors-allow-credentials-report.yaml`.
- `Mesh` and mesh HTTPRoute extension features `supportedFeatures` declarations restored; control plane support list test pins `Mesh`, `MeshClusterIPMatching`, `MeshConsumerRoute`, `MeshHTTPRoute*` declarations. `2026-05-09` targeted Mesh suite `PASS`, covering `MeshGRPCRouteWeight`, `MeshHTTPRoute307Redirect`, mesh redirect/rewrite/header/query/named rule, consumer route, frontend, ports, and traffic split; evidence at `tmp/conformance/mesh-supported-current-report.yaml` and `tmp/conformance/mesh-supported-current-report.log`.
- `TLSRoute` core passthrough `supportedFeatures` declaration restored; control plane support list test pins `TLSRoute` declaration, support matrix refreshed to 53 externally declared features. `2026-05-09` targeted conformance `Gateway,TLSRoute,ReferenceGrant` feature combination `PASS`, covering `TLSRouteHostnameIntersection`, `TLSRouteInvalidReferenceGrant`, `TLSRouteListenerTerminateNotSupported` and other core test cases; evidence at `tmp/conformance/tlsroute-restored-current-report.yaml` and `tmp/conformance/tlsroute-restored-current-report.log`.

Acceptance:

- Each capability must have unit / targeted conformance or e2e smoke before restoring `supportedFeatures` declaration.
- Support matrix updates all four layers: declared / implemented / tested / production-validated.
- Do not present implementation subsets as complete production-ready capabilities.

## P2: Watch / Index Contract Maintenance

Completed foundation:

- `controlplane/internal/controller` now has a reference index contract.
- When `BackendTLSPolicy` ConfigMap index is missing, structured fallback logging is recorded and namespace fallback is used.
- Controller contract and missing-index fallback already have unit tests.

Ongoing requirements:

- New resource integration must declare index, watch source, request mapping, and fallback semantics.
- Index-missing fallback must not silently revert to full scan.
- Changes involving `syncer_watch*` must first run controller package unit tests, not start kind directly.

## References

- [Architecture](../architecture.md)
- [IR layering](../ir-layering.md)
- [Gateway API support matrix](../gateway-api-support.md)
- [Gateway API experimental audit](gateway-api-experimental.md)
- [Status matrix](../status-matrix.md)
- [Regression index](../test/regression-index.md)
