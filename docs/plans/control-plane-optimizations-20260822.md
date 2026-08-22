# Control Plane Optimization Plan - 2026-08-22

## Scope

Optimize the Gateway control plane only. Changes are limited to the `gateway` repository worktree:

- xDS delta stream responsiveness and delta diff complexity.
- Translator route translation / backend-ref annotation concurrency limits.
- Admin `/v1/gateways` aggregation cost.
- Unused xDS snapshot proto wire-byte cache work.
- Existing Go formatting drift in touched files.

No Data Plane, Dashboard, Website, Helm, Proto, or Platform Release repository files may change.

## Acceptance Criteria

1. Delta xDS processes published snapshots even when the client is idle and not sending additional requests.
2. Delta resource diff is O(n) per resource type and does not repeatedly scan old resources for every new item.
3. Delta resource names remain backward-compatible with the current data plane delta cache behavior.
4. Translator full snapshot route translation and backend-ref annotation use bounded concurrency.
5. `/v1/gateways` only builds listener-to-gateway and backend indexes when the selected `include=` flags need them.
6. Snapshot proto cache does not pre-marshal or retain unused wire-byte copies.
7. All touched Go files are `gofmt` clean.
8. Verification commands pass:
   - `go test ./internal/xds ./internal/admin ./internal/controller ./internal/translator`
   - `go test ./internal/xds -run '^TestDeltaServer|^TestDeltaDiff|^TestSnapshotDelta'`
   - `go test ./internal/admin -run '^TestGatewaysEndpoint'`
   - `go test ./internal/translator -run '^$' -bench 'BenchmarkBuildSnapshotRouteFanout' -benchmem`
   - `go test ./internal/xds -run '^$' -bench 'BenchmarkToProtoSnapshotFanout' -benchmem`
   - `go test ./internal/controller -run '^$' -bench 'BenchmarkPublishSnapshotRouteFanout' -benchmem`
   - `git diff --check`
   - From workspace root, `git -C <component> status --short` for sibling component repositories shows no changes.

## Implementation Plan

1. Add regression coverage for idle delta snapshot delivery and O(n) delta version calculation.
2. Refactor delta stream receive handling to use a receive goroutine and a single select loop.
3. Rewrite delta diff to precompute old/new version maps.
4. Add bounded concurrency constants to translator route translation and backend-ref annotation phases.
5. Refactor `/v1/gateways` aggregation to compute listeners/backends lazily and reuse per-request backend indexes.
6. Remove the unused xDS snapshot wire-byte cache path.
7. Format, test, benchmark, and record exact command results in the final handoff.

## Verification Results

- `gofmt -l internal/xds/delta_server.go internal/xds/delta_server_integration_test.go internal/xds/delta_diff.go internal/xds/delta_diff_test.go internal/xds/snapshot_proto_cache.go internal/translator/translator.go internal/admin/server_gateways.go internal/translator/partial_backends.go internal/admin/server_response.go` - pass, no output.
- `go test ./internal/xds -run '^TestDeltaServer|^TestDeltaDiff|^TestSnapshotDelta'` - pass.
- `go test ./internal/admin -run '^TestGatewaysEndpoint'` - pass.
- `go test ./internal/translator` - pass.
- `go test ./internal/xds ./internal/admin ./internal/controller ./internal/translator` - pass.
- `go test ./internal/xds -run 'TestDeltaServer_(PushesSnapshotWhileClientIdle|DynamicSubscription|AckNonce|RemovedResources)' -count=50` - pass.
- `go test ./internal/admin -run '^TestGatewaysEndpoint' -count=20` - pass.
- `make test` - pass (`go test -count=1 -timeout 5m ./...`).
- `go test ./internal/xds -run '^$' -bench 'BenchmarkToProtoSnapshotFanout' -benchmem` - pass; cached fanout remains zero-allocation (`nodes_100`: `3332 ns/op`, `2 B/op`, `0 allocs/op`).
- `go test ./internal/translator -run '^$' -bench 'BenchmarkBuildSnapshotRouteFanout' -benchmem` - pass; `routes_200`: `9741861 ns/op`, `3788401 B/op`, `23811 allocs/op`.
- `go test ./internal/admin -run '^$' -bench 'BenchmarkFilterRoutesQueryRouteFanout' -benchmem` - pass; `routes_200`: `174159 ns/op`, `116369 B/op`, `1509 allocs/op`.
- `go test ./internal/controller -run '^$' -bench 'BenchmarkPublishSnapshotRouteFanout' -benchmem` - pass; `routes_200`: `4906035 ns/op`, `1303784 B/op`, `10942 allocs/op`.
- `git diff --check` - pass, no output.
- Workspace component status check from `/root/nantian-gw`: `gateway`, `dataplane`, `proto`, `dashboard`, and `website` were clean. `helm-charts` showed `M charts/nantian-gw/templates/aiservice-crd.yaml`; `platform-release` showed untracked `dist/` and `tmp/`. These are outside the gateway worktree and were left untouched.
