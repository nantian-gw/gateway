# CLAUDE.md

Go control plane for Nantian Gateway. It watches Kubernetes Gateway API resources, translates them into an internal routing model (IR), serves admin APIs, and publishes runtime snapshots to Rust data planes over gRPC/xDS.

See [AGENTS.md](AGENTS.md) for the fuller repository guide (translator maintenance rules, generated-code policy, acceptance criteria). This file adds verified build commands and an accurate package map. Where AGENTS.md or README.md name package paths, trust the map below — several paths in those docs are stale.

## Commands

Run from the repository root.

- `make build` — `go build ./...`
- `make test` — `go test -count=1 -timeout 5m ./...`
- `go test ./internal/<pkg>` — focused package tests while iterating
- `make e2e-smoke` — Kind smoke test
- `make e2e` — full e2e scenarios
- `make conformance` — Kind cluster + Gateway API conformance
- `make benchmarks` — `go test -bench=. -benchtime=1x -count=1 ./...`

Prefer focused `go test ./path` while iterating, then run the broader relevant target before committing.

## Project Map

Verified against the filesystem (README.md / AGENTS.md list some renamed-away paths — do not use those):

- `cmd/manager/` — controller manager entrypoint, wires runtime services
- `internal/controller/` — watches Kubernetes resources, drives full/partial rebuilds
- `internal/translator/` — Gateway API resources + policies → IR snapshots (highest-risk package)
- `internal/xds/` — gRPC/xDS server: publishes snapshots and status to data planes (NOT `internal/grpcserver`)
- `internal/gwapi/` — Gateway API helpers, validation, supported-feature declarations (NOT `internal/gatewayapi`)
- `internal/gwexp/` — experimental/extension resources: `aiservice`, `tokenpolicy`, `wasmplugin`, `routepolicy`, `backendlb` (NOT `internal/gatewayapiexperimental`)
- `internal/admin/` — operational, topology, metrics, and management APIs (auth in `auth.go`, rate limiting in `rate_limiter.go`, chatbot under `chatbot/`)
- `internal/ir/` — internal routing/runtime model shared by translator and xDS
- `internal/config/` — config schema, defaults, and duration/accessor helpers
- `internal/observability/` — Prometheus metrics
- `internal/{status,resources,nodeinfo,lifecycle,infrastructure,mesh,lbpolicy,tlspolicy,extfilter,compat}/` — supporting subsystems
- `deploy/kubernetes/overlays/` — Kustomize overlays (`production`, `kind-conformance`, `observability-enabled`, `kind-pprof`); there is no in-repo Helm chart
- `gen/` — generated protobuf; do not hand-edit

## Conventions

- Do not edit generated files under `gen/`. Change `.proto` in the sibling Proto repo and regenerate.
- English for code comments and docs by default; add localized text only when editing existing localized content.
- Translator changes must preserve Gateway API semantics (parent refs, route attachment, listener validity, backend refs, filters, status conditions), ReferenceGrant/namespace scoping, and policy precedence (BackendTLSPolicy, BackendLBPolicy, session persistence, AIService, TokenPolicy, WasmPlugin). Add focused tests for any such change.
- Config follows a consistent pattern: string/typed yaml field on the `*Config` struct, a default applied in `Load`, and an accessor method. New gRPC runtime knobs are opt-in (zero value keeps gRPC defaults / BDP autotuning).

## Git

Commit directly to `main` only when the user explicitly asks; the same applies to pushing to `origin`. Confirm before any commit or push unless the user has just authorized it for the current change.
