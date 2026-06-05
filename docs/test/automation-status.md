# Nantian Gateway Automation Status and Maintenance Rules

This document answers three maintenance-period questions:

1. Which tests can already be run automatically.
2. Which tests are in the plan but have not yet been incorporated into the standard release baseline.
3. After new capabilities are added, where to place test assets to prevent the plan from gradually diverging from the actual repository.

Recommended to use in conjunction with the following documents:

- [Test Plan](./plan.md)
- [Test Case Matrix](./case-matrix.md)
- [Test Execution Checklist](./checklist.md)
- [Latest Automation Baseline Record](./latest-baseline.md)
- [Security Regression Execution Template](./security-regression.md)
- [Performance Baseline Execution Template](./performance-baseline.md)

## 1. Automation Tiers

### 1.1 `A1` Release-Level One-Click Automation

This tier of tests should satisfy two conditions:

- A stable entry point already exists in the repository.
- Can be repeatedly executed as a standard baseline for release gating.

Current entry point:

```bash
./scripts/run-release-validation.sh
```

Current coverage:

- `make proto`
- `./scripts/run-skew-validation.sh`(invoked within release baseline using compatibility-only + mixed-version mode)
- `cd controlplane && go test ./...`
- `cargo test --manifest-path dataplane/Cargo.toml --workspace`
- `./tests/e2e/run-kind.sh`
- `./scripts/update-gateway-api-support.sh --check`
- `./tests/e2e/validate-status-surfaces.sh`
- `./tests/e2e/validate-admin-token-rotation.sh`
- `./tests/e2e/validate-backend-protocols.sh`
- `./tests/e2e/validate-gateway-cross-namespace-certs.sh`
- `./tests/e2e/validate-grpc-reference-grants.sh`
- `./tests/e2e/validate-reference-grants.sh`
- `./tests/e2e/validate-upstream-behavior.sh`
- `./tests/e2e/validate-session-persistence.sh`
- `./tests/e2e/validate-http-security.sh`
- `./tests/e2e/validate-mesh-frontends.sh`
- `ALL_FEATURES=true ./tests/conformance/run.sh`

Maintenance requirements:

- When adding release-blocking automated tests, prioritize integrating them into `scripts/run-release-validation.sh`.
- When this entry point fails, you must be able to quickly identify from the logs whether it is a proto, dual-end test, Kind, specialized E2E, or conformance failure.
### 1.2 `A2` In-Repo Automated but Not in Release Baseline

These tests already have scripts or stable commands but have not yet been incorporated into the standard release entry point.
There are currently no in-repo automated scripts that must be run separately.

If a specialized test that has repeatedly been a release blocker but has not yet been integrated into `A1` emerges again, it should first be placed here and then evaluated for promotion to the default baseline.

### 1.3 `A3` Semi-Automated, Repeatable

These tests already have stable commands but still rely on manual judgment to determine whether results are correct.

Typical content:

- Local control plane integrated debugging
- Local data plane integrated debugging
- Admin API view cross-referencing
- Log, metrics, admin API tri-party evidence alignment
- Canary `GatewayClass` preparation and rollback scripts

Main entry points:

```bash
cd controlplane && go run ./cmd/manager -config ../configs/controlplane/config.yaml

cargo run --manifest-path dataplane/Cargo.toml -p aeg-app -- \
  --config ../configs/dataplane/config.yaml

curl -fsS http://127.0.0.1:18081/v1/summary | jq
curl -fsS http://127.0.0.1:19080/v1/summary | jq
./scripts/prepare-canary-gatewayclass.sh
./scripts/rollback-canary-gatewayclass.sh
```

Maintenance requirements:

- Documentation must clearly specify which interfaces and fields to observe — it cannot just say "do an integrated debug".
- If a particular manual check repeatedly becomes a release blocker, consider scripting it.

### 1.4 `A4` Tests Requiring Specialized Environments

These tests typically cannot be casually re-run in full on development machines, but must be retained in long-term maintenance plans.

Includes:

- Performance baseline
- 24h/72h soak
- Header injection / CRLF deep analysis
- Slow body / idle timeout / large-scale slowloris and connection flood
- Capacity and saturation points
- Chaos / fault injection
- Production canary and rollback drills

Common tools:

- `fortio`
- `wrk2`
- `vegeta`
- `h2load`
- `ghz`
- `websocat`
- `tc`
- `osv-scanner`
- `trivy` / `grype`

Lowest-cost alternative entry points added in the repository:

```bash
./scripts/run-dataplane-reload-bench.sh
./tests/e2e/validate-http-concurrency.sh
./scripts/run-kind-a4-baseline.sh
./scripts/run-kind-fault-injection.sh
./scripts/run-kind-soak.sh
```

Maintenance requirements:

- Must document environment prerequisites, such as replica count, node specifications, load test targets, and observation metrics.
- Must record acceptance thresholds and evidence artifacts to avoid inconsistent standards each re-run.
- `validate-http-concurrency.sh` is a lightweight `A4` pre-check, not equivalent to a full performance baseline, and should not be directly merged into the default release baseline.
- `run-dataplane-reload-bench.sh` is used for locally fixed evidence baselines for dataplane reload / TLS asset rotation / xDS apply / route selection / request metadata / snapshot read / runtime index rebuild / observability traffic stats / HTTP capacity matrix / TCP-UDP stream tuning. Defaults to dataplane `release profile` collection, supports `ALLOCATOR=system|mimalloc|jemalloc` for allocator comparison, and bakes script request values together with the actual allocator exposed by `bench.json` into archived artifacts, prioritized over bringing up kind for heavier regression.
- `run-kind-a4-baseline.sh`, `run-kind-fault-injection.sh`, and `run-kind-soak.sh` generate long-term stored kind evidence; `2026-05-14` has archived one full round of Kind fault release gate, but this still cannot substitute for a truly completed round of `24h` soak and production-approximate environment node drain / apiserver jitter specializations.

## 2. Current Standard Entry Point for "Running a Full Automation Test Pass"

If the goal is to run a complete pass of the core regression tests already automated in the current repository, the recommended order is:

```bash
./scripts/run-release-validation.sh
```

Explanation:

- This command already covers the current `A1` release-level automation baseline, including proto skew / compatibility checks, support matrix consistency checks, condition/status/admin surface validation, and mesh frontend specialization.

After this set of commands completes, it can be concluded that:

- The mainline regression of "already automated and directly executable" tests in the current repository has been covered.
- But this does not mean that `A4` performance, security, soak, chaos, and production canary tests have been completed.

## 3. Capabilities Not Yet Fully Automated into the Baseline

The following capabilities, even if `P0/P1` in the plan, currently cannot be claimed as "already having a unified script that automatically covers them":

- Admin API snapshot comparison and log/metrics/admin evidence alignment during local integrated debugging
- Header injection / CRLF deep analysis
- Slow body, prolonged idle, and higher-intensity slowloris / connection flood
- HTTP/HTTPS/gRPC long-term performance baseline and 24h/72h soak
- Configuration scale testing, capacity inflection points, FD/memory saturation points
- Control plane leader switch, status storm, fault injection
- Production environment canary rollout and rollback drills

These items should not be removed from the plan, but should be explicitly marked as:

- Planned
- Not yet fully automated
- Requiring specialized environment or manual judgment

## 4. Evidence Retention Requirements

During maintenance, do not just note "ran it" — standardize the evidence types.

`A1/A2` At minimum, retain:

- Execution command
- Exit code
- Console output summary
- `tmp/conformance/report-v1.5.1.yaml`, or the `REPORT_OUTPUT` explicitly configured for this run
- `tmp/conformance/run.log`

`A3` At minimum, retain:

- Control plane `/v1/summary`
- Control plane `/v1/snapshot-sync`
- Control plane `/v1/nodes`
- Data plane `/v1/summary`
- Data plane `/v1/snapshot`
- Key access log / metrics excerpts

`A4` At minimum, retain:

- Load test parameters
- Environment specifications
- Metric screenshots or exports
- Threshold judgment conclusions
- Rollback or recovery records for anomalies

## 5. Rules for Integrating New Features into the Test System

When adding new features or making major changes, supplement test assets in the following order:

1. First, add `L0/L1` unit or module tests.
2. If the change involves real Kubernetes resource semantics, then add `tests/e2e/*.sh` or Kind-level validation.
3. If it is a release-blocking capability, merge the script into `scripts/run-release-validation.sh`.
4. If automation is temporarily not possible, still explicitly classify it as `A3` or `A4` in the [Test Plan](./plan.md) and [Test Execution Checklist](./checklist.md).
5. If the feature changes the publicly declared scope, also synchronously update the [Gateway API Support Matrix](../gateway-api-support.md).

## 6. Current Maintenance Recommendations

- Do not add "ideal test items" that exist only in documentation without specifying which tier (`A1/A2/A3/A4`) they belong to.
- Any specialized test that has been a stable release blocker 2 or more times should be prioritized for scripting and integrated into `A1` or `A2`.
- Before each release, confirm that the standard entry point still covers the currently declared supported scope, rather than defaulting to the previous version's script.
