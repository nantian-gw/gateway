# Control Plane Snapshot Stability — 2026-08-27

## Scope

- Repository changed: `gateway` only, in worktree `/root/.opencode/worktrees/gateway-snapshot-stability-20260827`.
- Problem: data planes can still apply alternating full snapshot versions even when Gateway, HTTPRoute, Service, and EndpointSlice data-plane inputs are effectively unchanged.
- Primary suspicion: snapshot version computation may still include Kubernetes/operator metadata that is useful for admin display but should not invalidate the data-plane snapshot.
- Out of scope: Data Plane, Dashboard, Helm, Proto, and root workspace documentation commits.

## Implementation Plan

1. Add focused regression tests for snapshot digest stability around route metadata fields.
2. Keep true data-plane changes versioned while excluding status/display-only fields from the digest.
3. Preserve stored snapshot contents for admin APIs and status reporting; only the digest projection should change.
4. Run focused IR/controller/translator checks, then the gateway repository acceptance checks.

## Acceptance Criteria

- `go test ./internal/ir` passes.
- `go test ./internal/controller` passes.
- `go test ./internal/translator` passes.
- `make test` passes from the `gateway` repository root.
- `git diff --check origin/main...HEAD` passes.
- The main checkout remains clean before merge, and only the gateway repository is modified.

## Verification Results

- `go test ./internal/ir` — passed.
- `go test ./internal/controller -run 'Predicate|Snapshot'` — passed.
- `go test ./internal/translator` — passed.
- `go test ./internal/controller` — passed.
- `make test` — passed (`go test -count=1 -timeout 5m ./...`).
- `git diff --check origin/main...HEAD` — passed.
- `git -C /root/nantian-gw/gateway status --short --branch` — clean on `main...origin/main` before merge.
