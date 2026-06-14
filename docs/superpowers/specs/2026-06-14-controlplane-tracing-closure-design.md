# Controlplane Tracing Closure Design

## Summary

This design closes the current gap between "the control plane contains tracing code" and "operators can enable, verify, and troubleshoot controlplane tracing from this repository alone."

The work stays strictly inside the `gateway` repository. It will not modify the sibling `dataplane`, `helm-charts`, `dashboard`, `website`, or `proto` repositories.

## Problem Statement

The control plane already has a partial tracing foundation:

- `internal/observability/tracing.go` configures OTLP gRPC export.
- `cmd/manager/app.go` wires tracing at startup.
- `internal/controller/`, `internal/infrastructure/`, `internal/admin/`, and `internal/grpcserver/` already emit some spans.
- `internal/config/` already parses tracing settings.

However, the current state is still incomplete from an operator's perspective:

1. Tracing startup is not obvious from controlplane startup behavior. Operators can enable tracing in config, but there is no explicit structured confirmation of the effective tracing settings.
2. Existing controlplane spans cover only part of the operational picture. The highest-value reconcile and publish paths still need better diagnostic attributes so that a failed or partial run can be understood from trace data alone.
3. Repository deployment assets do not provide a single explicit tracing-enabled Kustomize entry point, even though `deploy/` already exposes production and observability-oriented assets.
4. Repository documentation mentions observability assets, but does not provide a concise "how to enable controlplane tracing, how to verify it, and how to troubleshoot obvious misconfiguration" path.

## Goals

1. Make controlplane tracing startup state explicit and inspectable.
2. Increase the diagnostic value of controlplane trace spans without introducing a broad observability refactor.
3. Add a repository-local Kustomize deployment entry point for controlplane tracing.
4. Document a minimal enable/verify/troubleshoot workflow in the `gateway` repository.
5. Keep the implementation bounded to one component repository and compatible with existing defaults.

## Non-Goals

- No `dataplane` tracing changes.
- No Helm chart changes.
- No website or public docs site changes.
- No bundled OpenTelemetry Collector, Jaeger, Tempo, or other tracing backend installation assets.
- No Grafana dashboard redesign.
- No alerting-rule expansion or metrics naming cleanup.
- No change to default tracing behavior for existing base or production installs unless the user explicitly chooses the new tracing-enabled entry point.

## Existing Constraints

- The repository must be changed in an isolated worktree.
- The default deployment behavior must remain unchanged for existing base, Kind, conformance, and production entry points.
- Configuration remains file-first. This batch should not introduce a second, competing configuration system.
- The solution must fit the current `deploy/kubernetes/base` plus `deploy/kubernetes/overlays/*` structure.

## Proposed Design

### 1. Startup and tracing initialization visibility

`cmd/manager/app.go` and `internal/observability/tracing.go` will be extended so the controlplane can report its effective tracing state in a structured and operator-friendly way.

The implementation will:

- Keep the existing `config.Config -> observability.TracingConfig` flow.
- Normalize tracing config into a form suitable for both initialization and logging.
- Emit an explicit startup log after tracing configuration is resolved, including:
  - whether tracing is enabled
  - effective OTLP endpoint
  - whether insecure transport is enabled
  - effective sampler ratio
  - whether request headers are configured, without logging sensitive header values
- Preserve current disabled behavior as a no-op shutdown path.

The implementation should prefer a small helper or summary function over open-coded log field assembly in multiple places.

### 2. Higher-value controlplane span attributes

This batch does not aim to instrument every code path. It targets the existing highest-value controlplane spans and raises their diagnostic value.

The target areas are:

- `internal/controller/leader_runner.go`
- `internal/controller/syncer.go`
- `internal/infrastructure/reconciler.go`
- `internal/admin/tracing.go` only if small attribute additions materially improve trace usefulness

The implementation will preserve current span names and tracer identities where possible. It will add attributes that help answer practical operational questions such as:

- Which reconcile scopes were requested?
- Which reconcile scopes succeeded or failed?
- Was a snapshot actually published or skipped as unchanged?
- How large was the object set being processed?
- How many managed Gateways or eligible dataplane pods were seen?
- Which normalized admin route handled a request?

This batch will avoid:

- span renaming churn without strong value
- introducing cross-package tracing wrappers
- adding speculative low-value attributes

### 3. Explicit tracing-enabled deployment entry point

The repository will add a new Kustomize overlay dedicated to controlplane tracing enablement.

The preferred shape is:

- `deploy/kubernetes/overlays/observability-enabled/`

This overlay should:

- fit the existing overlay naming and `deploy/README.md` structure
- provide an explicit tracing-enabled path without modifying current base defaults
- replace the controlplane config map content with a tracing-enabled configuration file
- leave dataplane behavior unchanged

The tracing-enabled controlplane configuration should:

- enable `tracing.enabled`
- set `tracing.insecure` and `tracing.samplerRatio` to conservative, explicit values suitable for an example profile
- include a clearly replaceable OTLP endpoint value, consistent with existing repository style where operators must edit environment-specific values before apply

The overlay must be renderable with `kubectl kustomize`, even if the operator has not yet deployed a collector.

### 4. Repository-local operator documentation

The repository documentation will be updated in place instead of creating a large new docs tree.

Expected documentation changes:

- `README.md`
  - mention that controlplane tracing is available
  - point readers to the deploy guide for enablement details
- `deploy/README.md`
  - describe the new tracing-enabled overlay
  - explain which tracing settings are controlled by the overlay
  - document a minimal verification workflow
  - document basic troubleshooting guidance for the most likely failure modes
- `deploy/kubernetes/overlays/production/README.md`
  - update wording if needed so existing references to an `observability-enabled` profile match the actual repository layout after this change

The troubleshooting guidance should stay minimal and concrete. It should focus on:

- tracing enabled but endpoint unreachable
- tracing disabled because the wrong overlay or config file was used
- header configuration present but intentionally redacted from logs

## File Impact

The implementation is expected to modify only files inside the `gateway` repository, primarily:

- `cmd/manager/app.go`
- `cmd/manager/app_test.go`
- `internal/observability/tracing.go`
- `internal/observability/tracing_test.go`
- `internal/controller/leader_runner.go`
- `internal/controller/leader_runner_test.go`
- `internal/controller/syncer.go`
- `internal/controller/syncer_test.go`
- `internal/infrastructure/reconciler.go`
- `internal/infrastructure/reconciler_core_test.go`
- optionally `internal/admin/tracing.go` and existing admin tracing tests if a small, focused attribute addition is justified
- `deploy/README.md`
- `deploy/kubernetes/overlays/observability-enabled/` new overlay files
- `deploy/kubernetes/overlays/production/README.md`
- `README.md`

The implementation must not require changes in sibling repositories.

## Risks and Mitigations

### Risk: accidental default-behavior change

If tracing settings are added directly to shared base or production defaults, operators may start exporting traces unexpectedly.

Mitigation:

- keep tracing enablement behind a new explicit overlay
- leave existing base, Kind, kind-conformance, and production defaults unchanged

### Risk: sensitive data leakage through logs

If configured tracing headers are logged verbatim, credentials may leak into startup logs.

Mitigation:

- log only presence/count or key names if needed
- never log header values

### Risk: noisy or unstable span schema

Adding many new attributes or renaming existing spans could make traces harder to compare across versions.

Mitigation:

- preserve current span names where they already exist
- add a small number of operationally meaningful attributes

### Risk: docs and manifests drift apart

The repository already references observability-oriented install profiles. Adding another partial path would increase confusion.

Mitigation:

- make the new overlay concrete and renderable
- update `deploy/README.md` and `production/README.md` together

## Acceptance Criteria

This work is complete only when all of the following are true:

1. A controlplane tracing-enabled Kustomize overlay exists in the repository and renders successfully.
2. Existing install entry points keep their previous defaults unless the new overlay is explicitly used.
3. Controlplane startup emits an explicit, structured tracing status summary without exposing tracing header values.
4. Existing reconcile and snapshot publication tracing has more diagnostic attributes in the highest-value paths, with focused tests proving the new behavior.
5. Repository docs provide a concrete enable/verify/troubleshoot path for controlplane tracing.
6. No sibling repository is modified.

## Verification Plan

The implementation plan derived from this design must include, at minimum, these acceptance commands:

```bash
go test ./cmd/manager ./internal/observability ./internal/controller ./internal/infrastructure
make test
kubectl kustomize deploy/kubernetes/overlays/observability-enabled >/tmp/nantian-gw-observability-enabled.yaml
git diff --check origin/main...HEAD
git -C /root/nantian-gw/dataplane status --short --branch
git -C /root/nantian-gw/dashboard status --short --branch
git -C /root/nantian-gw/website status --short --branch
git -C /root/nantian-gw/proto status --short --branch
git -C /root/nantian-gw/helm-charts status --short --branch
```

If implementation changes the exact package boundaries, the focused `go test` command may expand, but `make test` and the overlay render check remain mandatory.
