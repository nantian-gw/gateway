# Production Overlay

This directory provides a Kustomize overlay for long-term environments, enabling a production-oriented install baseline without polluting the `deploy/kubernetes/base/` default baseline.

The matrix of install profiles, Secrets, NetworkPolicy, Services, ports, HPA, PDB, and resource requests/limits can be found in [Install Profile Matrix](../../../../docs/user/install-profiles.md). This directory is the current Kustomize source for the `single-cluster-prod` and `multi-replica-prod` profiles. The `observability-enabled` profile now lives in `../observability-enabled/` and reuses this overlay as its base.

The current overlay does the following by default:

- References the shared `../../base` baseline and `../../addons/dataplane-hpa`
- Inherits the dashboard Deployment / Service / NetworkPolicy from base; the dashboard uses a Node proxy to call controlplane / dataplane admin API
- Replaces controlplane / dataplane ConfigMaps with the overlay's own configuration content
- Enables controlplane gRPC mTLS by default
- Enables dataplane xDS mTLS by default
- Sets dataplane HTTP capacity to a high-concurrency runtime baseline, and adjusts dataplane CPU request/limit to a multi-core production baseline
- Configures dataplane `runtimeTuning.gracefulDrainPeriodMs: 10000` by default, preserving a brief drain window after readiness removal during Pod deletion, rolling upgrades, and node drains
- Disables dataplane access log by default, avoiding placing log rendering, sampling, and stdout writes in the request hot path under high QPS; explicitly enable and set a sampling rate when audit is needed
- Requires controlplane / dataplane admin Bearer Token by default
- Requires dataplane to provide a stable session persistence secret by default
- Tightens relevant Secret volume mounts from "optional" to "must-exist-or-fail"

## Pre-Use Required Modifications

1. Modify `statusAddresses` in `controlplane-config.yaml` — do not keep `replace-me.example.invalid`.
2. Prepare and apply the following Secrets:
   - `controlplane-admin-auth.secret.example.yaml`
   - `dataplane-admin-auth.secret.example.yaml`
   - `controlplane-grpc-tls.secret.example.yaml`
   - `dataplane-xds-tls.secret.example.yaml`
   - `dataplane-session-persistence.secret.example.yaml`
3. Based on the actual environment, confirm whether resources, HPA, exposure method, and additional `NetworkPolicy` need further tightening.
4. If retaining the dashboard, confirm that the `nantian-gw-dashboard` image has been built in your release pipeline and pinned to a trusted tag or digest; the current core release workflow only publishes controlplane / dataplane images.

## Performance and Capacity Prerequisites

This overlay is a production-oriented install baseline, not a guarantee that capacity validation for the target cluster has been completed. Before entering a long-term environment or production pilot, complete at least the following checks:

1. Based on actual node specifications, confirm whether controlplane / dataplane resource requests, limits, HPA targets, PDB, and replica counts match expected traffic.
2. Follow the [Performance Baseline Execution Template](../../../../docs/test/performance-baseline.md) to run a round of `run-kind-a4-baseline.sh` or equivalent staging stress tests, and archive HTTP/gRPC p99, success rate, FD, RSS, Threads, and key metrics to `reports/performance/runs/<run-id>/`.
3. Compare against the most recent clean baseline:

   ```bash
   ./scripts/compare-performance-runs.sh \
     reports/performance/runs/<baseline-run-id> \
     reports/performance/runs/<candidate-run-id>
   ```

4. Collect admin and metrics evidence both before and after stress testing, focusing on confirming that dataplane overload / rate-limit / circuit-breaker rejected counters only show expected growth, and that `/readyz`, `/v1/summary`, `/v1/listeners`, `/v1/routes`, and Prometheus metrics are consistent.
5. If access logging is enabled, first use the dataplane reload benchmark's `access_log_disabled_path` / `access_log_sampled_out_path` / `access_log_write_path` results to evaluate sampling rate and disk write path, avoiding making full access log writes the default in high-concurrency scenarios.

## Generation and Application

First preview the final manifests:

```bash
kustomize build deploy/kubernetes/overlays/production --load-restrictor LoadRestrictionsNone
```

To render a release single-file manifest using the same profile source:

```bash
./scripts/render-release-manifest.sh \
  --profile single-cluster-prod \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-install.yaml
```

Apply after confirming correctness:

```bash
kustomize build deploy/kubernetes/overlays/production \
  --load-restrictor LoadRestrictionsNone \
  | kubectl apply -f -
```

## Production Environment Conformance Prerequisites

If running Gateway API conformance directly in a production or long-term test cluster, do not only expose ports 80/443. Full HTTP/gRPC/TLS/UDP coverage depends on listener ports dynamically created by test cases. At minimum ensure the conformance runner can access:

- TCP: `80`, `443`, `8080`, `8090`, `8443`
- UDP: `5300`

The repository's Kind conformance entry point `tests/conformance/run.sh` automatically establishes local relays for these ports; the production overlay does not do this. In production environments, explicitly expose these ports using one of the following methods:

- Use control-plane-maintained per-Gateway Services and set the corresponding Service to `LoadBalancer` or `NodePort` via `Gateway.spec.infrastructure.parametersRef`.
- Use a shared dataplane Service or external LB, but confirm that LB listener synchronization covers all TCP/UDP ports required by conformance.
- If the conformance runner is outside the cluster network, confirm that security groups, firewalls, LB health checks, and `NetworkPolicy` all allow these ports.

Static address tests also require that `Gateway.spec.addresses`, control plane `statusAddresses`, and the truly reachable entry point are consistent. Do not keep `replace-me.example.invalid`, and do not continue using `127.0.0.1` in non-local tests. When using the repository scripts, explicitly declare conformance-usable addresses via the following variables:

```bash
CONFORMANCE_USABLE_ADDRESSES=IPAddress=<reachable entry IP> \
CONFORMANCE_UNUSABLE_ADDRESSES=IPAddress=203.0.113.13 \
ALL_FEATURES=true \
./tests/conformance/run.sh
```

If bypassing the repository scripts and running the upstream Gateway API harness directly, synchronously pass in the currently declared supported features. Otherwise, already-implemented capabilities like `UDPRoute` may be marked `SKIP` by the upstream harness, making test results appear not to reflect full coverage. The current repository recommends deriving the feature list from control plane source code:

```bash
cd controlplane
GOWORK=off go run ./cmd/gateway-api-support -format names
```

## Operational Constraints

- The overlay still retains the namespace-internal management access model from base by default; if Prometheus or operations entry points are not in the `nantian-gw` namespace, additional `NetworkPolicy` is needed.
- `controlplane-config.yaml` and `dataplane-config.yaml` are the true sources for the current overlay — do not modify the `configs/` directory and assume the overlay will automatically inherit changes.
- `dataplane-admin-auth.secret.example.yaml` serves dataplane's own admin auth; ensure this Secret is created and has correct content.
- `enableHttp3` remains disabled; HTTP/3 / QUIC is currently not in this production overlay's default scope.
- If the environment already has Prometheus Operator installed, optional alert rules are available in `controlplane-alert-rules.prometheusrule.yaml`. This file is not merged into `kustomization.yaml` by default, to avoid binding the production overlay to the `PrometheusRule` CRD; apply it separately with `kubectl apply -f deploy/kubernetes/overlays/production/controlplane-alert-rules.prometheusrule.yaml`, or explicitly add it to the overlay's `resources`.
