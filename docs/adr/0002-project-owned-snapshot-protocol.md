# 0002: Use a Project-Owned Snapshot Protocol Instead of Envoy xDS

Status: Accepted  
Date: 2026-04-30

## Context

Aether Gateway needs to deliver Gateway API-derived configuration from the Go
control plane to the Rust data plane. The runtime model includes Gateway API
listeners/routes, stream routes, TLS material, backend policy output, mesh
frontend metadata, workload hints and dataplane ACK/readiness status.

Envoy xDS is a mature ecosystem protocol, but directly adopting Envoy xDS would
force this project to map Gateway API semantics through Envoy resource types
even though the data plane is Rust proxy, not Envoy.

## Decision

Use a project-owned protobuf protocol and snapshot model for the control-plane
to data-plane contract.

The protocol remains xDS-like in lifecycle shape:

- long-lived gRPC stream
- complete snapshots with versions
- node reports and ACKs
- server-side publish/observe loop

But the payload is a Aether Gateway IR, not Envoy LDS/RDS/CDS/EDS.

## Alternatives Considered

- Reuse Envoy xDS resources: gains ecosystem familiarity, but adds translation
  layers that do not naturally match Rust proxy runtime structures.
- Use Kubernetes API watches directly from each data plane Pod: avoids a custom
  protocol, but increases data-plane permissions, Kubernetes API load and
  per-node reconciliation complexity.
- File-based config reload: easy to inspect, but slower and weaker for
  multi-node ACK/readiness tracking.

## Consequences

- The proto schema needs explicit compatibility discipline for new fields,
  defaults, old data-plane behavior and NACK/error reporting.
- The control plane can publish exactly the normalized runtime model required by
  the Rust crates.
- The data plane is not coupled to Envoy implementation details.
- External compatibility with generic Envoy xDS clients is not a goal of the
  current implementation.

## Revisit / Rollback Conditions

Revisit this decision if the project needs to interoperate with external Envoy
xDS control planes or clients, or if a future Rust proxy-native standard protocol
emerges and is better aligned than the current project-owned protobuf contract.
