# 0004: Defer HTTP/3 and QUIC Downstream Support

Status: Accepted  
Date: 2026-04-30

## Context

HTTP/3 and QUIC support would add a distinct downstream transport with separate
listener lifecycle, TLS/ALPN behavior, UDP exposure, conformance expectations,
performance baselines and operational troubleshooting.

The current project still has higher-priority work around Gateway API contract
clarity, production overlays, long-running soak evidence, resource usage,
hot-reload behavior, p99 latency and release evidence.

## Decision

Do not implement HTTP/3 or QUIC downstream support in the current milestone.

Keep the current implementation focused on:

- HTTPRoute and GRPCRoute over HTTP/1.1, HTTP/2, h2c and HTTPS termination
- TCPRoute, UDPRoute and TLSRoute stream runtime
- Gateway API conformance and production-readiness gaps outside HTTP/3
- stable admin, metrics, release and performance evidence

## Alternatives Considered

- Implement HTTP/3 immediately: could improve protocol coverage, but increases
  complexity before the existing HTTP/gRPC/stream paths are sufficiently proven.
- Add an experimental hidden HTTP/3 flag: helps prototyping, but risks confusing
  support claims and test coverage unless it is fully isolated.
- Permanently reject HTTP/3: too restrictive; the protocol can be valuable once
  the production baseline is more mature.

## Consequences

- Documentation and support matrices must not imply HTTP/3 production support.
- Gateway API support claims should distinguish implemented/tested features from
  deferred features.
- QUIC-specific performance, security and operational work is intentionally
  excluded from current release gates.

## Revisit / Rollback Conditions

Revisit this decision after conformance, production overlay, performance
baseline, hot-reload behavior and soak evidence are stable enough that a new
transport can be added without masking more fundamental readiness gaps.
