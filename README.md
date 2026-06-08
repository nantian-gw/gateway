# Nantian Gateway

<p align="center">
  <strong>A modern Kubernetes Gateway API implementation with split-plane architecture, built for production.</strong>
</p>

<p align="center">
  <a href="https://github.com/nantian-gw/gateway/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go" alt="Go"></a>
  <a href="https://gateway-api.sigs.k8s.io/"><img src="https://img.shields.io/badge/Gateway_API-v1.5.1-326ce5?logo=kubernetes" alt="Gateway API"></a>
  <a href="https://nantian.dev"><img src="https://img.shields.io/badge/docs-nantian.dev-7c3aed" alt="Docs"></a>
</p>

---

## What is Nantian Gateway?

Nantian Gateway is a [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) implementation that handles ingress traffic, API routing, and AI gateway features — all using standard Kubernetes resources. No custom CRDs for routing. No proprietary config language. Just Gateway API.

**If you've used nginx ingress or Envoy Gateway** — Nantian Gateway does the same job, but with a Go control plane and a Rust data plane, targeting full Gateway API v1.5.1 conformance with 55 supported features.

### Why Nantian Gateway?

| Problem | Nantian Gateway's answer |
|---|---|
| **Vendor lock-in** | Standard Gateway API — switch implementations without changing route definitions |
| **Complex AI routing** | Built-in AI Gateway: multi-provider proxy, API keys, rate limiting, PII masking |
| **Observability gaps** | Prometheus metrics, Grafana dashboards, and admin APIs out of the box |
| **Performance at scale** | Rust data plane with xDS push — sub-millisecond config propagation |
| **Custom logic** | Wasm plugin system for request/response hooks without rebuilding |

### Architecture at a Glance

```
  Users / Clients
        │
        ▼
  ┌───────────┐
  │  Data Plane │  ◄── Rust proxy (HTTP, gRPC, UDP, TLS)
  │  (Rust)     │      Handles live traffic
  └─────┬─────┘
        │ gRPC xDS (bidirectional stream)
  ┌─────┴─────┐
  │  Control    │  ◄── Go process
  │  Plane (Go) │      Watches Gateway API resources
  └─────┬─────┘      Translates → xDS config → pushes to data planes
        │
  ┌─────┴─────┐
  │ Kubernetes │  Gateway, HTTPRoute, GRPCRoute, TLSRoute…
  │    API     │
  └───────────┘
```

## Quick Start

Try it in 5 minutes with a local Kind cluster:

```bash
# Clone and deploy
git clone https://github.com/nantian-gw/gateway.git
cd gateway

# Start a Kind cluster with Nantian Gateway
./test/e2e/smoke/run.sh
```

Now create your first route:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-first-route
spec:
  parentRefs:
    - name: nantian-gateway
  rules:
    - backendRefs:
        - name: my-service
          port: 8080
```

For a complete walkthrough, see our [Getting Started guide](https://nantian.dev).

## Installation

### Helm (recommended for production)

```bash
helm repo add nantian-gw https://nantian-gw.github.io/helm-charts
helm install nantian-gw nantian-gw/nantian-gw \
  --namespace nantian-gw \
  --create-namespace
```

See [Helm chart documentation](https://github.com/nantian-gw/helm-charts) for custom values.

### Kustomize

```bash
kubectl apply -k deploy/kubernetes/overlays/production
```

### Requirements

- Kubernetes 1.28+
- [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) installed on the cluster

## Key Features

### Gateway API v1.5.1 Support

| Route Type | Status |
|---|---|
| HTTPRoute | ✅ Fully supported |
| GRPCRoute | ✅ Fully supported |
| TCPRoute | ✅ Fully supported |
| UDPRoute | ✅ Fully supported |
| TLSRoute | ✅ Passthrough |
| BackendTLSPolicy | ✅ Fully supported |
| BackendLBPolicy | ✅ Fully supported |

See the [full conformance report](reports/conformance/) for details.

### AI Gateway

Route AI traffic to multiple providers with a single endpoint:

- **Unified proxy** — OpenAI, Anthropic, Ollama behind one endpoint
- **Token counting & rate limiting** — per-user, per-model quotas
- **API key management** — centralized credential storage via Kubernetes Secrets
- **PII masking** — automatic detection and redaction of sensitive fields
- **A/B testing** — split traffic across models or providers

```yaml
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIBackend
metadata:
  name: my-llm
spec:
  provider: openai
  model: gpt-4o
  apiKeySecretRef:
    name: openai-credentials
```

→ [AI Gateway documentation](docs/design/ai-gateway/)

### Wasm Plugin System

Extend the data plane with custom logic without rebuilding or restarting:

- **Request/response hooks** — modify headers, bodies, or status codes
- **wasmtime runtime** — fast, sandboxed execution
- **Write in any language** — compile to Wasm from Rust, Go, C, or JavaScript

→ [Wasm plugin documentation](docs/design/wasm/)

### Observability

- **Prometheus metrics** — request rate, latency, errors by route and backend
- **Grafana dashboards** — pre-built templates in `deploy/observability/`
- **Admin API** — runtime configuration, health checks, diagnostic endpoints

## Documentation

| You want to… | Go here |
|---|---|
| Get started | [Getting Started](https://nantian.dev/getting-started/quick-start/) |
| Install in production | [Installation Guide](https://nantian.dev/installation/helm/) |
| Understand concepts | [Concepts](https://nantian.dev/concepts/) |
| Set up AI Gateway | [AI Gateway docs](docs/design/ai-gateway/) |
| Write a Wasm plugin | [Wasm SDK docs](docs/design/wasm/) |
| See what's supported | [Gateway API Support Matrix](docs/gateway-api-support.md) |
| Contribute | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Report a bug | [Issues](https://github.com/nantian-gw/gateway/issues) |
| See the roadmap | [ROADMAP.md](ROADMAP.md) |

[Full documentation site →](https://nantian.dev)

## Development

```bash
# Prerequisites: Go 1.26+, Rust (for data plane), Kind (for e2e)

# Generate protobuf
make proto

# Build
make build

# Run unit tests
make test

# Run benchmarks
make benchmarks

# Run conformance suite (requires Kind)
make conformance
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development workflow.

## Related Projects

| Project | Description |
|---|---|
| [nantian-gw/dataplane](https://github.com/nantian-gw/dataplane) | Rust data plane (HTTP proxy, xDS client, AI gateway, Wasm runtime) |
| [nantian-gw/dashboard](https://github.com/nantian-gw/dashboard) | Next.js admin console |
| [nantian-gw/website](https://github.com/nantian-gw/website) | Documentation site ([nantian.dev](https://nantian.dev)) |
| [nantian-gw/helm-charts](https://github.com/nantian-gw/helm-charts) | Helm charts for Kubernetes deployment |
| [nantian-gw/proto](https://github.com/nantian-gw/proto) | Shared protobuf contract |

## Project Status

Nantian Gateway is under active development. It has a working control plane, data plane, admin interfaces, Kind smoke tests, conformance workflows, and production deployment overlays. It is not yet an officially recognized Gateway API implementation.

## License

Apache 2.0 — see [LICENSE](LICENSE).
