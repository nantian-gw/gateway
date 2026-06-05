# Release, Canary, and Rollback Runbook

This document is intended for maintainers who plan to deploy Aether Gateway to long-term environments. It does not repeat explanations of all deployment parameters; for configuration details, please refer to [Production Operations](operations.md) and [Gateway API Support Matrix](../gateway-api-support.md).

## 1. Objective

This repository recommends splitting releases into two parts:

- Control plane release
- Data plane release

Both can use the same version number, but should not default to going live with "same time, same entry point, same Service direct replacement."

## 2. Pre-Release Checklist

Before starting canary deployment, at least confirm the following conditions are met:

- The current version's release summary has been written to [`CHANGELOG.md`](../../CHANGELOG.md).
- Upgrade, rollback, skew, admin API, or deployment default value changes involved in the current version have been written to [Compatibility Notes](compatibility-notes.md).
- The capability boundaries of the current version have been updated in [Gateway API Support Matrix](../gateway-api-support.md).
- Changes have completed minimum verification, refer to [Test Plan](../test/plan.md).
- If changes involve controlplane / dataplane / proto upgrade contracts, the following has been completed:

```bash
./scripts/run-skew-validation.sh
```

- If changes involve Gateway API semantics, the following has been completed:

```bash
ALL_FEATURES=true ./tests/conformance/run.sh
```

- If this release candidate references archived `full-suite`, `A4`, chaos, or soak evidence, explicitly verify whether the evidence points to the current candidate commit or falls within the approved commit window:

```bash
./scripts/verify-release-evidence.sh \
  --candidate <candidate-commit> \
  --conformance reports/conformance/runs/<full-suite-id>/metadata.yaml \
  --performance reports/performance/runs/<a4-run-id>/metadata.txt \
  --chaos reports/chaos/runs/<fault-run-id>/metadata.txt \
  --soak reports/soak/runs/<soak-run-id>/metadata.txt
```

`performance`, `chaos`, and `soak` metadata must record `code_tree_state=clean`. `performance` evidence must also include `throughput-report.json` in the same directory, and `coverage.missing_protocols`, `coverage.missing_scenarios`, `reload.live_traffic.missing_protocols`, and `reload.live_traffic.missing_mutations` must all be empty arrays; additionally, `source-kind-a4/slo-gate.json` or direct kind A4 `slo-gate.json` must record overall `status=pass` and all profile `status=pass`. If you must reuse evidence from a historical dirty working tree or old scripts lacking `code_tree_state`, you must explicitly add `--allow-dirty-performance-code-tree`, `--allow-dirty-chaos-code-tree`, or `--allow-dirty-soak-code-tree` to the command, and record this `risk-accepted` in the release notes; the default release path must not reference such evidence.

If you want to refresh summaries like `docs/test/latest-baseline.md`, `reports/performance/README.md`, `reports/chaos/README.md`, `reports/soak/README.md` together with this set of archives after verification passes, you can directly use the `verify + summary refresh` wrapper entry point:

```bash
./scripts/refresh-release-evidence.sh \
  --candidate 66c7bef \
  --allow-commit af4a7d3
```

The auto-discovery mode scans old `*kind-a4*` naming and new-style `*a4-profiles` / `*a4-scenarios` naming for performance archives, but only selects evidence with `code_tree_state=clean`, complete `throughput-report.json` coverage, and source SLO gate `pass`; soak must also satisfy `duration_seconds>=86400` and `traffic/summary.json.slo_gate.status=pass`. If you want to reference historical evidence that does not meet these conditions, do not rely on auto-discovery; explicitly pass the specific run and record the risk acceptance in `verify-release-evidence.sh`.

`chaos` evidence must also simultaneously satisfy both `conclusions/summary.json` `release_gate_status=pass` and `traffic/summary.json` `slo_gate.status=pass`; archives where only fault scenario conclusions pass but continuous traffic SLO fails cannot serve as default release evidence.

If you want the main entry point `./scripts/run-release-validation.sh` to also enforce the same evidence gate after all regular validations, you can set:

```bash
REQUIRE_RELEASE_EVIDENCE=true \
RELEASE_EVIDENCE_ALLOW_COMMITS="<approved-window-commit>" \
./scripts/run-release-validation.sh
```

`REQUIRE_RELEASE_EVIDENCE=true` reuses `scripts/refresh-release-evidence.sh --check-only` auto-discovery and full evidence gate, and appends `scripts/check-evidence-reference-alignment.sh` by default. When explicit archives need to be specified, you can set `RELEASE_EVIDENCE_CONFORMANCE_RUN`, `RELEASE_EVIDENCE_PERFORMANCE_RUN`, `RELEASE_EVIDENCE_CHAOS_RUN`, and `RELEASE_EVIDENCE_SOAK_RUN`; when you need to verify evidence first and refresh document summaries later, you can temporarily set `SKIP_RELEASE_EVIDENCE_ALIGNMENT=true`, but the alignment check must still pass before formal packaging.

After refresh completes, the `CHANGELOG.md` release summary evidence block should also point to the same set of runs. `scripts/prepare-release-assets.sh` will re-execute `scripts/refresh-release-evidence.sh --check-only` and `scripts/check-evidence-reference-alignment.sh` before actually packaging release assets; if the candidate commit does not match the archived evidence, or if changelog, README summaries, and archived evidence are inconsistent, release packaging will be blocked. On successful packaging, the release bundle will include `release-evidence.txt`, recording the current candidate commit, approved evidence window, and the four types of metadata paths that actually passed checks.

If this release allows reusing adjacent and reviewed evidence windows, rather than requiring `performance` / `chaos` / `soak` to all share the exact same SHA as the candidate commit, you can explicitly set before executing `scripts/prepare-release-assets.sh`:

```bash
RELEASE_EVIDENCE_ALLOW_COMMITS="<approved-window-commit>"
```

If `A4`, chaos, or soak evidence temporarily reuses a commit window that is "adjacent to and reviewed with the candidate commit" rather than the exact same commit, the allowed window must be explicitly written into the command, for example:

```bash
./scripts/verify-release-evidence.sh \
  --candidate <candidate-commit> \
  --allow-commit <approved-window-commit> \
  --conformance reports/conformance/runs/<full-suite-id>/metadata.yaml \
  --performance reports/performance/runs/<a4-run-id>/metadata.txt \
  --chaos reports/chaos/runs/<fault-run-id>/metadata.txt \
  --soak reports/soak/runs/<soak-run-id>/metadata.txt
```

- If the data plane has `sessionPersistence` enabled, a stable shared secret has been configured, not a temporary in-process key.
- Control plane and data plane admin authentication, metrics, TLS/mTLS, and NetworkPolicy have been configured as required by [Production Operations](operations.md).
- If this release or installation package retains the dashboard, the `aether-gateway-dashboard` image has been separately built, scanned, and pinned; the current core release workflow only publishes controlplane / dataplane images.
- Can clearly answer the following two questions:
  - Which Gateways / namespaces / domains are affected by this canary deployment?
  - How to switch back within 5 minutes if an anomaly occurs?

## 3. Recommended Release Order

### 3.1 Internal Verification

First complete the minimum closed-loop verification within the repository:

```bash
./scripts/run-skew-validation.sh
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
./tests/e2e/run-kind.sh
ALL_FEATURES=true ./tests/conformance/run.sh
```

If you need to archive the full-suite report:

```bash
scripts/archive-conformance-report.sh <report-id> tmp/conformance/report-v1.5.1.yaml
```

### 3.2 Control Plane Canary

This repository recommends using an independent `GatewayClass` for control plane canary deployment, for example:

- Stable: `aether`
- Canary: `aether-canary`

Approach:

1. Deploy the canary control plane first.
2. Create or switch a small number of `Gateway`s to the canary `GatewayClass`.
3. Observe `Accepted`, `Programmed`, `ResolvedRefs`, node ACKs, and snapshot version progression.
4. Expand `Gateway` coverage after confirming no issues.

The benefits of this approach are:

- Does not affect the entire cluster from the start.
- Rollback only requires switching `GatewayClass` back to stable or stopping canary control plane takeover.
- Status, logs, and snapshots are easier to isolate and troubleshoot by version.

### 3.3 Data Plane Canary

This repository does not recommend mixing old and new data plane Pods directly behind the same entry Service for version canary deployment.

A safer approach is:

1. Use an independent canary `Gateway` / Service / exposed address.
2. First receive synthetic traffic or shadow traffic.
3. Then receive a single service, low-risk namespace, or low-risk domain.
4. Finally expand the scope.

For gateway implementation canary deployment itself, the following is more recommended:

- `GatewayClass` dimension splitting
- Independent `Gateway` / Service / LoadBalancer
- Independent observability metrics and logs

Rather than directly mixing different versions into the same data plane backend pool.

## 4. Recommended Canary Stages

### Stage 0: Internal Environment

- Only verify test domains and test namespaces
- Only receive synthetic traffic
- Focus on deployment correctness, snapshot synchronization, ACKs, and basic traffic

### Stage 1: Pre-Production Shadow

- Production traffic is only mirrored; canary results are not directly returned to users
- Compare status codes, latency, and error types between stable and canary

### Stage 2: Low-Risk Services

- 1 namespace
- 1 or a small number of Gateways
- 1 low-risk domain or service entry point

### Stage 3: Expand Scope

- Gradually expand by namespace, domain, and Gateway count
- It is not recommended to mix traffic by Pod ratio at first; instead, expand by entry object boundaries first

### Stage 4: Full Rollout

- Retain a rollback window after full switchover is complete
- It is recommended to observe logs, metrics, and status stability for at least 24 hours
- Long-term stability evidence for release candidates should use `REQUIRE_24H=true ./scripts/run-kind-soak.sh` or equivalent pre-production entry point, to avoid short pilot runs being mistakenly archived as `24h` soak

## 5. Rollout Thresholds

At each stage, at least check the following metrics:

### 5.1 Business Metrics

- HTTP `2xx / 4xx / 5xx` ratios
- gRPC `UNAVAILABLE` / `DEADLINE_EXCEEDED`
- WebSocket connection success rate
- Request timeout rate

### 5.2 Performance Metrics

- `p95` / `p99`
- TLS handshake latency
- upstream connect latency
- Configuration propagation latency
- Data plane reload or hot-reload time

### 5.3 Stability Metrics

- pod restart
- Continuous memory growth
- Continuous FD growth
- Abnormal increase in retry rate
- Abnormal increase in backend reset / connect failure

### 5.4 Control Plane Metrics

- reconcile error rate
- Status update anomalies
- Abnormal ratio of `Accepted` / `Programmed` / `ResolvedRefs`
- Node ACK stuck or snapshot version not progressing

## 5.5 Manual Confirmation Points

Before each round of rollout, retain at least one explicit manual confirmation; do not proceed solely based on "the script didn't error":

- Stage 0 -> Stage 1:
  - The relationship between canary `GatewayClass` and target `Gateway` is correct
  - `Accepted` / `Programmed` / `ResolvedRefs` are all stable
  - `/v1/summary`, `/v1/nodes`, `/v1/snapshot` have no version jitter
- Stage 1 -> Stage 2:
  - Status codes, error types, and latency between stable / canary show no significant deviation
  - Synthetic / shadow traffic has not exposed configuration ambiguity
- Stage 2 -> Stage 3:
  - Metrics from the low-risk service stage meet rollout thresholds
  - No new `Gateway` / `Route` abnormal states
- Stage 3 -> Stage 4:
  - No new error spikes within the past observation window
  - The rollback entry point is still available, and the responsible person has confirmed they can switch back within 5 minutes

## 6. Rollback Conditions

It is recommended to predefine a set of hard thresholds rather than judging on the spot:

- `5xx` significantly higher than baseline
- `p99` significantly higher than baseline
- gRPC deadline / unavailable errors significantly increased
- WebSocket connection success rate decreased
- `Gateway` / `Route` status anomalies increased
- Control plane reconcile errors continuously increasing
- Data plane memory or FD continuously leaking

## 7. Rollback Actions

The following actions are recommended in priority order:

1. Stop rollout, switch the entry point back to stable.
2. For control plane canary, remove or switch back the canary `GatewayClass`.
3. For data plane canary, switch canary Service / LoadBalancer / DNS weight back to 0.
4. Execute image rollback or Helm rollback if necessary.
5. Preserve the scene:
   - Control plane logs
   - Data plane logs
   - `/v1/summary`
   - `/v1/snapshot`
   - `/v1/nodes`
   - Conformance / smoke / stress test report paths

Do not immediately delete the cluster or clear the scene.

This repository already provides canary `GatewayClass` and rollback script entry points:

- `./scripts/prepare-canary-gatewayclass.sh`
- `./scripts/rollback-canary-gatewayclass.sh`

It is recommended to first prepare the canary `GatewayClass`, then switch target `Gateway`s to the canary class in batches; during rollback, prioritize using the rollback script to batch switch `Gateway.spec.gatewayClassName` back to the stable class, then delete the canary class as appropriate.

## 7.1 Post-Rollback Verification Checklist

After executing rollback, do not rely solely on script exit codes; at least perform the following checks:

- Target `Gateway` has been switched back to stable `GatewayClass`
- `Accepted`, `Programmed`, `ResolvedRefs` restored to pre-rollback baseline
- `/v1/summary`, `/v1/nodes`, `/v1/snapshot` show no canary version continuing to progress
- Service-side `5xx`, gRPC deadline/unavailable, WS connection success rate restored to baseline
- Canary entry point, Service, LoadBalancer, DNS, or traffic weight no longer handles production requests
- Logs, admin, metrics, and script output within the rollback window have been archived

## 8. Minimum Troubleshooting Checklist for Anomalies

When issues occur, at least check in the following order:

1. `kubectl describe gateway` and related Route conditions
2. Control plane logs
3. Data plane logs
4. Control plane `/v1/summary`, `/v1/nodes`
5. Data plane `/v1/summary`, `/v1/snapshot`
6. Whether node ACK is stalled
7. Whether snapshot version is oscillating

## 9. Post-Release Wrap-Up

It is recommended to retain the following artifacts for each formal release:

- Conformance report for the current version
- Key smoke / e2e results
- Metric screenshots or exports within the release window
- Rollback strategy and actual effective version records
- Image digest, SBOM, and attestation evidence
- If anomalies occurred, retain post-mortem conclusions and fix commits

If this release includes Gateway API semantic changes, it is recommended to synchronously update:

- [Gateway API Support Matrix](../gateway-api-support.md)
- [Test Plan](../test/plan.md)
- [Production Operations](operations.md)
- [Compatibility Notes](compatibility-notes.md)

## 10. Supply Chain Artifact Verification

The release workflow now additionally produces the following supply chain materials:

- `image-digests.txt`
- `controlplane-image.spdx.json`
- `dataplane-image.spdx.json`
- `release-assets.spdx.json`
- `RELEASE_NOTES.md`
- `aether-gateway-<tag>-release-assets.tar.gz`

Image provenance and SBOM attestation will be published to GHCR associated records via GitHub Artifact Attestations. At minimum, retain the following for verification:

```bash
gh attestation verify oci://ghcr.io/<owner>/aether-gateway-controlplane@sha256:<digest> \
  --repo <owner>/aether-gateway

gh attestation verify oci://ghcr.io/<owner>/aether-gateway-dataplane@sha256:<digest> \
  --repo <owner>/aether-gateway
```

If you need to verify the provenance of the release asset tarball:

```bash
gh attestation verify aether-gateway-<tag>-release-assets.tar.gz \
  --repo <owner>/aether-gateway
```