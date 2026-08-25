# Snapshot Determinism Investigation Plan - 2026-08-25

## Scope

Investigate and fix excessive control plane snapshot publication when Gateway, routes, Services, and EndpointSlices are stable. Changes are limited to the `gateway` repository worktree.

No Data Plane, Dashboard, Website, Helm, Proto, Platform Release, or workspace-root files may change.

## Findings

Repeated versions that cycle back to prior hashes were caused by fallback TLS certificate handling rather than unordered Kubernetes list fan-in.

- Full snapshot builds called `injectFallbackCertificates`, which generated a fresh random fallback leaf key/certificate on every build for TLS listeners without user-provided `certificateRefs`.
- The generated fallback `SecretMaterial` is included in the IR snapshot digest, so any unrelated rebuild could publish a new snapshot version even when Gateway, HTTPRoute, Service, and EndpointSlice runtime state was unchanged.
- Gateway listener partial rebuilds did not re-run fallback injection after replacing listener TLS refs and secret material, so full and partial rebuild paths could alternate between snapshots with and without fallback TLS material.
- Process-local fallback leaf caching is not sufficient for Kubernetes deployments with multiple control plane replicas or restarts. Each Translator instance could still generate a different leaf for the same listener, causing data planes to alternate between snapshot hashes as they reconnect to different replicas.
- The fallback CA creation race also needs deterministic handling: if a replica loses a create race, it must reload the persisted CA rather than continuing with locally generated material.
- Post-push CI annotations exposed generated protobuf descriptor drift in the gateway vendored Go stubs. `buf generate ../proto` updated `gen/go/gateway/control/v1/config.pb.go` without requiring proto source changes.

Follow-up investigation on 2026-08-25 found two additional sources of unnecessary snapshot work:

- Service and EndpointSlice watches filtered managed frontend objects, but their update predicate still admitted status-only and irrelevant metadata updates for user Services and EndpointSlices.
- Gateway `status.addresses` is status-derived display metadata. When another event caused a rebuild while Gateway status addresses were converging or drifting, the listener `nantian.dev/display-addresses` metadata changed the runtime snapshot digest even though Gateway/Route/Service/Endpoint runtime routing inputs were unchanged.

## Acceptance Criteria

1. Reproduce the issue with a focused test that builds snapshots repeatedly from unchanged inputs and fails before the fix.
2. Snapshot ID generation is deterministic for unchanged Gateway/HTTPRoute/Service/EndpointSlice inputs.
3. Snapshot ID generation is deterministic across Translator instances sharing the same Kubernetes fallback CA Secret.
4. Snapshot content ordering remains canonicalized at the IR boundary, without changing Gateway API semantics.
5. Existing partial rebuild and full rebuild behavior remains intact.
6. Service and EndpointSlice update predicates skip status-only or irrelevant metadata updates, while preserving runtime-relevant Service port/mesh metadata changes and EndpointSlice endpoint/port/service-label changes.
7. Gateway status-derived listener display addresses do not change the runtime snapshot digest.
8. Verification commands pass:
   - `go test -count=1 ./internal/tls ./internal/controller ./internal/translator ./internal/ir`
   - `go test -count=100 ./internal/translator -run 'TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossRepeatedBuilds|TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossTranslatorInstances|TestBuildGatewayListenersForSnapshotPreservesFallbackTLSCertificate'`
   - `go test -count=1 -timeout 10m ./...`
   - `git diff --check`
   - Best-effort local kind smoke test, unless blocked by local image pull/network.

## Implementation Plan

1. Trace snapshot ID calculation and publication short-circuit logic. Done.
2. Add regression tests for repeated full builds and Gateway listener partial rebuilds with unchanged TLS listener inputs. Done.
3. Cache fallback TLS leaf certificates per canonical hostname set in the `FallbackCertManager`. Done.
4. Reuse fallback injection in `BuildGatewayListenersForSnapshot` after listener/secret replacement. Done.
5. Persist fallback TLS leaf certificates in the fallback CA Secret so separate Translator instances reuse identical material. Done.
6. Reload the persisted fallback CA when losing a Secret create race. Done.
7. Refresh generated Go protobuf stubs after CI reported descriptor drift. Done.
8. Run focused repeated tests, full tests, generation checks, and diff checks. Done.
9. Add Service/EndpointSlice predicate regression tests for status-only updates and runtime-relevant changes. Done.
10. Exclude listener display address metadata from the runtime snapshot digest while preserving it on the stored snapshot. Done.

## Verification Results

- Pre-fix reproduction:
  - `go test -count=1 ./internal/translator -run 'TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossRepeatedBuilds|TestBuildGatewayListenersForSnapshotPreservesFallbackTLSCertificate'`
  - Result: failed; repeated full build changed the snapshot ID, and partial listener rebuild changed the snapshot ID.
- Post-fix focused checks:
  - `go test -count=1 ./internal/tls -run TestFallbackCertManagerPersistsLeafAcrossManagers -v`
  - Result: passed.
  - `go test -count=1 ./internal/translator -run 'TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossRepeatedBuilds|TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossTranslatorInstances|TestBuildGatewayListenersForSnapshotPreservesFallbackTLSCertificate'`
  - Result: passed.
  - `go test -count=1 ./internal/tls ./internal/controller ./internal/translator ./internal/ir`
  - Result: passed.
  - `go test -count=100 ./internal/translator -run 'TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossRepeatedBuilds|TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossTranslatorInstances|TestBuildGatewayListenersForSnapshotPreservesFallbackTLSCertificate'`
  - Result: passed.
  - `buf generate ../proto && go mod tidy`
  - Result: updated `gen/go/gateway/control/v1/config.pb.go`; no module file changes.
  - `go test -count=1 -timeout 10m ./...`
  - Result: passed.
  - `git diff --check`
  - Result: passed.
- Follow-up checks for Service/EndpointSlice predicate and display-address digest fix:
  - `go test -count=1 ./internal/controller ./internal/ir`
  - Result: passed.
  - `go test -count=1 ./internal/tls ./internal/controller ./internal/translator ./internal/ir`
  - Result: passed.
  - `go test -count=1 -timeout 10m ./...`
  - Result: passed.
  - `git diff --check`
  - Result: passed.
- Local kind validation:
  - Built a local `ghcr.io/nantian-gw/nantian-controlplane:bugscan` image from the current worktree via host `go build` and `docker import` after Dockerfile builds timed out during `go mod download`.
  - `kind create cluster --name nantian-bugscan --config scripts/ci/kind-ci-config.yaml --wait 5m`
  - Result: blocked by local Docker networking before Kubernetes bootstrapped. Docker failed to program DNAT rules with `iptables v1.8.13 (nf_tables): RULE_APPEND failed (No such file or directory): rule in chain DOCKER`.
  - Confirmed the issue is host Docker networking, not the gateway manifests, with `docker run --name ntgw-port-test -d -p 127.0.0.1:18080:80 docker.io/library/nginx:alpine`, which failed with the same DNAT error.
