# Prometheus Scrape Bundle

This directory contains example Prometheus scrape assets for:

- the controlplane metrics endpoint exposed by `nantian-controlplane-metrics`
- the dataplane metrics endpoint exposed by `nantian-dataplane-metrics`

Use these files when you want Prometheus to scrape each dataplane pod separately so queries like `sum(nantian_gateway_dataplane_ready)` reflect ready replica counts instead of the view from a single Service request.

Metric names, label cardinality classes, and golden-signal expectations are documented in [`docs/contracts/metrics-cardinality.md`](../../../docs/contracts/metrics-cardinality.md). Update that contract before adding default scrape metrics, recording rules, or Grafana queries that introduce new labels.

## Why This Exists

`nantian_gateway_dataplane_ready` is a per-pod gauge, not a replica counter.

- Metric definition: `1 if the dataplane has applied at least one snapshot, 0 otherwise.`
- A single request to `nantian-dataplane-metrics` only hits one backend pod.
- Replica-level views require Prometheus to scrape every endpoint or pod independently and then aggregate with PromQL.

Do not use a single static target such as:

```yaml
static_configs:
  - targets:
      - nantian-dataplane-metrics.nantian-gw.svc:19080
```

That configuration only gives you a single-pod view.

## Files

### Native Prometheus

- `native/prometheus-controlplane-scrape.yaml`
- `native/prometheus-dataplane-scrape.yaml`
- `native/prometheus-dataplane-rules.yaml`

Each scrape file contains two example `scrape_configs` fragments:

- `endpoints`
- `endpointslice`

Choose one discovery mode per plane. Copy the matching controlplane and dataplane `scrape_configs` blocks into your existing `prometheus.yml`.
Copy the rules file into a Prometheus rule directory and add it to `rule_files`,
for example:

```yaml
rule_files:
  - /etc/prometheus/rules/prometheus-dataplane-rules.yaml
```

### Prometheus Operator

- `operator/secret-dataplane-admin-token.example.yaml`
- `operator/servicemonitor-controlplane.yaml`
- `operator/podmonitor-controlplane.yaml`
- `operator/servicemonitor-dataplane.yaml`
- `operator/podmonitor-dataplane.yaml`
- `operator/prometheusrule-dataplane.yaml`
- `operator/networkpolicy-prometheus-scrape.yaml`

Choose one scrape object per plane:

- `ServiceMonitor`
- `PodMonitor`

Do not apply both for the same controlplane or dataplane targets, or you will scrape the same pods twice and inflate aggregate queries.

The scrape and rule examples assume:

- kubelet cAdvisor metrics are scraped for `container_cpu_usage_seconds_total`,
  `container_cpu_cfs_throttled_periods_total`, `container_cpu_cfs_periods_total`,
  and `container_memory_working_set_bytes`.
- kube-state-metrics is scraped for `kube_pod_container_resource_requests` and
  `kube_pod_container_resource_limits`.

Adjust those values if your cluster uses different conventions.

The Operator objects also assume:

- Operator namespace: `monitoring`
- Prometheus release label: `kube-prometheus-stack`

If your cluster enforces Kubernetes `NetworkPolicy`, also apply `operator/networkpolicy-prometheus-scrape.yaml` or an equivalent policy. The base Aether Gateway manifests only allow the `nantian-gw` namespace to reach controlplane `18082/TCP` and dataplane `19080/TCP`; a Prometheus pod running in `monitoring` will otherwise discover targets but fail to scrape them.

The native rules file and the Operator `PrometheusRule` create dataplane
container resource recording rules:

- `nantian_gateway_dataplane_container_cpu_cores`
- `nantian_gateway_dataplane_container_cpu_request_cores`
- `nantian_gateway_dataplane_container_cpu_throttle_ratio`
- `nantian_gateway_dataplane_container_memory_working_set_bytes`
- `nantian_gateway_dataplane_container_memory_limit_bytes`
- `nantian_gateway_dataplane_container_memory_request_bytes`

These rules intentionally use cAdvisor and kube-state-metrics rather than the
dataplane admin endpoint. The dataplane `/metrics` process metrics remain a
process-local cross-check, while the container rules reflect Kubernetes resource
requests, limits, working set, and CPU throttling.

## Bearer Token

If dataplane admin auth is enabled, keep the `authorization` sections and create a real Secret from `operator/secret-dataplane-admin-token.example.yaml`.

If dataplane admin auth is disabled, remove the `authorization` block from:

- the native Prometheus scrape job you copy
- the `ServiceMonitor`
- the `PodMonitor`

For Prometheus Operator, the Secret referenced by `authorization.credentials` must exist in the same namespace as the `ServiceMonitor` or `PodMonitor`, which is `monitoring` in the examples here.

## Recommended Choice

- Native Prometheus: prefer `endpointslice` if your Prometheus and Kubernetes versions support it.
- Prometheus Operator: prefer `PodMonitor` if you want the most direct per-pod scrape behavior.

## Validation Queries

The exact `job` label may vary between native Prometheus, `ServiceMonitor`, and `PodMonitor` setups.
For first-pass validation, prefer filtering by namespace and checking pod labels directly.

Use these queries after rollout:

```promql
nantian_gateway_snapshot_builds_total{namespace="nantian-gw"}
```

```promql
histogram_quantile(0.95, sum by (le) (rate(nantian_gateway_controlplane_xds_publish_ack_lag_seconds_bucket{namespace="nantian-gw"}[5m])))
```

```promql
nantian_gateway_dataplane_ready{namespace="nantian-gw"}
```

```promql
count(nantian_gateway_dataplane_ready{namespace="nantian-gw"})
```

```promql
sum(nantian_gateway_dataplane_ready{namespace="nantian-gw"})
```

```promql
nantian_gateway_dataplane_ready_replicas{namespace="nantian-gw"}
```

```promql
nantian_gateway_dataplane_container_cpu_throttle_ratio{namespace="nantian-gw"}
```

```promql
nantian_gateway_dataplane_container_memory_working_set_bytes{namespace="nantian-gw"}
```

```promql
sum by (status_class) (rate(nantian_gateway_dataplane_admin_requests_total{namespace="nantian-gw"}[5m]))
```

```promql
histogram_quantile(0.95, sum by (le, route) (rate(nantian_gateway_dataplane_admin_request_duration_seconds_bucket{namespace="nantian-gw"}[5m])))
```

```promql
kube_deployment_spec_replicas{namespace="nantian-gw",deployment="nantian-dataplane"}
```

```promql
kube_deployment_status_replicas_available{namespace="nantian-gw",deployment="nantian-dataplane"}
```

Interpretation:

- `count = 3` and `sum = 3`: three dataplane pods are being scraped separately and all are ready.
- `count = 1` and `sum = 1`: only one scrape target is being discovered.
- `count = 3` and `sum = 2`: three scrape targets exist but one dataplane pod is not ready under this metric’s definition.
- Empty controlplane panels usually mean the controlplane scrape object or native `scrape_configs` block is missing, the `job_controlplane` Grafana variable is not matching the real `job` label, or NetworkPolicy blocks Prometheus from reaching `nantian-controlplane-metrics:18082`.
- Empty container resource recording rules usually mean kubelet cAdvisor or
  kube-state-metrics is not scraped, or your cluster uses different namespace,
  pod, or container labels. Keep the recording rules aligned with your deployed
  namespace and the `dataplane` container name.

## Apply Examples

Do not apply `operator/secret-dataplane-admin-token.example.yaml` as-is.
Copy it first, replace the placeholder token, then apply the real Secret manifest.

Prometheus Operator:

```bash
cp deploy/observability/prometheus/operator/secret-dataplane-admin-token.example.yaml /tmp/nantian-dataplane-admin-token.yaml
$EDITOR /tmp/nantian-dataplane-admin-token.yaml
kubectl apply -f /tmp/nantian-dataplane-admin-token.yaml
kubectl apply -f deploy/observability/prometheus/operator/networkpolicy-prometheus-scrape.yaml
kubectl apply -f deploy/observability/prometheus/operator/podmonitor-controlplane.yaml
kubectl apply -f deploy/observability/prometheus/operator/podmonitor-dataplane.yaml
kubectl apply -f deploy/observability/prometheus/operator/prometheusrule-dataplane.yaml
```

Or, if you prefer Service-based discovery:

```bash
cp deploy/observability/prometheus/operator/secret-dataplane-admin-token.example.yaml /tmp/nantian-dataplane-admin-token.yaml
$EDITOR /tmp/nantian-dataplane-admin-token.yaml
kubectl apply -f /tmp/nantian-dataplane-admin-token.yaml
kubectl apply -f deploy/observability/prometheus/operator/networkpolicy-prometheus-scrape.yaml
kubectl apply -f deploy/observability/prometheus/operator/servicemonitor-controlplane.yaml
kubectl apply -f deploy/observability/prometheus/operator/servicemonitor-dataplane.yaml
kubectl apply -f deploy/observability/prometheus/operator/prometheusrule-dataplane.yaml
```

Native Prometheus:

Copy one `scrape_configs` fragment from `native/prometheus-controlplane-scrape.yaml` and one from `native/prometheus-dataplane-scrape.yaml` into your deployed `prometheus.yml`, copy `native/prometheus-dataplane-rules.yaml` into a configured `rule_files` path, reload Prometheus, then verify the target count in `Status -> Targets`.
