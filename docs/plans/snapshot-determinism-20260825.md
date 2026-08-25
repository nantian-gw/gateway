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

## Acceptance Criteria

1. Reproduce the issue with a focused test that builds snapshots repeatedly from unchanged inputs and fails before the fix.
2. Snapshot ID generation is deterministic for unchanged Gateway/HTTPRoute/Service/EndpointSlice inputs.
3. Snapshot ID generation is deterministic across Translator instances sharing the same Kubernetes fallback CA Secret.
4. Snapshot content ordering remains canonicalized at the IR boundary, without changing Gateway API semantics.
5. Existing partial rebuild and full rebuild behavior remains intact.
6. Verification commands pass:
   - `go test -count=1 ./internal/tls ./internal/controller ./internal/translator ./internal/ir`
   - `go test -count=100 ./internal/translator -run 'TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossRepeatedBuilds|TestBuildSnapshotKeepsFallbackTLSCertificateStableAcrossTranslatorInstances|TestBuildGatewayListenersForSnapshotPreservesFallbackTLSCertificate'`
   - `go test -count=1 -timeout 10m ./...`
   - `git diff --check`

## Implementation Plan

1. Trace snapshot ID calculation and publication short-circuit logic. Done.
2. Add regression tests for repeated full builds and Gateway listener partial rebuilds with unchanged TLS listener inputs. Done.
3. Cache fallback TLS leaf certificates per canonical hostname set in the `FallbackCertManager`. Done.
4. Reuse fallback injection in `BuildGatewayListenersForSnapshot` after listener/secret replacement. Done.
5. Persist fallback TLS leaf certificates in the fallback CA Secret so separate Translator instances reuse identical material. Done.
6. Reload the persisted fallback CA when losing a Secret create race. Done.
7. Run focused repeated tests, full tests, and diff checks. Done.

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
  - `go test -count=1 -timeout 10m ./...`
  - Result: passed.
  - `git diff --check`
  - Result: passed.
