# Implementation Review Packet

This is the external review entrypoint for Nantian Gateway. It aggregates the current architecture, Gateway API support matrix, conformance evidence, release/install/runbook materials, security posture, governance documents, maintainer state, and adopter/case-study status.

This packet is evidence navigation, not a certification claim. It does not mean Nantian Gateway is officially recognized by Gateway API, production-complete, or a mature CNCF community project.

---

This page is a material index for Nantian Gateway external technical review, Gateway API implementation claim preparation, and community assessment.
It is not equivalent to an official endorsement statement, nor does it prove that the project has reached mature CNCF community status. It simply aggregates currently available reviewable materials into a single entry point, making it easier for reviewers to quickly assess the evidence boundary.

## Current Status

As of `2026-05-12`:

- The project is a Kubernetes Gateway API implementation in progress.
- The control plane dependency and local verification scripts have aligned the default Gateway API version to `v1.5.1`.
- The current `GatewayClass.status.supportedFeatures` declares `55` features.
- The most recent clean full-suite conformance baseline is `3af22b42` from `2026-05-08`.
- The current `reports/conformance/latest/` points to the `0355945e` kind mesh profile from `2026-05-12`, not a full-suite.
- There are currently no named external adopters or public case studies.
- There is currently only one public maintainer; multi-maintainer community governance has not yet formed.

For external references, it is recommended to use the minimum communication stance in the [Community Readiness Checklist](community-readiness.md) rather than interpreting this page as an "officially recognized implementation."

## Review Entry Points

| Review Topic | Current Materials | Purpose |
| --- | --- | --- |
| Project Scope and Architecture | [README](../README.md), [architecture.md](architecture.md), [design.md](design.md) | Understand control plane, data plane, proto/IR, and runtime boundaries. |
| Gateway API Support Scope | [gateway-api-support.md](gateway-api-support.md), [status-matrix.md](status-matrix.md) | Review the four-level boundary (declared / implemented / tested / production-validated) and status semantics. |
| Conformance Evidence | [reports/conformance/README.md](../reports/conformance/README.md), [test/latest-baseline.md](test/latest-baseline.md) | Distinguish between latest archive, clean full-suite baseline, and subsequent targeted validation. |
| Experimental Expansion Boundary | [backlog/gateway-api-experimental.md](backlog/gateway-api-experimental.md) | Determine which experimental capabilities are supported and which are explicitly deferred. |
| Installation and Release | [user/install-profiles.md](user/install-profiles.md), [user/release-runbook.md](user/release-runbook.md), [deploy production overlay](../deploy/kubernetes/overlays/production/README.md) | Review installation profiles, release gates, canary, and rollback procedures. |
| Version Compatibility | [skew-compatibility.md](skew-compatibility.md), [contracts/versioning.md](contracts/versioning.md), [VERSIONING.md](../VERSIONING.md) | Review controlplane/dataplane/proto skew and version support stance. |
| Security and Supply Chain | [SECURITY.md](../SECURITY.md), [security risk register](security/risk-register.md), [structured security review](security/structured-review-2026-03.md) | Review security disclosure, risk acceptance, supply chain, and production security boundaries. |
| Governance and Maintainers | [GOVERNANCE.md](../GOVERNANCE.md), [MAINTAINERS.md](../MAINTAINERS.md), [CODEOWNERS](../CODEOWNERS), [CONTRIBUTING.md](../CONTRIBUTING.md) | Review roles, ownership, review routing, and contribution process. |
| External Evidence Threshold | [external-review-evidence.md](external-review-evidence.md) | Review whether multi-maintainer, external contributor, adopter, case study, release history, and public review evidence have met the closure criteria. |
| Roadmap and Backlog | [docs/roadmap.md](roadmap.md), [backlog/README.md](backlog/README.md), [ROADMAP.md](../ROADMAP.md) | Review phase goals, exit criteria, and still-incomplete production evidence. |
| Adopter and Compatibility | [adopters-and-compatibility.md](adopters-and-compatibility.md) | Review public adopter/case study entry point and compatibility matrix. Currently no named adopters. |

## Currently Supportable Claims

Can support:

- The project has an independent control plane and Rust/Rust proxy data plane.
- The project has integrated the Gateway API conformance harness with archived full-suite passing evidence.
- The project has a clear Gateway API support matrix and unsupported boundary.
- The project has basic governance, maintainer, contribution, security, versioning, and release documentation.
- The project has a production Kustomize overlay, release manifest rendering, and basic release evidence gate.

Cannot support:

- Cannot claim to be an officially recognized Gateway API implementation.
- Cannot claim to have met mature CNCF community project conditions.
- Cannot claim multi-maintainer community governance or named external adopters.
- Cannot claim `24h/72h` soak or production-grade node drain / apiserver-watch disruption is complete. Currently only Kind release-gate fault injection archived evidence exists.
- Cannot claim all Gateway API features, HTTP/3 / QUIC, ListenerSet, TLSRoute terminate/mixed mode, or full BackendLBPolicy are supported.

## External Review Checklist

| Check Item | Current Status | Description |
| --- | --- | --- |
| Code dependencies, CRD bundle, and conformance Gateway API version alignment | Ready | Current default version is `v1.5.1`. |
| Support matrix and supportedFeatures traceability | Ready | `docs/gateway-api-support.md` is aligned with `controlplane/internal/gatewayapi/supported_features.go`. |
| Clean full-suite conformance archive | Ready | Most recent clean full-suite is `2026-05-13-46f1956c-full`. |
| Current commit full-suite evidence | Not guaranteed | New externally referenced commits should still re-archive full-suite. |
| Release / install / rollback documentation | Basic readiness | Production overlay, install profiles, and release runbook exist. |
| Long-running and fault injection | Partially complete | Kind fault injection release gate passed at `reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults/`. Actual `24h` soak and production-like environment fault injection are not yet complete. |
| Public governance materials | Basic readiness | Code of conduct, contributing, governance, maintainers, security, and versioning documentation exist. |
| Multi-maintainer and external contributor evidence | Not complete | Current public maintainer is still a single person. Closure criteria in [external-review-evidence.md](external-review-evidence.md). |
| Public adopter / case study | Not complete | Page entry exists but no named entries yet. Closure criteria in [external-review-evidence.md](external-review-evidence.md). |

## Maintenance Rules

- When `reports/conformance/latest/`, clean full-suite baseline, support matrix, or release evidence is updated, check whether this page is still accurate.
- When adding or removing a supported feature, update [gateway-api-support.md](gateway-api-support.md) and this page’s review checklist in sync.
- When public adopter, case study, reviewer, or maintainer changes occur, update [external-review-evidence.md](external-review-evidence.md), [adopters-and-compatibility.md](adopters-and-compatibility.md), [community-readiness.md](community-readiness.md), and this page in sync.
- This page can only aggregate existing evidence. Plans, intentions, or incomplete validations should not be written as completed status.
