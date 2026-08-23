# Snapshot Input Predicate Optimization - 2026-08-23

## Scope

Optimize the Gateway control plane snapshot syncer only. Changes are limited to the `gateway` repository worktree:

- Tighten snapshot input update predicates so status-only and irrelevant annotation-only updates do not trigger snapshot rebuild work.
- Preserve create/delete/generic handling and all semantically relevant config triggers.
- Keep ListenerSet Accepted status handling unchanged because it can affect route attachment semantics.
- Do not modify Data Plane, Dashboard, Website, Helm, Proto, or Platform Release repositories.

## Acceptance Criteria

1. HTTPRoute status-only updates are ignored by `snapshotInputMutationPredicate`.
2. Irrelevant annotation-only updates on HTTPRoute and Gateway are ignored by `snapshotInputMutationPredicate`.
3. Generation changes, label changes, and `gateway.nantian.dev/*` annotation changes still trigger snapshot reconciliation.
4. Create, delete, and generic events still trigger snapshot reconciliation for watched snapshot inputs.
5. ListenerSet Accepted status update behavior remains unchanged.
6. The storm benchmark predicates no longer fail for status-only and irrelevant annotation-only updates.
7. Verification commands pass:
   - `go test ./internal/controller -run 'TestSnapshotInputMutationPredicate|TestSnapshotListenerSetMutationPredicate'`
   - `go test ./internal/controller -run '^$' -bench 'BenchmarkSnapshotInput(StatusStorm|IrrelevantAnnotationStorm)' -benchmem -count=1`
   - `go test ./internal/controller`
   - `go test ./internal/ir -run 'TestSnapshotDigestIgnoresStatusSummaries' -count=1`
   - `gofmt -l internal/controller/syncer_predicate.go internal/controller/syncer_predicate_test.go`
   - `git diff --check`
   - `make test`
   - From workspace root, sibling component status checks show no unrelated repository changes caused by this work.

## Implementation Plan

1. Update predicate tests to assert status-only and irrelevant annotation-only updates are skipped.
2. Replace the unconditional update predicate with create/delete/generic-only fallback behavior.
3. Keep generation, label, and relevant annotation predicates as the only update triggers.
4. Run focused tests/benchmarks, then the full gateway test suite.
5. Commit the worktree branch, merge into `gateway/main`, push `main`, and monitor CI.

## Verification Results

- `go test ./internal/controller -run 'TestSnapshotInputMutationPredicate|TestSnapshotListenerSetMutationPredicate' -count=1` - pass (`ok github.com/nantian-gw/gateway/internal/controller 0.011s`).
- `go test ./internal/controller -run '^$' -bench 'BenchmarkSnapshotInput(StatusStorm|IrrelevantAnnotationStorm)' -benchmem -count=1` - pass; status-only and irrelevant annotation storms no longer fail and report `0 B/op`, `0 allocs/op`.
- `go test ./internal/controller -count=1` - pass (`ok github.com/nantian-gw/gateway/internal/controller 0.669s`).
- `go test ./internal/ir -run 'TestSnapshotDigestIgnoresStatusSummaries' -count=1` - pass (`ok github.com/nantian-gw/gateway/internal/ir 0.002s`).
- `gofmt -l internal/controller/syncer_predicate.go internal/controller/syncer_predicate_test.go` - pass, no output.
- `git diff --check` - pass, no output.
- `make test` - pass (`go test -count=1 -timeout 5m ./...`).
- Workspace sibling component status check from `/root/nantian-gw`: `gateway`, `dataplane`, `proto`, `dashboard`, `website`, `helm-charts`, and `platform-release` all remained on clean `main...origin/main`.
