# Production Operations

This document is intended for users who plan to run Aether Gateway in long-term environments. The focus is not on quick trials, but on minimizing the attack surface, enabling control channel security, standardizing upgrade/rollback, and certificate rotation. If you are already in a release window, please also read the [Release, Canary, and Rollback Runbook](release-runbook.md).

If you prefer to start from the Kubernetes manifests in the repository rather than manually tweaking production parameters on the `base/` defaults, use the [deploy/kubernetes/overlays/production](../../deploy/kubernetes/overlays/production/README.md) overlay first.

## 1. Pre-Production Checklist

- Keep at least 2 replicas for both the control plane and data plane.
- Retain `PodDisruptionBudget`, rolling upgrade strategy, and `readinessProbe`.
- Set a non-zero `runtimeTuning.gracefulDrainPeriodMs` for the data plane, and ensure this value is less than the Pod's `terminationGracePeriodSeconds`; the repo production overlay defaults to `10000ms / 30s`.
- Expose control plane admin/metrics and data plane admin only via in-cluster `ClusterIP`.
- Use `nantian-controlplane-metrics` and `nantian-dataplane-metrics` for metrics scraping.
- Retain control plane permissions for `get/list/watch/create/update/patch` on `coordination.k8s.io/leases`; high-availability aggregation in `/v1/nodes` depends on shared Lease state.
- Configure Bearer Token for control plane admin and data plane admin.
- Configure TLS for control plane gRPC; when deploying across nodes or networks, it is recommended to directly enable mTLS.
- Configure CA for data plane `xdsTls`; when enabling mTLS, simultaneously configure client certificates.
- If using `HTTPRoute.sessionPersistence`, `GRPCRoute.sessionPersistence`, or `BackendLBPolicy.sessionPersistence`, configure a stable `sessionPersistence.secretKey` or `secretKeyFile` for the data plane.
- Keep `NetworkPolicy` enabled, only allowing the actually needed namespaces and ports.
- The repository default base manifests currently only allow the `nantian-gw` namespace to access control plane `admin` / `metrics`, and only expose the control plane `healthProbe` port to probes; if Prometheus or an operations entry point is not in that namespace, add corresponding additional `NetworkPolicy` entries.
- Prometheus should only scrape the dedicated metrics Service; do not reuse the admin interface.
- If the control plane needs to publish a global ingress address directly, change `statusAddress` to a real reachable external IP or domain name; if a Gateway requires multiple programmable addresses, use `statusAddresses` to explicitly list all addresses. Do not use `127.0.0.1` in cluster deployments — the loopback address in local sample configurations is only suitable for local process debugging.
- If a Gateway explicitly sets `spec.addresses`, ensure it represents the same set of programmable addresses as `statusAddress` or `statusAddresses`; otherwise `Programmed=False`, `Reason=AddressNotUsable` may appear. For explicit IP addresses, the control plane-maintained Gateway Service will synchronously set `externalIPs`, and for `LoadBalancer` type, will additionally set the first `loadBalancerIP`.
- If an entry in `spec.addresses` only declares `type` without filling in `value`, the control plane will automatically allocate an available address based on the request type; if no programmable address of that type is currently available, it will write back `Programmed=False`, `Reason=AddressNotAssigned`. Such automatic allocation only selects from the set of addresses currently permitted to be published to `Gateway.status.addresses`, and will not reassign addresses from derived `Gateway Service` objects whose metadata/ownership is still drifting.
- If the derived Gateway Service has not yet completed metadata/ownership convergence, the control plane will not directly publish the Service's currently exposed address to `Gateway.status.addresses`; instead it falls back to the global `statusAddress` / `statusAddresses`. This avoids exposing drifting old addresses to upstream systems. For explicit `spec.addresses`, the control plane will still reference the Service's current `LoadBalancer ingress` / `externalIPs` to determine "whether the address is programmable," so during convergence you are more likely to see `Programmed=False, Reason=Pending` rather than prematurely degrading to `AddressNotUsable`.

## 2. Enabling Admin Bearer Token

Both the control plane and data plane support two methods:

- Fill in `adminAuth.bearerToken` directly in YAML
- Read from a mounted Secret file via `adminAuth.bearerTokenFile`

Using Secret files is recommended for production environments.

Currently, when using `bearerTokenFile`, both the control plane and data plane will re-read the file on each admin request authentication, enabling restart-free token rotation in coordination with Secret volume updates.

### 2.1 Control Plane Admin Token

Create the Secret:
