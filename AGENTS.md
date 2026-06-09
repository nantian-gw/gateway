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

## Acceptance

Every change needs a spec, plan, and strict acceptance criteria. Record exact verification commands and results before marking work complete.

For documentation-only changes in this repository, run at least:

- `go test ./internal/translator` when touching translator documentation.
- `make test` unless the plan explicitly scopes a smaller command and records why.
- A local README link/path check when rewriting `README.md`.
- `git diff --check origin/main...HEAD`.

For behavior changes, add focused tests first and then run all affected package checks.
