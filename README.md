# Nantian Gateway

<p align="center">
  <strong>A Kubernetes Gateway API control plane for split-plane ingress, API routing, and AI gateway workloads.</strong>
</p>

<p align="center">
  <a href="https://github.com/nantian-gw/gateway/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go" alt="Go"></a>
  <a href="https://gateway-api.sigs.k8s.io/"><img src="https://img.shields.io/badge/Gateway_API-v1.5.1-326ce5?logo=kubernetes" alt="Gateway API"></a>
  <a href="https://nantian.dev"><img src="https://img.shields.io/badge/docs-nantian.dev-7c3aed" alt="Docs"></a>
</p>

> [Chinese README](README.zh-CN.md)

## What Is Nantian Gateway?

Nantian Gateway is a Kubernetes Gateway API implementation with a Go control plane and a Rust data plane. This repository contains the control plane: it watches Gateway API resources, translates them into internal routing state, serves operational and admin APIs, and publishes runtime snapshots to data planes over gRPC/xDS.

Use Nantian Gateway when you want standard Kubernetes Gateway API resources for ingress traffic, API routing, and AI gateway workloads without inventing a custom routing CRD or proprietary configuration language.

## Architecture

```text
Kubernetes API
  Gateway, HTTPRoute, GRPCRoute, TLSRoute, policies, Services, Secrets
        |
        v
Go control plane (this repository)
  watch -> translate -> validate/status -> publish snapshots
        |
        | gRPC/xDS
        v
Rust data plane
  HTTP, gRPC, TCP, UDP, TLS, AI gateway, and Wasm runtime traffic handling
        |
        v
Backends and AI providers
```

The control plane is designed to stay Kubernetes-native. Gateway API resources remain the source of truth. The Rust data plane consumes the translated runtime model and handles live traffic. Optional sibling projects provide the Helm chart, Dashboard, Website, and shared Proto contract.

## Install

### Helm

Helm is the recommended production installation path:

```bash
helm repo add nantian-gw https://chart.nantian.dev
helm install nantian-gw nantian-gw/nantian-gw \
  --namespace nantian-gw \
  --create-namespace
```

See the [Helm chart repository](https://github.com/nantian-gw/helm-charts) for values, schema validation, and chart release details.

### Kustomize

Repository-local Kubernetes overlays are available under `deploy/kubernetes/`:

```bash
kustomize build deploy/kubernetes/overlays/production \
  --load-restrictor LoadRestrictionsNone \
  | kubectl apply -f -
```

### Requirements

- Kubernetes 1.28 or newer.
- Gateway API CRDs installed on the cluster.
- A compatible Nantian data plane deployment.

## Verify Locally

For a local smoke test with Kind, run:

```bash
make e2e-smoke
```

The smoke test creates a local cluster, installs required resources, deploys the gateway stack, and verifies a basic route path.

For comprehensive e2e scenario testing (multi-route, header/query matching, URL rewrite, traffic splitting, TLS, error handling), run:

```bash
make e2e
```

For Gateway API conformance testing, run:

```bash
make conformance
```

All commands require local Kubernetes tooling such as Kind, kubectl, and kustomize.

## Gateway API Support

Nantian Gateway targets Gateway API v1.5.1. Use the conformance package and supported-feature declarations as the local source of truth for exact support status:

- [Conformance tests](conformance/)
- [Gateway API support matrix](docs/gateway-api-feature-support.md)
- [Gateway API support tool](cmd/gateway-api-support/)
- [Supported feature declarations](internal/gatewayapi/supported_features.go)

The public documentation site also summarizes supported scenarios at [nantian.dev](https://nantian.dev).

## AI Gateway And Extensions

Nantian Gateway can route AI traffic and extension behavior through Kubernetes-managed configuration:

- AI provider routing and model selection.
- Token policy and quota-oriented extension resources.
- WasmPlugin resources for sandboxed request and response extension hooks.
- Backend TLS and load-balancing policies for upstream behavior.

Related control-plane source areas:

- `internal/gatewayapiexperimental/`
- `internal/translator/ai_service.go`
- `internal/translator/token_policy.go`
- `internal/translator/wasm_plugin.go`

## Operations And Observability

The gateway control plane exposes operational surfaces for production use:

- Admin APIs for health, topology, runtime state, and diagnostics.
- Prometheus metrics for controller, admin, and gRPC publication paths.
- Controlplane tracing can be enabled with `deploy/kubernetes/overlays/observability-enabled/` when you want OTLP trace export from the controlplane without changing the default production overlay.
- Deployment overlays under `deploy/kubernetes/`.
- Observability assets under `deploy/observability/`.
- Generated admin API contracts under `docs/contracts/`.

## Development

Common commands:

```bash
make build
make test
go test ./internal/translator
make e2e-smoke
make e2e
make conformance
```

Protobuf source and generation workflow live in the sibling Proto repository. Generated protobuf code under `gen/` should not be edited by hand.

For agent-specific repository guidance, see [AGENTS.md](AGENTS.md). For community expectations, see [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Related Repositories

| Repository | Purpose |
|---|---|
| [nantian-gw/dataplane](https://github.com/nantian-gw/dataplane) | Rust data plane for traffic handling and xDS consumption. |
| [nantian-gw/helm-charts](https://github.com/nantian-gw/helm-charts) | Helm chart for Kubernetes installation. |
| [nantian-gw/dashboard](https://github.com/nantian-gw/dashboard) | Next.js admin console. |
| [nantian-gw/website](https://github.com/nantian-gw/website) | Documentation site at [nantian.dev](https://nantian.dev). |
| [nantian-gw/proto](https://github.com/nantian-gw/proto) | Shared protobuf contract. |

## Project Status

Nantian Gateway is under active development. It has a working control plane, data plane integration, admin APIs, Kind smoke tests, conformance workflows, and production deployment overlays. It is not yet an officially recognized Gateway API implementation.

## License

Apache 2.0. See [LICENSE](LICENSE).
