# User Documentation

This documentation entry point is intended for triers, deployers, and troubleshooters.
If you need to modify code, add tests, or adjust repository structure, see [Developer Entry](../developer/README.md).

## Suggested Reading Order

1. [Gateway API Support Matrix](../gateway-api-support.md): First confirm which Gateway API capabilities are already supported by the current version.
2. [Getting Started](getting-started.md): Start the system using Kind or local process mode.
3. [Install Profile Matrix](install-profiles.md): Choose from `kind-dev`, `kind-hostnetwork-perf`, `single-cluster-prod`, `multi-replica-prod`, or `observability-enabled` install entry points.
4. [Traffic Profile Examples](traffic-profiles.md): Choose north-south `Gateway`, north-south TCP/UDP, or east-west service parent.
5. [Admin API](admin-api.md): View health status, snapshots, listeners, routes, backends, and node information.
6. [Dashboard README](../../dashboard/README.md): When using or developing the Web admin console, see local preview, build, and Node proxy instructions.
7. [Production Operations](operations.md): Enable authentication, TLS/mTLS, long-term environment configuration, and certificate rotation.
8. [Release, Canary, and Rollback Runbook](release-runbook.md): Perform pre-launch checks, canary rollout, and rollback.
9. [Implementation Review Packet](../implementation-review-packet.md): For external technical review, implementation claim preparation, and pre-open-source evidence checks.

## Applicable Scenarios

- Running Nantian Gateway for the first time.
- Confirming whether the current version supports the Gateway API capabilities you need before integration.
- Validating basic Gateway, HTTPRoute, GRPCRoute, TCPRoute, UDPRoute, and TLSRoute behavior in Kind.
- Choosing north-south / east-west traffic access patterns for production or long-term environments.
- Viewing current snapshots and node status via the admin API.
- Using the dashboard as a Web management console for the controlplane / dataplane admin API.
- Performing basic troubleshooting without diving into source code.
- Preparing rollout thresholds and rollback actions for release windows.
- Confirming current public evidence and remaining boundaries for external technical review.

## Current Recommendations

- For first-time trials, prefer [Kind Getting Started](getting-started.md).
- If you only need to see current status, skip the design docs and directly use the [Admin API](admin-api.md).
- If preparing to move a version to a long-term environment, first see the [Install Profile Matrix](install-profiles.md) and [Traffic Profile Examples](traffic-profiles.md), then [Production Operations](operations.md) and [Release, Canary, and Rollback Runbook](release-runbook.md).
- If preparing for open-source, external presentation, or evaluating implementation claims, first see the [Implementation Review Packet](../implementation-review-packet.md), [Community Readiness Checklist](../community-readiness.md), and [Public Adopter / Case Study / Compatibility Matrix](../adopters-and-compatibility.md).
- Only switch to developer documentation when source-level debugging is needed.