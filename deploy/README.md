# Deploy Guide

`deploy/` now only carries the repository's built-in deployment assets, no longer mixing content with different purposes in a single directory layer.

## Directory Structure

```text
deploy/
  README.md
  kubernetes/
    addons/
      dataplane-hpa/
    base/
    overlays/
      kind/
      kind-hostnetwork/
      observability-enabled/
      production/
  observability/
    grafana/
```

- `deploy/kubernetes/addons/ai-gateway/`
  Optional AI Gateway addon. Contains AIService + TokenPolicy CRDs and can be independently enabled without depending on core gateway functionality.
- `deploy/kubernetes/addons/dataplane-hpa/`
  Optional dataplane HPA addon. The production overlay includes it by default, and release assets also separately include `hpa.yaml`.
- `deploy/kubernetes/base/`
  Fixed install baseline. This is the sole shared Kubernetes manifest source of truth in the repository.
- `deploy/kubernetes/overlays/kind/`
  Kind debug entry point. Currently a thin overlay that directly reuses `base/` and additionally provides a Kind cluster configuration file.
- `deploy/kubernetes/overlays/kind-hostnetwork/`
  Kind high-concurrency stress test entry point. It reuses `kind/` but places the dataplane in host network, adjusting rolling strategy and resource limits to reduce the impact of Services, conntrack, and CFS quotas on tail latency.
- `deploy/kubernetes/overlays/production/`
  Hardened overlay for long-term environments, containing stricter configuration, patches, Secret templates, and optional alert rules.
- `deploy/kubernetes/overlays/observability-enabled/`
  Controlplane tracing entry point for operators who want OTLP trace export without changing the default production overlay. It reuses `production/` and replaces only the controlplane ConfigMap content with a tracing-enabled example configuration.
- `deploy/observability/grafana/`
  Observability-related assets, currently primarily Grafana JSON.

## File-Level Responsibilities

Files in `deploy/kubernetes/base/` are split by resource boundary:

| File | Scope |
| --- | --- |
| `kustomization.yaml` | Base entry point, aggregating fixed base resources and two configuration ConfigMaps |
| `namespace-gatewayclass.yaml` | Namespace and fixed `GatewayClass` |
| `rbac.yaml` | ServiceAccount, ClusterRole, ClusterRoleBinding |
| `controlplane-config.yaml` | controlplane default configuration content |
| `dataplane-config.yaml` | dataplane default configuration content |
| `controlplane.yaml` | controlplane Deployment and PDB |
| `dataplane.yaml` | dataplane Deployment and PDB |
| `dashboard.yaml` | Dashboard standalone Deployment, Service, and NetworkPolicy |
| `services-networkpolicy.yaml` | All fixed Services and fixed NetworkPolicies |
| `aiservice-crd.yaml` | AIService CRD (AI Gateway Phase 1) |
| `tokenpolicy-crd.yaml` | TokenPolicy CRD (AI Gateway Phase 2) |
| `wasmplugin-crd.yaml` | WasmPlugin CRD (Wasm Support) |
| `../addons/dataplane-hpa/hpa.yaml` | dataplane HPA addon |

Responsibilities in `deploy/kubernetes/overlays/`:

| Path | Scope |
| --- | --- |
| `overlays/kind/kustomization.yaml` | Kind debug entry point, currently directly reuses base |
| `overlays/kind/kind-config.yaml` | Kind cluster topology and host port mapping |
| `overlays/kind-hostnetwork/kustomization.yaml` | Kind high-concurrency stress test entry, reuses `kind` and enables dataplane host network |
| `overlays/kind-hostnetwork/kind-config.yaml` | Kind high-concurrency stress test topology, default 1 control-plane plus 2 workers, no host port mapping |
| `overlays/kind-hostnetwork/patch-controlplane-networkpolicy-hostnetwork.json` | Allows hostNetwork dataplane to connect to controlplane xDS port with node source address |
| `overlays/kind-hostnetwork/patch-dataplane-hostnetwork.yaml` | dataplane uses host network, cross-node anti-affinity, no-surge rolling update |
| `overlays/kind-hostnetwork/patch-dataplane-performance-resources.json` | dataplane stress test resource settings, retains memory limit, removes CPU limit, and removes hostNetwork-incompatible network sysctls |
| `overlays/production/kustomization.yaml` | Production overlay entry point |
| `overlays/production/controlplane-config.yaml` | Production controlplane configuration replacement |
| `overlays/production/dataplane-config.yaml` | Production dataplane configuration replacement |
| `overlays/observability-enabled/kustomization.yaml` | Observability-enabled entry point, reuses `production` |
| `overlays/observability-enabled/controlplane-config.yaml` | Tracing-enabled controlplane configuration replacement example |
| `overlays/production/patch-*.yaml` | Tightens key Secret mounts from optional to required |
| `overlays/production/*.secret.example.yaml` | Production-required Secret templates |
| `overlays/production/controlplane-alert-rules.prometheusrule.yaml` | Optional alert rule template |

## Naming Conventions

Fixed resources follow this set of rules:

- Workload: `nantian-gw-<component>`
- Fixed internal Service: `nantian-gw-<component>-<role>`
- ConfigMap / Secret: `nantian-gw-<component>-<purpose>`

Current fixed components are:

- `controlplane`
- `dataplane`
- `dashboard`

Fixed Service roles currently include:

- `grpc`
- `admin`
- `metrics`

Static install resources now use the `nantian-gw-<component>` prefix consistently. Fixed Services use `nantian-gw-<component>-<role>`, so controlplane and dataplane admin, metrics, and gRPC endpoints are distinguishable from their workloads when reading directories or troubleshooting.

Additionally, fixed static resources now include standard `app.kubernetes.io/*` labels wherever possible.
If you use `kubectl get all --show-labels` or filter by labels in the cluster, it is now easier to distinguish controlplane, dataplane, and fixed Service roles than before.

## Fixed Service Descriptions

| Service | Port | Purpose | Who Accesses It |
| --- | --- | --- | --- |
| `nantian-gw-controlplane-grpc` | `18080` | controlplane gRPC / xDS publish entry | dataplane |
| `nantian-gw-controlplane-admin` | `18081` | controlplane admin API | Ops entry, `kubectl port-forward`, controlled proxies |
| `nantian-gw-controlplane-metrics` | `18082` | controlplane metrics scrape entry | Prometheus or other scrapers |
| `nantian-gw-dataplane-admin` | `19080` | dataplane admin API | Ops entry, `kubectl port-forward`, controlled proxies |
| `nantian-gw-dataplane-metrics` | `19080` | dataplane metrics scrape entry, currently reuses admin server port | Prometheus or other scrapers |
| `nantian-gw-dashboard` | `8080` | Web admin console, Node server serves SPA and same-origin proxies admin API | Ops entry, `kubectl port-forward`, controlled proxies |

Notes:

- `nantian-gw-dataplane-metrics` and `nantian-gw-dataplane-admin` both currently point to the dataplane `admin` port; the difference lies in purpose and scrape entry point, not in backend port numbers.
- `nantian-gw-dashboard` does not directly access the Kubernetes API; it only proxies controlplane / dataplane admin Services through the container-internal Node server.
- These Services are all part of the fixed static manifest and are suitable for writing into operations documentation, scripts, and monitoring configurations.
- These fixed Services are all part of the base static manifest, currently defined in `deploy/kubernetes/base/services-networkpolicy.yaml`, not dynamically generated runtime objects.

## Dynamic Service Descriptions

In addition to the above fixed Services, the control plane also dynamically creates or maintains some Services based on Gateways / Routes / mesh frontends.

These Services:

- Are not static content from `deploy/kubernetes/base/`
- Follow Gateway resources, frontend exposure methods, and current state changes
- Should be understood as control plane runtime artifacts rather than static install assets

If you see additional Services in the cluster, don't first suspect that the `deploy/` directory has duplicate definitions; first check:

- Gateway / HTTPRoute / GRPCRoute / Service parent related resources
- controlplane admin API
- The notes on Gateway infrastructure and runtime exposure in `docs/user/operations.md`

The two most important dynamic Services currently are:

| Service | Source | Default Type | Purpose | Carries Business Traffic? |
| --- | --- | --- | --- | --- |
| `nantian-gw-dataplane` | shared dataplane Service | `NodePort` | Aggregates all current Gateway listener ports, providing a unified frontend entry for the dataplane | Yes |
| `nantian-gw-<gatewayName>` | per-Gateway Service | `ClusterIP` | Dedicated frontend Service for a single Gateway, individually exposable per Gateway infrastructure parameters | Yes |

Notes:

- `nantian-gw-dataplane` is the shared frontend entry; current Kind smoke defaults to using it.
- `nantian-gw-<gatewayName>` is a dedicated Service derived from the Gateway name; for example, if the `Gateway` is named `edge`, the corresponding Service is typically `nantian-gw-edge`.
- per-Gateway Services default to `ClusterIP`, but can be lowered to `NodePort` or `LoadBalancer` via `Gateway.spec.infrastructure.parametersRef`, making them the more natural north-south exposure point in long-term environments.
- These objects are maintained by the control plane reconcile loop and are not static install assets from `deploy/kubernetes/base/`.

## Traffic Entry Relationships

To determine "where does traffic actually come in," understand the following relationships:

| Object | Role |
| --- | --- |
| `Gateway` | Entry semantics, defines listeners, declares which traffic set should be admitted |
| Dynamic `Service` | The actual Kubernetes network bearing point; clients ultimately hit this |
| `HTTPRoute` / `GRPCRoute` | Matching and forwarding rules after entering a listener, not the entry point itself |
| backend `Service` | The backend service ultimately routed to |

The standard chain can be simplified as:

```text
Client -> Gateway's corresponding Service -> dataplane listener -> Route -> backend Service
```

This means:

- From the Gateway API semantic perspective, the entry object is `Gateway`.
- From the Kubernetes network landing perspective, the object that actually accepts connections is the dynamic Service, not `HTTPRoute`.
- `HTTPRoute` is only responsible for continuing to forward requests that have already entered the Gateway listener to the backend.

## Kind vs Production Entry Differences

The most easily confused aspect of the current repository is that the entries seen in Kind quick-start and production deployments are not exactly the same.

| Scenario | Default Entry | Notes |
| --- | --- | --- |
| Kind / smoke | `nantian-gw-dataplane` | Shared dataplane Service uses `NodePort`; Kind overlay maps HTTP `18080`, HTTPS/TLS `18443`, UDP `5300` / `5301`, TCPRoute `19000` / `19001` by default |
| Long-term / production | `nantian-gw-<gatewayName>` | Better to expose per Gateway individually, then control `ClusterIP` / `NodePort` / `LoadBalancer` via `parametersRef` |

So if you see locally:

- `curl http://127.0.0.1:18080`
- `printf 'GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n' | nc -w 5 127.0.0.1 19000`

That's typically hitting the shared dataplane Service.

If you're troubleshooting "why doesn't a certain Gateway have an external address" in production, the priority checks should be:

- The Gateway's corresponding `nantian-gw-<gatewayName>` Service
- Its `type`, `externalIPs`, `LoadBalancer ingress`
- Whether `Gateway.status.addresses` has converged with that Service

## When to Use Which Entry Point

- Local or Kind bring-up:
  `kustomize build deploy/kubernetes/overlays/kind --load-restrictor LoadRestrictionsNone`
- Kind high-concurrency stress test:
  `kustomize build deploy/kubernetes/overlays/kind-hostnetwork --load-restrictor LoadRestrictionsNone`
  This entry point places the dataplane directly on the node network. During stress testing, hit the Gateway listener port on the Kind node container IP, e.g., HTTP listener `http://<node-ip>/`, to isolate the impact of Services, kube-proxy, and conntrack on tail latency.
- Long-term environments:
  `kustomize build deploy/kubernetes/overlays/production --load-restrictor LoadRestrictionsNone | kubectl apply -f -`
- Long-term environments with controlplane OTLP tracing enabled:
  `kustomize build deploy/kubernetes/overlays/observability-enabled --load-restrictor LoadRestrictionsNone | kubectl apply -f -`
- Release single-file install manifest:
  Rendered from the Kustomize entry point corresponding to the install profile via `scripts/render-release-manifest.sh --profile <profile>` as `install.yaml`. The current profile matrix is in `docs/user/install-profiles.md`
  The current render script only replaces controlplane / dataplane images; dashboard resources come from base manifests. In production environments where the dashboard is needed, patch the `nantian-gw-dashboard` image to a published, digest-pinned image in your own overlay or release pipeline.
- HPA:
  Use `deploy/kubernetes/addons/dataplane-hpa/hpa.yaml`, or have it automatically included via the production overlay
- Grafana observability dashboard:
  Use `deploy/observability/grafana/nantian-gw-observability-dashboard.json`

If you are choosing a business traffic entry point rather than an install entry point, see `docs/user/traffic-profiles.md`. That document provides north-south HTTP/gRPC, north-south TCP/UDP, and east-west service parent examples separately.

## Controlplane Tracing Verification

Use `deploy/kubernetes/overlays/observability-enabled/` when you want the production install baseline plus a tracing-enabled controlplane configuration example. This overlay reuses `overlays/production/` and replaces only `nantian-gw-controlplane-config`.

Render the overlay with the same load-restrictor setting already used by repository scripts:

```bash
kustomize build deploy/kubernetes/overlays/observability-enabled \
  --load-restrictor LoadRestrictionsNone \
  >/tmp/nantian-gw-observability-enabled.yaml
```

Then verify the rendered manifest contains the tracing block and endpoint fields from your configured example:

```bash
rg -n "tracing:|endpoint:|samplerRatio" \
  /tmp/nantian-gw-observability-enabled.yaml
```

Apply the rendered manifest, restart the controlplane Deployment so the ConfigMap replacement takes effect, and then inspect logs for the startup summary that confirms tracing was configured:

```bash
kubectl apply -f /tmp/nantian-gw-observability-enabled.yaml
kubectl rollout restart deploy/nantian-gw-controlplane -n nantian-gw
kubectl rollout status deploy/nantian-gw-controlplane -n nantian-gw
kubectl logs -n nantian-gw deploy/nantian-gw-controlplane | rg "configured controlplane tracing"
```

Troubleshooting:

- Wrong overlay used: if the rendered manifest or live ConfigMap does not contain a `tracing:` block, you likely applied `overlays/production/` instead of `overlays/observability-enabled/`.
- OTLP endpoint unreachable: if tracing is enabled but spans do not arrive, confirm the endpoint in `deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml` resolves and accepts OTLP/gRPC traffic from the `nantian-gw` namespace.
- Tracing header values intentionally redacted from startup logs: startup logs summarize tracing configuration, but do not echo sensitive header values; inspect the applied ConfigMap or Secret-backed configuration source instead of expecting those values in logs.
