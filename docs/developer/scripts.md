# Script Contract

This document records the script entry points in the repository that maintainers and operators call directly.
The goal is to make local reproduction, CI reproduction, and release evidence collection use the same set of commands, rather than writing different workflows in different workflow files.

## Common Conventions

- Scripts derive paths from the repository root, unless the documentation explicitly supports `*_ROOT` or `*_DIR` environment variables.
- Scripts that support `--help` must print usage with exit code `0`.
- Parameter errors use exit code `2`.
- Validation failures, test failures, or security scan failures use exit code `1`.
- `tmp/` is the local temporary artifacts directory and is not committed as long-term evidence.
- `reports/` is a commit-worthy long-term evidence directory and must not be deleted by cleanup scripts by default.
- Run the cheapest validation entry point first, then escalate to Kind, conformance, soak, or release gate.

## Make Entry Points

| Target | Command | Purpose | Exit Code Semantics |
| --- | --- | --- | --- |
| `make proto` | `./scripts/install-proto-tools.sh` + `./scripts/generate-proto.sh` | Generate control plane Go proto and data plane Rust proto | Non-zero on generation failure |
| `make build` | Go build + Cargo workspace build | Build control plane and data plane | Non-zero if any build fails |
| `make test-unit` | Go full unit tests + Cargo workspace unit tests | Minimum repo-level unit tests | Non-zero if any test fails |
| `make test-targeted` | `./scripts/run-targeted-validation.sh` | Select lowest-cost validation based on current diff | Non-zero if selected validation fails |
| `make test-security` | `./scripts/run-security-scans.sh` | Run security scan bundle | Non-zero if any scan fails |
| `make test-dataplane-guardrails` | `./scripts/run-dataplane-guardrails.sh` | Data plane dependency and source guardrails | Non-zero if guardrails fail |
| `make doctor` | `./scripts/doctor.sh` | Check local development tools and kind registry status | Non-zero if required tools are missing |
| `make clean-artifacts` | `./scripts/clean-artifacts.sh` | Safely clean local temporary/build artifacts | Non-zero on parameter or security check failure |

Dashboard validation (`cd dashboard && npm run check`) is not implicitly covered by these Make entry points. Dashboard changes are validated separately with their own build, test, and lint bundle (`npm run check` runs: app-router validation → tests → lint → production build). Core release workflows (`make target`, `.github/workflows/release.yml`) do not trigger dashboard checks. See [`dashboard/README.md`](../../dashboard/README.md) for the dashboard validation bundle and release boundary details.

## Local Environment Check

### `scripts/doctor.sh`

Purpose:

- Check whether local development tools are sufficient to support Go, Rust, Kind, performance, and deployment validation.
- Check whether the `kind-registry` local registry is running, to avoid repeatedly misdiagnosing registry issues as code problems during Kind debugging.

Check items:

- Required: `go`, `cargo`, `rustc`, `kubectl`, `kind`, `docker`.
- Recommended: `perf`, `flamegraph` or `flamegraph.pl`, `kustomize` or `kubectl kustomize`.
- Local state: Check `kind-registry` container when Docker is available.

Environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `STRICT` | `false` | When `true`, recommended check failures also return non-zero |
| `CHECK_LOCAL_REGISTRY` | `true` | When `false`, skip `kind-registry` check |

Artifacts:

- Only outputs terminal status, does not write files.

Exit codes:

- `0`: All required tools present, or only non-strict recommended warnings.
- `1`: Required tools missing; or `STRICT=true` and recommended check failed.
- `2`: Parameter error.

## Local Artifact Cleanup

### `scripts/clean-artifacts.sh`

Purpose:

- Clean local temporary directories and build artifacts, to prevent old artifacts from affecting tests or consuming disk space.
- By default preserves `reports/`, which contains referenceable evidence such as conformance, performance, chaos, and soak results.

Default cleanup:

- `tmp/`
- `target/`
- `dataplane/target/`
- `dashboard/.next/`

Environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `CLEAN_ARTIFACTS_ROOT` | Current repository root | Specify the repository root to clean; can point to a temporary fixture during testing |
| `DRY_RUN` | `false` | When `true`, only print paths that would be deleted |

Security constraints:

- Refuses to clean `/` or empty paths.
- Target directory must contain `Makefile` and `dataplane/Cargo.toml`.
- Neither default nor explicit paths may delete `reports/`.

Exit codes:

- `0`: Cleanup or dry-run completed.
- `2`: Parameter error or security check failure.

## Validation and Test Entry Points

### `scripts/run-targeted-validation.sh`

Purpose:

- Select the lowest-cost validation based on the current diff or explicit file list.
- Output selection reasons and reasons for skipping more expensive validations.

Common environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `PLAN_ONLY` | `false` | When `true`, only print the plan, do not execute |
| `INCLUDE_KIND` | `false` | When `true`, allow selection of Kind-related validations |
| `SKIP_BUILD` | `true` | Default build strategy passed to Kind smoke |
| `BASE_REF` | `HEAD` | Used to collect diff when no file list is provided |

Artifacts:

- No fixed artifacts; downstream commands executed may write to their own directories.

### `tests/e2e/run-kind.sh`

Purpose:

- Run reusable Kind smoke tests, validating deployment, basic traffic, and key cluster behaviors.
- When reusing or creating a cluster, removes `kube-system/kindnet` CPU/memory requests/limits to prevent local CNI quota constraints from contaminating performance and tail latency evidence.

Common environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `SKIP_BUILD` | `false` | When `true`, reuse existing images |
| `SKIP_SMOKE` | `false` | When `true`, only prepare the environment without running smoke tests |
| `RECREATE_CLUSTER` | `false` | When `true`, recreate the Kind cluster |
| `IMAGE_TAG` | Current timestamp | Specify image tag |

Artifacts:

- Primarily uses `tmp/kind/` for registry, CRD, and image tag caches.

### `tests/conformance/run.sh`

Purpose:

- Run the Gateway API conformance harness.

Common environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ALL_FEATURES` | `false` | When `true`, run the full suite for declared features |
| `REPORT_OUTPUT` | `tmp/conformance/...` | Conformance report output path |
| `CONFORMANCE_LOG_PATH` | `tmp/conformance/...` | Harness log output path |
| `SKIP_BUILD` | `false` | Reuse existing images |

Artifacts:

- Written to `tmp/conformance/` by default.
- For long-term reference, archive to `reports/conformance/` via `scripts/archive-conformance-report.sh`.

## Release and Evidence Entry Points

| Script | Purpose | Default Artifacts |
| --- | --- | --- |
| `scripts/run-release-validation.sh` | Release candidate validation entry point | `tmp/conformance/` and downstream validation artifacts |
| `scripts/prepare-release-assets.sh` | Render install manifests, README, and release bundle | Caller-specified output directory |
| `scripts/refresh-release-evidence.sh` | Refresh release evidence summaries and marker blocks | Updated `docs/` and `reports/` summaries |
| `scripts/verify-release-evidence.sh` | Verify release evidence is referenceable | Terminal output |
| `scripts/check-dependabot-alert-triage.sh` | GitHub Dependabot alert triage gate | `tmp/dependabot-alert-triage/latest/` |
| `scripts/run-security-scans.sh` | Security scan bundle; excludes ignored local artifacts such as `.worktrees/`, `tmp/`, `node_modules/`, and build output by default (`OSV_SCAN_EXCLUDES` / `GRYPE_SCAN_EXCLUDES` override) | `tmp/security-scans/latest/` |
| `scripts/run-dataplane-guardrails.sh` | Data plane dependency and source guardrails | `tmp/dataplane-guardrails/latest/` |
| `scripts/run-dataplane-perf-baseline.sh` | Data plane local performance baseline | `tmp/dataplane-perf-baseline/<run-id>/` |
| `scripts/run-dataplane-throughput-baseline.sh` | Data plane throughput report; can aggregate fault/soak evidence via `CHAOS_INPUT_DIR` / `SOAK_INPUT_DIR` | `reports/performance/runs/<run-id>-dataplane-throughput/` |
| `scripts/run-dataplane-reload-bench.sh` | Data plane local microbenchmark covering reload, xDS, TLS, observability, HTTP capacity, and stream tuning | `tmp/dataplane-reload-bench/<run-id>/` |
| `scripts/run-controlplane-status-bench.sh` | Control plane status benchmark | `tmp/controlplane-status-bench/<run-id>/` |
| `scripts/run-kind-a4-baseline.sh` | Kind A4 performance and long-connection/UDP scenario evidence | `reports/performance/runs/<run-id>/` |
| `scripts/run-kind-fault-injection.sh` | Kind fault injection evidence | `reports/chaos/runs/<run-id>/` |
| `scripts/run-kind-soak.sh` | Kind soak evidence | `reports/soak/runs/<run-id>/` |

`scripts/render-release-manifest.sh` supports `--profile <name>`, with currently available values `kind-dev`, `kind-hostnetwork-perf`, `single-cluster-prod`, `multi-replica-prod`, and `observability-enabled`. Release asset packaging uses `RELEASE_INSTALL_PROFILE=single-cluster-prod` by default; to temporarily generate Kind-scoped or hostNetwork stress test manifests, override this environment variable explicitly.

`scripts/prepare-release-assets.sh` requires release assets to have image digest pins. The formal release workflow passes digests obtained after build/push via `RELEASE_CONTROLPLANE_DIGEST` and `RELEASE_DATAPLANE_DIGEST`; when packaging locally, you can directly pass image references in the `image@sha256:<digest>` format. The script generates `image-digests.txt`, `release-evidence.txt`, and `RELEASE_NOTES.md` categorized by breaking changes, upgrade notes, security, conformance, performance, and known issues; `release-evidence.txt` records the conformance / performance / chaos / soak metadata selected by `refresh-release-evidence.sh --check-only` before this packaging.

`scripts/run-release-validation.sh` by default only executes the release candidate's build, test, Kind, targeted E2E, and conformance baseline. Production candidates that need to include archived conformance / performance / chaos / soak evidence in the same entry point should set `REQUIRE_RELEASE_EVIDENCE=true`; the script then calls `scripts/refresh-release-evidence.sh --check-only` and `scripts/check-evidence-reference-alignment.sh` after other validations pass. Use `RELEASE_EVIDENCE_CANDIDATE` to override the candidate commit, `RELEASE_EVIDENCE_ALLOW_COMMITS` to pass space-separated approved evidence windows, or `RELEASE_EVIDENCE_CONFORMANCE_RUN`, `RELEASE_EVIDENCE_PERFORMANCE_RUN`, `RELEASE_EVIDENCE_CHAOS_RUN`, `RELEASE_EVIDENCE_SOAK_RUN` to explicitly specify archive runs. To only verify evidence windows without requiring current document summaries to be refreshed, set `SKIP_RELEASE_EVIDENCE_ALIGNMENT=true`.

`scripts/verify-release-evidence.sh` is a hard gate for release evidence, not just a metadata freshness check. Beyond requiring conformance / performance / chaos / soak evidence to point to the candidate commit or an explicitly allowed window, performance, chaos, and soak evidence must by default record `code_tree_state=clean`; old evidence missing this field or from a dirty code tree can only pass explicit risk acceptance via `--allow-dirty-performance-code-tree` / `--allow-dirty-chaos-code-tree` / `--allow-dirty-soak-code-tree`. Performance evidence must also include a `throughput-report.json` in the same directory, and `coverage.missing_protocols`, `coverage.missing_scenarios`, `reload.live_traffic.missing_protocols`, and `reload.live_traffic.missing_mutations` must all be empty arrays; additionally, `source-kind-a4/slo-gate.json` or the direct Kind A4 `slo-gate.json` must record a total `status=pass` with all profiles `status=pass`. Chaos evidence must include `conclusions/summary.json` with `release_gate_status=pass`, and `traffic/summary.json` must record `slo_gate.status=pass`; soak evidence must record `duration_seconds>=86400` in `metadata.txt`, and `traffic/summary.json` must record `slo_gate.status=pass`. `scripts/refresh-release-evidence.sh` automatically scans old `*kind-a4*` archives as well as new-style `*a4-profiles` / `*a4-scenarios` archives when discovering performance evidence, and skips performance / chaos / soak archives that do not meet these conditions, preventing dirty, incomplete coverage, or source SLO-failed A4, `10m` pilot, incomplete / SLO-failed fault injection, or SLO-failed soak from being mistakenly used as release-gating materials.

`scripts/run-kind-fault-injection.sh` in release-gate mode requires all four conclusions: `controlplane-leader-switch`, `dataplane-pod-restart`, `node-drain`, and `apiserver-watch-disruption`. When `SKIP_NODE_DRAIN=false` is set and the current Kind cluster has no non-control-plane Ready nodes, the script refreshes the Kind stack with `RECREATE_CLUSTER=true KIND_WORKER_NODES=2 SKIP_BUILD=true`; two workers is the default because node drain requires at least one schedulable replica outside the drained worker to satisfy PDB. `tests/e2e/run-kind.sh` also supports `KIND_WORKER_NODES=<n>` for rendering additional worker nodes.
In the same release-gate mode, before starting sustained HTTP traffic, the smoke `Deployment/echo` is scaled to `FAULT_HTTP_BACKEND_REPLICAS=2` and ready backend Pods are confirmed to be distributed across at least two nodes, preventing node drain from evicting a single-replica backend and being misrecorded as a gateway traffic SLO failure. If the scheduler places both echo replicas on the same worker, the script creates a temporary `app=echo` / `nantian.dev/fault-traffic-guard=true` Pod on the missing worker, serving as the backend endpoint guard for this fault evidence.
In the `dataplane-pod-restart` scenario, after deleting one dataplane Pod, the script waits for the Deployment rollout and the number of non-terminating ready dataplane Pods to return to the desired replica count before recording recovered and proceeding to the node drain scenario, preventing the window during which the replacement is not yet ready from being incorrectly recorded as recovery complete.

## CI Constraints

- CI should prefer calling `make` or `scripts/*`, avoiding copying test logic inside workflows.
- If CI needs a new validation level, prefer extending existing script parameters or modes.
- New scripts must document their purpose, parameters, key environment variables, artifact directories, and exit codes.
- New scripts must at minimum pass `bash -n`; complex scripts should add fixture tests in `tests/scripts/`.

## Script Classification and Addition Rules

`scripts/script-inventory.yaml` is the machine-readable source of script classification. When adding, deleting, or renaming `scripts/*.sh` or `scripts/lib/*.sh`, the file must be updated accordingly, and must be run:

```bash
./scripts/check-script-inventory.sh
```

Current classifications:

| Class | Meaning |
| --- | --- |
| `stable` | Stable entry points called directly by operators, CI, Makefile, release, or documentation. |
| `check` | Static contract or consistency check entry points. |
| `audit` | Security, dependency, cluster, or manifest audit entry points. |
| `evidence` | Entry points that generate conformance, performance, chaos, soak, or release evidence. |
| `internal` | Helpers under `scripts/lib/`, not called directly as user entry points. |
| `candidate` | Currently stable, but suitable for subsequent helper extraction or dispatcher consolidation. |
| `deprecated` | Retained compatibility wrappers; must document replacement command and removal window. |

Before adding a top-level script, confirm:

- Can existing script parameters or environment variables be extended instead.
- Whether it is just duplicating bootstrap logic and should instead go in `scripts/lib/common.sh`.
- Whether a long-term stable path is needed; if it is a one-time development aid, prefer placing it in documentation commands or temporary `tmp/`.
- Whether lightweight self-tests should be added in `tests/scripts/`.

## Current Script Inventory

| Script | Class | Owner | Purpose |
| --- | --- | --- | --- |
| `scripts/archive-conformance-report.sh` | `evidence` | `release` | Archive a Gateway API conformance report into the reports/conformance metadata layout. |
| `scripts/audit-controlplane-rbac.sh` | `audit` | `security` | Compare the controlplane RBAC manifests with the required RBAC baseline. |
| `scripts/audit-gateway-api-bundle.sh` | `audit` | `gateway-api` | Inspect Gateway API CRD bundle and GatewayClass support status in a cluster. |
| `scripts/audit-vendored-upstream.sh` | `audit` | `dataplane` | Verify vendored Rust proxy/runtime sources are not reintroduced. |
| `scripts/build-kind-runtime-images-local.sh` | `candidate` | `kind` | Build local runtime images for Kind workflows with the production dataplane allocator default. |
| `scripts/build-wasm-examples.sh` | `stable` | `dataplane` | Build Wasm example plugins (hello-plugin, auth-plugin) and prebuilt modules for wasm32-wasi. |
| `scripts/check-community-governance-contract.sh` | `check` | `docs` | Verify public community governance docs and policy entrypoints stay aligned. |
| `scripts/check-dependabot-alert-triage.sh` | `audit` | `security` | Verify open GitHub Dependabot alerts have repository triage evidence. |
| `scripts/check-evidence-reference-alignment.sh` | `check` | `release` | Verify release evidence references point at current archived reports. |
| `scripts/check-gateway-api-version-alignment.sh` | `check` | `gateway-api` | Verify Gateway API version constants and docs stay aligned. |
| `scripts/check-admin-api-drift.sh` | `check` | `admin-api` | Verify admin API route contract matches surface doc and user docs. |
| `scripts/check-metrics-cardinality-contract.sh` | `check` | `observability` | Verify metrics cardinality docs, Prometheus docs, and Grafana golden signal query semantics stay aligned. |
| `scripts/check-rust-unwraps.sh` | `check` | `dataplane` | Guard against disallowed unwrap or expect usage in Rust production paths. |
| `scripts/check-script-inventory.sh` | `check` | `developer-experience` | Verify every script and helper has inventory metadata and documentation coverage. |
| `scripts/check-stream-route-test-coverage.sh` | `check` | `gateway-api` | Verify stream Route supplemental coverage remains tested and documented. |
| `scripts/clean-artifacts.sh` | `stable` | `developer-experience` | Safely clean local temporary and build artifacts while preserving reports. |
| `scripts/collect-admin-snapshots.sh` | `evidence` | `observability` | Collect controlplane and dataplane admin snapshots and optional metrics. |
| `scripts/compare-performance-runs.sh` | `evidence` | `performance` | Compare performance run summaries with percentage thresholds and absolute noise floors. |
| `scripts/doctor.sh` | `stable` | `developer-experience` | Check required and recommended local development tools. |
| `scripts/generate-proto.sh` | `stable` | `proto` | Generate controlplane Go proto bindings from proto definitions. |
| `scripts/install-proto-tools.sh` | `stable` | `proto` | Install Go protobuf generator tools used by make proto. |
| `scripts/lib/conformance-report.sh` | `internal` | `conformance` | Shared conformance report parsing helpers. |
| `scripts/lib/common.sh` | `internal` | `developer-experience` | Shared shell bootstrap, logging, validation, and safe filesystem helpers. |
| `scripts/lib/kind-evidence.sh` | `internal` | `kind` | Shared Kind evidence collection helpers. |
| `scripts/lib/kind-image-sync.sh` | `internal` | `kind` | Shared Kind/local-registry image synchronization helpers. |
| `scripts/lib/performance-common.sh` | `internal` | `performance` | Shared performance benchmark helpers (log, require_command, write_metadata). |
| `scripts/prepare-canary-gatewayclass.sh` | `stable` | `release` | Prepare a canary GatewayClass from the stable class. |
| `scripts/prepare-release-assets.sh` | `stable` | `release` | Package release install manifests, checksums, and release docs. |
| `scripts/publish-conformance-reports.sh` | `evidence` | `conformance` | Publish archived conformance reports to the conformance reports branch. |
| `scripts/refresh-release-evidence.sh` | `evidence` | `release` | Refresh release evidence summaries and marker-managed docs. |
| `scripts/render-release-manifest.sh` | `stable` | `release` | Render release install manifests with concrete image references. |
| `scripts/rollback-canary-gatewayclass.sh` | `stable` | `release` | Roll Gateways back from a canary GatewayClass. |
| `scripts/run-controlplane-status-bench.sh` | `evidence` | `controlplane` | Run local controlplane status benchmark evidence. |
| `scripts/run-dataplane-guardrails.sh` | `audit` | `dataplane` | Run dataplane dependency and runtime source guardrails. |
| `scripts/run-dataplane-perf-baseline.sh` | `evidence` | `performance` | Run local dataplane performance baseline evidence. |
| `scripts/run-dataplane-throughput-baseline.sh` | `evidence` | `performance` | Generate dataplane throughput baseline reports from collected evidence. |
| `scripts/run-dataplane-reload-bench.sh` | `evidence` | `dataplane` | Run local dataplane microbenchmarks for reload, xDS apply, TLS rotation, observability, HTTP capacity, and stream tuning. |
| `scripts/run-kind-a4-baseline.sh` | `evidence` | `performance` | Run Kind A4 HTTP/gRPC/WebSocket/SSE/MCP/TCP/UDP performance evidence. |
| `scripts/run-kind-fault-injection.sh` | `evidence` | `resilience` | Run Kind fault injection evidence. |
| `scripts/run-kind-soak.sh` | `evidence` | `resilience` | Run Kind soak evidence. |
| `scripts/run-release-validation.sh` | `stable` | `release` | Run the release candidate validation baseline. |
| `scripts/run-security-scans.sh` | `audit` | `security` | Run the repository security scan bundle, excluding ignored local artifacts by default. |
| `scripts/run-skew-validation.sh` | `stable` | `proto` | Validate proto and adjacent-release skew compatibility. |
| `scripts/run-targeted-validation.sh` | `stable` | `developer-experience` | Select the cheapest validation commands for the current diff. |
| `scripts/update-gateway-api-support.sh` | `stable` | `gateway-api` | Update or check generated Gateway API support matrix sections. |
| `scripts/verify-release-evidence.sh` | `evidence` | `release` | Verify release evidence directories and metadata are usable. |
