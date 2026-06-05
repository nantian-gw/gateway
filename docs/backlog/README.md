# Backlog Index

This directory organizes the long-term engineering backlog from the root `ROADMAP.md` and provides shorter reading entry points by topic.
The root `ROADMAP.md` remains the authoritative source for the full backlog; this directory only provides topic views such as control plane, data plane, release, and security, and does not replace or compress the root inventory.

## Documents

| Document | Scope |
| --- | --- |
| [controlplane.md](controlplane.md) | Control plane performance, status, infrastructure, Gateway API gaps, and large file splitting |
| [dataplane.md](dataplane.md) | Data plane forwarding efficiency, resource usage, p99, success rate, hot reload, and module splitting |
| [gateway-api-experimental.md](gateway-api-experimental.md) | Gateway API experimental capability assessment, deferral boundaries, and reassessment conditions |
| [release.md](release.md) | Release gate, installation profile, upgrade/rollback, report archiving, SBOM/provenance/digest |
| [security.md](security.md) | Supply chain, RBAC, secret/token/certificate rotation, mTLS, admin authentication |
| [../roadmap.md](../roadmap.md) | External release roadmap and phase exit criteria |

## Backlog Rules

- Each entry must have a clear acceptance criterion, specifying at minimum unit tests, script checks, kind, conformance, or report evidence.
- Entries requiring kind, conformance, soak, or performance stress testing must not be marked complete without evidence.
- Completed items should keep only a brief completion note, with detailed history in commit, report, ADR, or proposal.
- The root `ROADMAP.md` should not be reduced to an index; if important backlog items are added or closed here, the corresponding root entries must be updated in sync.
- Roadmaps, long-term performance research, production gaps, security records, and historical completion notes should no longer be written only in scattered documents, to avoid drift between the root backlog and topic views.

## Deferred Scope Guard

Until P0/P1 items are cleared, the following directions are only permitted to retain design, research, or risk records, and must not enter default installation, release gate, or `GatewayClass.status.supportedFeatures` expansion:

| Deferred Direction | Current Approach | Reassessment Trigger |
| --- | --- | --- |
| HTTP/3 / QUIC | Keep ADR and backlog, no default listener or conformance declaration | Submit proposal after v0.2/v0.3 baseline stabilizes |
| Operator | Only as a packaging backlog, does not replace Kustomize / release manifest | After production overlay, release evidence, and upgrade/rollback process stabilize |
| Complex incremental xDS patch | Keep full snapshot + partial rebuild optimization path | After reload-under-load and p99 evidence proves snapshot swap is the bottleneck |
| Default allocator switch | Only allowed as a reference profile in performance reports | After multi-environment performance baselines prove benefit with no RSS / stability regression |
| Large plugin system | Not entering core architecture for now | After core Gateway API subset, governance, and security boundaries stabilize |
| Additional Gateway API Experimental features | Current `v1.5.1` experimental audit complete, no further supportedFeatures expansion | After all reassessment conditions in `gateway-api-experimental.md` are met |

These entries are not "never do", but require completing a verifiable closed loop of Gateway API, release, performance, and production evidence before expanding the protocol surface, installation form, or runtime complexity.

## Current Highest Priorities

- Control plane scoped reconcile, reducing full status / infrastructure recomputation.
- Real data plane forwarding performance baseline, covering HTTP/gRPC/stream/TCP/UDP and reload-under-load.
- `24h` soak, node drain, apiserver/watch jitter release gate.
- Production install profile matrix and release asset digest / SBOM / provenance.
- Secret, certificate, admin token, mTLS, and session persistence secret rotation without restart verification.
