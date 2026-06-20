# Distributed Tracing Configuration

Nantian Gateway supports OpenTelemetry distributed tracing via OTLP (gRPC).

## Control Plane

Enable tracing in the control plane config:

```yaml
tracing:
  enabled: true
  endpoint: "otel-collector:4317"
  insecure: true
  samplerRatio: 0.1
  headers:
    x-api-key: "my-key"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable OTLP tracing |
| `endpoint` | string | — | OTLP gRPC collector endpoint |
| `insecure` | bool | `false` | Use plaintext (no TLS) |
| `samplerRatio` | float | `1.0` | Sampling ratio (0.0—1.0) |
| `headers` | map | — | Custom gRPC metadata headers |

### Instrumented Operations

| Span Name | Component | Attributes |
|---|---|---|
| `admin GET/POST /v1/...` | HTTP Admin | http.method, http.route, http.status_code |
| `controlplane.syncer.publish_snapshot` | Controller | scope, gateway/route/backend counts |
| `controlplane.infrastructure.reconcile` | Infrastructure | managed_gateways, service/endpoint counts |
| gRPC server spans | otelgrpc | rpc.service, rpc.method |

## Data Plane

Enable tracing in the data plane config:

```yaml
observability:
  tracing:
    level: "info"
    format: "json"
    open_telemetry:
      enabled: true
      endpoint: "http://otel-collector:4317"
      protocol: "grpc"
      timeout_ms: 3000
      insecure: true
      sample_ratio: 0.1
      service_name: "nantian-dataplane"
      service_namespace: "production"
      service_instance_id: "dp-01"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable OTLP tracing |
| `endpoint` | string | — | OTLP collector URL |
| `protocol` | string | `grpc` | OTLP protocol |
| `sample_ratio` | float | `1.0` | Sampling ratio |
| `service_name` | string | `nantian-dataplane` | Service name in traces |

### Instrumented Operations

| Span Name | Component | Attributes |
|---|---|---|
| Request span (auto) | Proxy | http.method, url.path, client.address, server.address |
| Request enriched | Proxy | gateway.listener, route.name/namespace/kind, backend |
| Response enriched | Proxy | http.status_code, retry.attempts, response_flags |
| `ai.inference` | AI Gateway | ai.model, ai.format, ai.stream, prompt_tokens, completion_tokens |
| `ai.first_token` | AI Gateway | ai.first_token_ms |

### W3C Trace Context Propagation

- Incoming `traceparent` headers are extracted and continued
- Outgoing `traceparent` headers are injected to upstream backends
- gRPC xDS streams propagate trace context (via otelgrpc)

## End-to-End Trace Flow

```
Client Request (traceparent)
  → Data Plane Proxy (extract + continue)
    → AI Gateway Filter (child span: ai.inference)
    → Upstream Backend (inject traceparent)
  ← Data Plane Proxy (enrich + end)
```

Control plane reconciliation traces independently:
```
CRD Change Event
  → Controller Reconciler (span: publish_snapshot)
    → Infrastructure Reconciler (span: reconcile)
  → gRPC xDS Push (span: otelgrpc)
```

## Viewing Traces

Use any OTLP-compatible backend:
- **Jaeger**: `jaeger-all-in-one` with OTLP gRPC on port 4317
- **Grafana Tempo**: OTLP endpoint
- **OpenTelemetry Collector**: Forward to any backend
