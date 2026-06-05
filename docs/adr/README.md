# Architecture Decision Records

This directory records durable architecture decisions for Nantian Gateway.

ADR format:

- Status
- Date
- Context
- Decision
- Alternatives Considered
- Consequences
- Revisit / Rollback Conditions

Index:

- [0001: Split Go Control Plane and Rust Rust proxy Data Plane](0001-go-controlplane-rust-nantian-dataplane.md)
- [0002: Use a Project-Owned Snapshot Protocol Instead of Envoy xDS](0002-project-owned-snapshot-protocol.md)
- [0004: Defer HTTP/3 and QUIC Downstream Support](0004-defer-http3-quic.md)
- [0005: Make the Production Overlay Secure by Default](0005-production-overlay-secure-defaults.md)
