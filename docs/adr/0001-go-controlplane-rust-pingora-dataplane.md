# 0001: Split Go Control Plane and Rust Rust proxy Data Plane

Status: Accepted  
Date: 2026-04-30

## Context

Aether Gateway needs to watch Kubernetes Gateway API resources, reconcile
status and derived infrastructure, publish runtime snapshots, and proxy
multi-protocol traffic efficiently.

These responsibilities have different runtime constraints:

- Kubernetes controller logic benefits from the Go ecosystem, controller-runtime,
  Gateway API types, fake clients, status reconcilers and established operator
  patterns.
- High-throughput HTTP/gRPC forwarding benefits from Rust, Rust proxy and stricter
  ownership around hot-path data structures.
- Stream routes, TLS assets, xDS apply state, access logging and admin
  introspection need to evolve independently from Kubernetes watch mechanics.

## Decision

Use Go for the control plane and Rust/Rust proxy for the data plane.

The Go control plane owns Kubernetes integration, translation, status,
infrastructure reconciliation, admin APIs, metrics and snapshot publication.

The Rust data plane owns request forwarding, listener/runtime apply, HTTP/gRPC
proxying, stream routing, access logs, dataplane admin APIs and runtime metrics.

The boundary between the two planes is a versioned protobuf snapshot stream.

## Alternatives Considered

- All-Go gateway: simpler build and debugging story, but weaker fit for the
  desired Rust proxy data path and lower confidence for hot-path optimization.
- All-Rust controller and data plane: one language and stronger data-path
  consistency, but substantially higher cost for Kubernetes controller-runtime
  parity, fake-client testing and status/controller ecosystem integration.
- Embed Rust proxy behind a Go control process: reduces deployment count, but
  couples Kubernetes reconciliation lifecycles to forwarding lifecycles and
  makes safe hot reload harder.

## Consequences

- The repository must maintain clear controlplane/dataplane module boundaries.
- Protobuf compatibility and snapshot defaults are part of the product contract.
- Tests should validate both local plane behavior and cross-plane skew.
- Operationally, control plane and data plane images can be rolled, scaled and
  debugged independently.

## Revisit / Rollback Conditions

Revisit this decision if Rust proxy is no longer the preferred forwarding runtime,
or if maintaining the cross-language protocol becomes a larger reliability risk
than the benefits of independent data-plane optimization.
