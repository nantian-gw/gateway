# Aether Gateway

A Kubernetes Gateway API implementation targeting v1.5.1 with 55 declared supported features.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](controlplane/go.mod)

## Overview

Aether Gateway provides a split-plane architecture for Kubernetes Gateway API. This repository contains the Go control plane, shared protobuf contract, deployment manifests, test suites, and documentation.

The project is organized across three sibling repositories:

| Repository | Description |
|---|---|
| **aether-gateway/aether-gateway** (this repo) | Go control plane, proto contract, deploy configs, tests, docs |
| **aether-gateway/dataplane** | Rust data plane (HTTP/stream proxy runtime, xDS client, AI gateway, Wasm plugins) |
| **aether-gateway/dashboard** | Next.js/React admin console and Node proxy server |
| **aether-gateway/website** | Project website and documentation site |

## Architecture

```
┌──────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                   │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │ Gateway  │  │ HTTPRoute│  │ EndpointSlice     │  │
│  └────┬─────┘  └────┬─────┘  └────────┬──────────┘  │
│       └──────────────┼───────────────┘               │
│                      │ watch                         │
│              ┌───────┴────────┐                      │
│              │  Control Plane  │                     │
│              │  (Go)           │                     │
│              │  Translator     │                     │
│              │  xDS Server     │                     │
│              └───────┬────────┘                      │
│                      │ gRPC/xDS                      │
│              ┌───────┴────────┐                      │
│              │  Data Plane     │                     │
│              │  (Rust)         │                     │
│              │  HTTP / Stream  │                     │
│              │  Admin API      │                     │
│              └────────────────┘                      │
└──────────────────────────────────────────────────────┘
```

## Quick Start

```bash
make proto
make build
make test-unit
```

## Features

- **Gateway API v1.5.1**: Gateway, HTTPRoute, GRPCRoute, UDPRoute, TLSRoute passthrough, BackendTLSPolicy, TCPRoute, and BackendLBPolicy support with conformance evidence.
- **Split-plane architecture**: Go control plane pushes configuration snapshots to data planes over gRPC bidirectional xDS streams.
- **Observability**: Admin APIs, Prometheus metrics, Grafana dashboard.
- **Production overlays**: Kustomize base and production overlay with release manifest rendering.
- **AI Gateway**: Multi-format AI proxy (OpenAI, Anthropic, Ollama) with token counting, rate limiting, API key management, PII masking, and A/B testing. See `docs/design/ai-gateway/`.
- **Wasm Plugin System**: wasmtime-based plugin runtime for custom request/response hooks. See `docs/design/wasm/`.

## Install

For a quick trial with Kind:

```bash
./tests/e2e/run-kind.sh
```

For longer-running environments:

```bash
kubectl apply -k deploy/kubernetes/overlays/production
```

See `docs/user/getting-started.md` and `deploy/README.md` for detailed setup instructions.

## Verify

Cheapest-first validation:

```bash
make proto
make build
make test-unit
```

Layered verification based on change scope:

```bash
./scripts/run-targeted-validation.sh
./tests/e2e/run-kind.sh
ALL_FEATURES=true ./tests/conformance/run.sh
```

## Documentation

| Entry point | Description |
|---|---|
| `docs/README.md` | Full doc index |
| `docs/user/getting-started.md` | User quick start |
| `docs/gateway-api-support.md` | Feature support matrix |
| `docs/development.md` | Developer guide |
| `docs/user/operations.md` | Operations and troubleshooting |
| `docs/user/admin-api.md` | Admin API reference |
| `docs/design/ai-gateway/` | AI Gateway architecture and CRDs |
| `docs/design/wasm/` | Wasm plugin architecture and SDK |
| `ROADMAP.md` | Project roadmap and milestones |
| `CONTRIBUTING.md` | Contribution guide |

Governance files: `LICENSE`, `MAINTAINERS.md`, `GOVERNANCE.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `VERSIONING.md`, `CHANGELOG.md`.

## Repository Layout

```
aether-gateway/
├── controlplane/   Go controller, translator, status, admin, xDS server
├── proto/          gRPC config discovery protocol (single source of truth)
├── deploy/         Kubernetes Kustomize base + overlays, observability assets
├── tests/          Unit tests, Kind smoke, conformance harness
├── docs/           Developer and user documentation
├── scripts/        Build, test, and release automation
└── configs/        Local runtime configs for controlplane and dataplane
```

## Project Status

Aether Gateway is suitable for continued Gateway API implementation work, internal evaluation, controlled trials, and contributor review. It has a working control plane, data plane, admin interfaces, Kind smoke tests, conformance workflow, production overlay, and open source governance materials. It is not yet presented as an officially recognized Gateway API implementation or a mature multi-maintainer open source community.

Latest conformance report: see `reports/conformance/`.

## License

Apache 2.0. See `LICENSE`.