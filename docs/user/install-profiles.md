# Installation Profile Matrix

This document is the profile matrix for current Kustomize / release manifest installation entry points.
It is intended for deployers and release maintainers, answering "which entry point should I render or apply manifests from, what Secrets are required, which Services are exposed by default, and what minimum verification is needed."

If you are choosing a north-south / east-west traffic access pattern rather than choosing an installation entry point, see [Traffic Profile Examples](traffic-profiles.md) first. Installation profiles can be combined with traffic profiles; for example, `single-cluster-prod` can simultaneously carry north-south `Gateway` and east-west service parent traffic.

The current repository includes a Next.js/React dashboard and its Kubernetes `Deployment` / `Service` / `NetworkPolicy`, rendered together with base / overlay via `deploy/kubernetes/base/dashboard.yaml`. The core release boundary of installation profiles remains centered on controlplane, dataplane, and observability; the dashboard image is not currently part of the core image set published by the release workflow.

Current official entry points are:

- Local or Kind: `deploy/kubernetes/overlays/kind`
- Kind high-concurrency load testing: `deploy/kubernetes/overlays/kind-hostnetwork`
- Long-term environments: `deploy/kubernetes/overlays/production`
- Release single-file manifest: `scripts/render-release-manifest.sh --profile <profile> ...`
- Release asset packaging: `scripts/prepare-release-assets.sh`, defaulting to `RELEASE_INSTALL_PROFILE=single-cluster-prod`

Helm charts and Operators are not yet current default installation entry points. If these packaging forms are added later, they must reuse the profile field semantics from this document to avoid drift from Kustomize / release manifests.

Dashboard note: `scripts/render-release-manifest.sh` currently only replaces controlplane and dataplane images. If you keep the dashboard in production or long-term environments, replace the `nantian-gw-dashboard` image in your own overlay / release pipeline with a published, digest-pinned image; if you do not use the UI, you can also remove the dashboard resources in your environment overlay.

## Profile Overview

| Profile | Applicable Scenario | Render Source | Default External Exposure | Minimum Verification |
| --- | --- | --- | --- | --- |
| `kind-dev` | Local development, Kind smoke, low-cost validation | `deploy/kubernetes/overlays/kind` | Not directly exposed to public internet; exposed via kind/e2e scripts or port-forward | `kubectl kustomize deploy/kubernetes/overlays/kind` |
| `kind-hostnetwork-perf` | High-concurrency load testing inside Kind, needing to bypass Service / conntrack to observe dataplane limits | `deploy/kubernetes/overlays/kind-hostnetwork` | Dataplane directly listens on node network; load tests should use Kind node container IPs and Gateway listener ports, not ClusterIP Services | `kubectl kustomize deploy/kubernetes/overlays/kind-hostnetwork` |
| `single-cluster-prod` | Default production entry point for single-cluster long-term environments | `deploy/kubernetes/overlays/production` | Does not automatically create external LBs; exposed via Gateway-corresponding Service or external ingress | `kubectl kustomize deploy/kubernetes/overlays/production` |
| `multi-replica-prod` | Multi-replica control plane and data plane, long-term environments requiring HPA/PDB | `deploy/kubernetes/overlays/production` | Same as `single-cluster-prod`, exposed per cluster LB / Gateway strategy | `kubectl kustomize deploy/kubernetes/overlays/production` |
| `observability-enabled` | Requires Prometheus/Grafana collection and alerting | `deploy/kubernetes/overlays/production` + `deploy/observability/` examples | Metrics Service is open within the namespace by default; cross-namespace collection requires explicit NetworkPolicy | `./scripts/check-metrics-cardinality-contract.sh` |

## Rendering Release Manifest

Rendering local or Kind profile:

```bash
./scripts/render-release-manifest.sh \
  --profile kind-dev \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-install.yaml
```

Rendering production profile:

```bash
./scripts/render-release-manifest.sh \
  --profile single-cluster-prod \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-install.yaml
```

`scripts/prepare-release-assets.sh` renders `install.yaml` in the release asset as `single-cluster-prod` by default. The release asset must record image digests, so the formal release workflow passes `RELEASE_CONTROLPLANE_DIGEST` and `RELEASE_DATAPLANE_DIGEST` after image build/push. When packaging locally, you can also pass image references in `image@sha256:<digest>` format.

To temporarily generate a Kind-targeted release asset, override the profile explicitly and pass digests:

```bash
RELEASE_INSTALL_PROFILE=kind-dev \
RELEASE_CONTROLPLANE_DIGEST=sha256:<64-hex-controlplane-digest> \
RELEASE_DATAPLANE_DIGEST=sha256:<64-hex-dataplane-digest> \
./scripts/prepare-release-assets.sh \
  v0.0.0 \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-release
```

The `install.yaml` rendered from production profiles does not inline real Secrets. Before deploying, you must generate real Secrets from `deploy/kubernetes/overlays/production/*.secret.example.yaml` and replace environment-specific fields such as `statusAddresses`.

## Profile Details

### `kind-dev`

Entry points:

- `kubectl kustomize deploy/kubernetes/overlays/kind`
- `./scripts/render-release-manifest.sh --profile kind-dev ...`

Default configuration:

| Item | Current Value |
| --- | --- |
| Secrets | controlplane gRPC TLS, controlplane admin auth, dataplane xDS TLS, dataplane admin auth are all optional mounts |
| NetworkPolicy | controlplane gRPC/admin/metrics only allow access within the `nantian-gw` namespace; healthz allows cluster-external probe sources; dataplane admin/metrics only allow access within the namespace |
| Services | controlplane gRPC `18080`, admin `18081`, metrics `18082`; dataplane admin/metrics `19080`; dashboard HTTP `8080` |
| Data plane traffic ports | dataplane Pod listens on HTTP `10080` and HTTPS `443` by default; actual Gateway exposure is handled by the Gateway-corresponding Service created or managed by the control plane |
| HPA | Not enabled by default |
| PDB | controlplane `minAvailable: 1`, dataplane `minAvailable: 1` |
| resources | controlplane requests `100m/128Mi`, limits `500m/512Mi`; dataplane requests `250m/256Mi`, memory limit `1Gi`, no CPU limit set |
| observability | Metrics Service provided by default, no Prometheus Operator resources included |

Minimum verification:

```bash
kubectl kustomize deploy/kubernetes/overlays/kind >/tmp/nantian-gw-kind.yaml
```

Kind smoke testing should still prefer the repository scripts:

```bash
./tests/e2e/run-kind.sh
```

### `kind-hostnetwork-perf`

Entry points:

- `kubectl kustomize deploy/kubernetes/overlays/kind-hostnetwork`

Default configuration:

| Item | Current Value |
| --- | --- |
| Baseline | Reuses `kind-dev` |
| dataplane replicas | Default 1, suitable for single-node baseline; scale by node count for multi-node load testing |
| dataplane network | `hostNetwork: true`, `dnsPolicy: ClusterFirstWithHostNet` |
| dataplane scheduling | `requiredDuringSchedulingIgnoredDuringExecution` cross-node anti-affinity to avoid same-node host port conflicts |
| dataplane rolling update | `maxSurge: 0`, `maxUnavailable: 1` to avoid same-node port conflicts during rollouts |
| dataplane resources | requests `2CPU/512Mi`, memory limit `2Gi`, no CPU limit |
| hostNetwork compatibility | Removes base `net.ipv4.ip_unprivileged_port_start` sysctl, relies on `NET_BIND_SERVICE` for low-port binding; additionally allows node source addresses to connect to controlplane xDS `18080` |
| Load test entry point | Use Kind node container IP's Gateway listener port to hit dataplane directly, e.g. HTTP listener `http://<node-ip>/`; do not use dataplane ClusterIP / NodePort Services to evaluate application limits |

It is recommended to create a separate multi-node Kind load testing cluster:

```bash
kind create cluster --name nantian-gw-perf --config deploy/kubernetes/overlays/kind-hostnetwork/kind-config.yaml
```

Before multi-node load testing, scale dataplane to match the number of workers / nodes; anti-affinity prevents multiple hostNetwork dataplane replicas on the same node:

```bash
kubectl -n nantian-gw scale deployment/nantian-dataplane --replicas=2
```

Minimum verification:

```bash
kubectl kustomize deploy/kubernetes/overlays/kind-hostnetwork >/tmp/nantian-gw-kind-hostnetwork.yaml
```

This profile is only for local or controlled load testing environments. It causes the dataplane to listen on node network ports and is not suitable as a default production installation entry point; the production long-term entry point remains `single-cluster-prod`.

### `single-cluster-prod`

Entry points:

- `kubectl apply -k deploy/kubernetes/overlays/production`
- `./scripts/render-release-manifest.sh --profile single-cluster-prod ...`

Required Secrets:

| Secret | Purpose | Example File |
| --- | --- | --- |
| `nantian-controlplane-admin-auth` | controlplane admin Bearer Token | `deploy/kubernetes/overlays/production/controlplane-admin-auth.secret.example.yaml` |
| `nantian-controlplane-grpc-tls` | controlplane gRPC server certificate and dataplane client CA | `deploy/kubernetes/overlays/production/controlplane-grpc-tls.secret.example.yaml` |
| `nantian-dataplane-admin-auth` | dataplane admin Bearer Token | `deploy/kubernetes/overlays/production/dataplane-admin-auth.secret.example.yaml` |
| `nantian-dataplane-xds-tls` | dataplane client certificate and CA for connecting to controlplane xDS/gRPC | `deploy/kubernetes/overlays/production/dataplane-xds-tls.secret.example.yaml` |
| `nantian-dataplane-session-persistence` | stable session persistence secret | `deploy/kubernetes/overlays/production/dataplane-session-persistence.secret.example.yaml` |

Default configuration:

| Item | Current Value |
| --- | --- |
| NetworkPolicy | Follows base namespace-scoped management plane access model; cross-namespace Prometheus or ops proxy access requires additional allow rules |
| Services | controlplane gRPC/admin/metrics, dataplane admin/metrics, and dashboard HTTP all retain ClusterIP semantics |
| ports | controlplane `18080/18081/18082/18083`; dataplane runtime `10080/443`, admin/metrics `19080` |
| HPA | dataplane HPA enabled by default, `minReplicas: 2`, `maxReplicas: 10`, CPU `70%`, memory `75%` |
| PDB | controlplane `minAvailable: 1`, dataplane `minAvailable: 1` |
| resources | dataplane overlay defaults to requests `2CPU/512Mi`, memory limit `2Gi`, no CPU limit; adjust based on real RPS, TLS, streaming, and metrics scrape scale before launch |
| dataplane HTTP capacity | Uses high-concurrency capacity baseline by default, with `reusePort` enabled; `runtimeTuning.httpCapacity` only retains explicit overrides; access logging is disabled by default, enable explicitly with sampling rate only when business auditing is needed |
| dataplane shutdown drain | `runtimeTuning.gracefulDrainPeriodMs: 10000`, combined with readiness removal, PDB, and `terminationGracePeriodSeconds: 30` to reduce short-connection reset risk during rolling updates, Pod deletion, and node drain |
| observability | Metrics Service exposed by default; Prometheus collection assets under `deploy/observability/prometheus/` |

Minimum verification:

```bash
kubectl kustomize deploy/kubernetes/overlays/production >/tmp/nantian-gw-production.yaml
```

Pre-apply checks:

- `statusAddresses` in `controlplane-config.yaml` must not retain `replace-me.example.invalid`.
- All Secret examples must be copied into real Secrets before applying.
- For full conformance, the external entry point must cover the required ports for HTTP/gRPC/TLS/UDP, not just expose `80` and `443`.

### `multi-replica-prod`

`multi-replica-prod` currently reuses the production overlay; the difference is that capacity and replica strategy must be explicitly confirmed before deployment:

| Item | Current Baseline | Pre-Launch Action |
| --- | --- | --- |
| controlplane replicas | base default `2` | Confirm whether adjustment is needed based on apiserver watch, status update, and leader election load |
| dataplane replicas | base default `2`, HPA `2-10` | Adjust HPA max based on ingress traffic, connections, CPU, RSS, FD, and p99 |
| PDB | `minAvailable: 1` | Multi-node production should evaluate stricter availability targets |
| topology spread | `ScheduleAnyway` distribution by `kubernetes.io/hostname` | If the cluster has zone labels, additional zone dimension constraints can be added |
| resources | dataplane overlay defaults to requests `2CPU/512Mi`, memory limit `2Gi`, no CPU limit | Calibrate requests and memory limits with release performance / soak evidence |

Minimum verification:

```bash
./scripts/render-release-manifest.sh \
  --profile multi-replica-prod \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-multi-replica.yaml
```

Post-deployment checks at minimum:

```bash
kubectl -n nantian-gw get deploy,hpa,pdb
kubectl -n nantian-gw rollout status deploy/nantian-controlplane
kubectl -n nantian-gw rollout status deploy/nantian-dataplane
```

#### Replica and Capacity Calculation

Do not simply assume that controlplane and dataplane "just need 2 replicas by default." `2` is only a high-availability lower bound, not a capacity conclusion.

Control plane replicas should be calculated by "data plane node count + config churn + node status persistence pressure," not directly by north-south request RPS. The following method is recommended:

```text
desired_controlplane_replicas =
  max(
    2,
    ceil(connected_dataplane_nodes / stream_budget_per_replica),
    ceil(config_change_events_per_minute / reconcile_budget_per_replica),
    ceil(node_status_writes_per_second / lease_write_budget_per_replica),
  )
```

Notes:

- `stream_budget_per_replica` represents the number of xDS streams a single controlplane replica can stably handle within the target ACK latency.
- `reconcile_budget_per_replica` represents the Gateway / Route / EndpointSlice change rate a single controlplane replica can stably handle within the target snapshot latency.
- `lease_write_budget_per_replica` represents the node status write rate a single controlplane replica can stably handle without accumulating a persistent queue backlog.
- These budgets must not be guessed; they must come from your load test or pre-production baseline, not directly from the repository defaults.

When scaling the control plane, prioritize observing:

- `nantian_gateway_controlplane_xds_publish_ack_lag_seconds`
- `nantian_gateway_controlplane_xds_publish_nack_lag_seconds`
- `nantian_gateway_controlplane_xds_snapshot_ack_timeouts_total`
- `nantian_gateway_controlplane_node_status_persist_queue_depth`
- `nantian_gateway_controlplane_node_status_persist_pending_nodes`
- `driftedNodeCount` and `currentVersionReadyCount` in `/v1/snapshot-sync`

Rules of thumb:

- If ACK lag noticeably rises and approaches `snapshotAckTimeout`, prioritize scaling the controlplane.
- If `driftedNodeCount` does not return to near `0` over an extended period under normal network conditions, prioritize scaling the controlplane or reducing config churn.
- If the Lease persistence queue remains non-zero or `node_status_persist_dropped_total` increases, prioritize scaling the controlplane.

Data plane replicas should be calculated by "proven per-replica capacity," not simply aligned to the control plane by Pod count. Recommended formula:

```text
desired_dataplane_replicas =
  max(
    2,
    ceil(peak_rps / proven_rps_per_replica),
    ceil(peak_concurrent_connections / proven_connections_per_replica),
    ceil(peak_bandwidth_mbps / proven_bandwidth_mbps_per_replica),
    ceil(peak_listener_or_route_set / proven_listener_or_route_budget_per_replica),
  )
```

Notes:

- `proven_*_per_replica` must come from your environment baseline, e.g. kind A4, dataplane perf baseline, pre-production real traffic replay — not theoretical values.
- For production planning, do not use the peak values from load testing directly as budget; use only approximately `60%~70%` of proven per-replica capacity as the long-term budget ceiling.
- If configuration changes are frequent, take the larger of "traffic budget" and "reload / xDS convergence budget" as the final replica count.

When scaling the data plane, prioritize observing:

- `nantian_gateway_dataplane_ready` and ready replica aggregate view
- CPU, RSS, FD, connection count
- Request `p95` / `p99`
- xDS connect / stream failure count
- `readinessState`, `currentSnapshotStatus`, listener stale / serving-last-good signals in `/v1/summary`

If only north-south traffic is growing while controlplane metrics remain stable, do not scale the controlplane first; prioritize scaling the dataplane.
If only object churn, ACK lag, or Lease persistence backlog is building while dataplane CPU / p99 remain stable, do not only scale the dataplane; prioritize scaling the controlplane.

### `observability-enabled`

`observability-enabled` currently reuses the production overlay and combines it with example resources under `deploy/observability/`.

Default resources:

| Item | Current Value |
| --- | --- |
| controlplane metrics Service | `nantian-controlplane-metrics:18082` |
| dataplane metrics Service | `nantian-dataplane-metrics:19080` |
| metrics NetworkPolicy | Default allows access only within the `nantian-gw` namespace |
| Prometheus Operator examples | `deploy/observability/prometheus/operator/servicemonitor-dataplane.yaml`, `podmonitor-dataplane.yaml`, `prometheusrule-dataplane.yaml` |
| Native Prometheus examples | `deploy/observability/prometheus/native/prometheus-dataplane-scrape.yaml` |
| Grafana observability dashboard | `deploy/observability/grafana/nantian-gw-observability-dashboard.json` |
| controlplane PrometheusRule | `deploy/kubernetes/overlays/production/controlplane-alert-rules.prometheusrule.yaml`, not merged into production kustomization by default |

Collection recommendations:

- Dataplane readiness is a per-pod gauge; Prometheus must scrape each pod or endpoint independently — do not query `nantian-dataplane-metrics` Service only once.
- If Prometheus is in the `monitoring` namespace, additional NetworkPolicy is needed to allow it to access the metrics Service or Pods in the `nantian-gw` namespace.
- If admin auth is enabled, Prometheus needs a real token Secret; do not apply `*.example.yaml` directly.

Minimum verification:

```bash
./scripts/check-metrics-cardinality-contract.sh
```

Post-deployment PromQL checks:

```promql
count(nantian_gateway_dataplane_ready{namespace="nantian-gw"})
sum(nantian_gateway_dataplane_ready{namespace="nantian-gw"})
```

If `count` is only `1` in a three-replica environment, it means Prometheus is only scraping a single target — the dataplane ready semantics themselves do not represent the total replica count.

## Upgrades, Rollbacks, and Canary

Upgrade and rollback procedures should not rely on simply "re-applying the same set of Pods behind the same Service." The current repository recommends canarying at the Gateway API object boundary:

1. Use the [Release, Canary, and Rollback Runbook](release-runbook.md) for pre-release checks.
2. Use `scripts/prepare-canary-gatewayclass.sh` to prepare a canary class from the stable `GatewayClass`.
3. Switch a small number of `Gateway` objects to the canary `GatewayClass`.
4. Observe Gateway / Route conditions, node ACKs, snapshot versions, p99, success rate, and error types.
5. If rollback is needed, use `scripts/rollback-canary-gatewayclass.sh` to switch the target Gateway back to the stable class.

Minimum commands:

```bash
./scripts/prepare-canary-gatewayclass.sh
./scripts/rollback-canary-gatewayclass.sh
```

To package a release asset before release:

```bash
RELEASE_CONTROLPLANE_DIGEST=sha256:<64-hex-controlplane-digest> \
RELEASE_DATAPLANE_DIGEST=sha256:<64-hex-dataplane-digest> \
./scripts/prepare-release-assets.sh \
  v0.0.0 \
  ghcr.io/example/nantian-controlplane:v0.0.0 \
  ghcr.io/example/nantian-dataplane:v0.0.0 \
  /tmp/nantian-gw-release
```

## Current Boundaries

- The `production` overlay is the current production installation entry point; Helm and Operator remain in the future packaging backlog.
- Dashboard Kubernetes resources are rendered with base / overlay, but dashboard image publishing and verification are not part of the current core release gate; users must patch/pin the dashboard image themselves, or remove the resource when the UI is not needed.
- `observability-enabled` does not apply Prometheus Operator CRD-related objects by default, to avoid tightly coupling the production overlay to Prometheus Operator.
- Full conformance requires additional TCP/UDP listener ports; production environments should not only expose `80` and `443`.
- HTTP/3 / QUIC is not currently within the default scope of these profiles.
- North-south / east-west are traffic profiles, not new installation profiles; for examples and troubleshooting order, see [Traffic Profile Examples](traffic-profiles.md).
