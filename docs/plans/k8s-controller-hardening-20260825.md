# Kubernetes Controller Hardening Plan - 2026-08-25

## Scope

Optimize and harden the Gateway control plane Kubernetes controller behavior only. Changes are limited to the `gateway` repository worktree:

- Align TLSRoute status/index/list behavior with the existing `enableExperimentalGateway` feature gate.
- Add the RBAC permission needed for admin `authMode: kubernetes` TokenReview authentication.
- Remove stale CI test skips after making the affected logging test deterministic.

No Data Plane, Dashboard, Website, Helm, Proto, Platform Release, or workspace-root files may change.

## Acceptance Criteria

1. Standard mode does not register TLSRoute status controllers, TLSRoute route indexes, or TLSRoute list paths.
2. Experimental mode still registers and reconciles TLSRoute status paths.
3. Optional TLSRoute controller registration is skipped when the TLSRoute CRD is absent.
4. The base control plane ClusterRole grants `authentication.k8s.io/tokenreviews` `create` so `adminAuth.authMode: kubernetes` can validate bearer tokens.
5. CI no longer skips `TestReconcileIgnoresManagedFrontendResourceChanges`, `TestSyncerReconcileFailureQueuesScopedRetryWithoutFullRebuild`, or `TestConfigureKubernetesLoggingRoutesKlogThroughSlog`.
6. The Kubernetes logging test is deterministic across repeated runs.
7. Verification commands pass:
   - `go test -count=1 ./internal/status ./internal/controller ./cmd/manager`
   - `go test -count=50 ./cmd/manager -run 'TestConfigureKubernetesLoggingRoutesKlogThroughSlog'`
   - `go test -count=50 ./internal/controller -run 'TestReconcileIgnoresManagedFrontendResourceChanges|TestSyncerReconcileFailureQueuesScopedRetryWithoutFullRebuild'`
   - `go test -count=1 -timeout 10m ./...`
   - `git diff --check`
   - From workspace root, sibling component repositories remain unchanged.

## Implementation Plan

1. Update status controller setup, indexes, and state loading so TLSRoute follows the experimental Gateway gate.
2. Add focused tests for standard-mode TLSRoute exclusion and experimental-mode retention.
3. Add TokenReview RBAC to the control plane ClusterRole.
4. Isolate Kubernetes logging tests from controller-runtime global logger state and remove CI skips.
5. Run focused checks, repeated stability checks, full Go tests, and workspace cleanliness checks.

## Verification Results

- `go test -count=1 ./internal/status ./internal/controller ./cmd/manager` — passed.
- `go test -count=50 ./cmd/manager -run 'TestConfigureKubernetesLoggingRoutesKlogThroughSlog'` — passed.
- `go test -count=50 ./internal/controller -run 'TestReconcileIgnoresManagedFrontendResourceChanges|TestSyncerReconcileFailureQueuesScopedRetryWithoutFullRebuild'` — passed.
- `go test -count=1 -timeout 10m ./...` — passed.
- `go build ./...` — passed.
- `go test -race -count=1 -timeout 10m ./...` — passed.
- `git diff --check` — passed.
- `GOROOT=$(GOTOOLCHAIN=go1.26.4 go env GOROOT) GOTOOLCHAIN=go1.26.4 golangci-lint run --enable-only=govet,copyloopvar,bodyclose,durationcheck,errorlint,ineffassign,sqlclosecheck` — passed, 0 issues.
- `for d in gateway dataplane proto dashboard website helm-charts platform-release; do git -C /root/nantian-gw/$d status --short --branch; done` — passed; sibling component repositories remained unchanged.
