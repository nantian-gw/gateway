# Documentation Index

The `docs/` directory is organized into two categories by target audience:

- Developer-facing: covers architecture, design, development workflow, testing, and contribution process.
- User-facing: covers deployment, operations, troubleshooting, and management interfaces.

## Documentation Map

| Document | Audience | Current Status | Primary Purpose |
| --- | --- | --- | --- |
| [developer/README.md](developer/README.md) | Developers | Covered | Developer entry point and recommended reading order |
| [roadmap.md](roadmap.md) | Users / Maintainers / External Evaluators | Covered | Public version roadmap, phase goals, and exit criteria |
| [backlog/README.md](backlog/README.md) | Maintainers / Developers | Covered | Long-term backlog navigation, split by control plane, data plane, release, and security |
| [development.md](development.md) | Developers | Covered | Local development, validation, integration testing, and contribution process |
| [architecture.md](architecture.md) | Developers | Covered | Control plane, data plane, IR, and boundary definitions |
| [design.md](design.md) | Developers | Basic coverage | Component decomposition, protocol, and runtime design |
| [design/ai-gateway/](design/ai-gateway/) | Developers | Covered | AI Gateway architecture, multi-format proxy, CRD, and observability design |
| [design/wasm/](design/wasm/) | Developers | Covered | Wasm plugin system architecture, PluginManager, user SDK, and AI sandbox |
| [skew-compatibility.md](skew-compatibility.md) | Developers / Release Maintainers | Covered | Version skew boundaries for controlplane, dataplane, and proto; upgrade contract and automation entry |
| [gateway-api-support.md](gateway-api-support.md) | Developers / Users | Basic coverage | Current Gateway API capability scope, support matrix, and gap notes |
| [implementation-review-packet.md](implementation-review-packet.md) | Users / Maintainers / External Evaluators | Covered | External technical review, Gateway API implementation claim preparation, and community evaluation material entry |
| [adopters-and-compatibility.md](adopters-and-compatibility.md) | Users / Maintainers / External Evaluators | Covered | Public adopter, case study, and compatibility evidence entry; clarifies which environments and evidence are already public and which remain unannounced. |
| [community-readiness.md](community-readiness.md) | Maintainers / External Evaluators | Covered | Gateway API implementation claim and CNCF community readiness boundaries |
| [external-review-evidence.md](external-review-evidence.md) | Maintainers / External Evaluators | Covered | Multi-maintainer, external contributor, adopter, case study, and public review evidence threshold ledger |
| [status-matrix.md](status-matrix.md) | Developers / Maintainers | Covered | Gateway API status matrix within currently declared support scope, reason/message semantics, and test anchors |
| [test/plan.md](test/plan.md) | Developers / Release Maintainers | Covered | Current repository test plan, verification layers, execution entry points, and pass criteria |
| [test/multi-environment-performance-baseline.md](test/multi-environment-performance-baseline.md) | Release Maintainers / External Evaluators | Covered | Multi-environment performance and capacity baseline evidence standards, distinguishing Kind, non-Kind lab, managed Kubernetes, and production canary |
| [security/structured-review-2026-03.md](security/structured-review-2026-03.md) | Maintainers / Release Maintainers | Covered | Current structured security audit conclusions, scope, evidence, and residual risks |
| [security/risk-register.md](security/risk-register.md) | Maintainers / Release Maintainers | Covered | Current security risk acceptance items and follow-up action ledger |
| [user/README.md](user/README.md) | Users | Covered | User entry point and reading path |
| [user/getting-started.md](user/getting-started.md) | Users | Covered | Kind / local process quick start |
| [user/install-profiles.md](user/install-profiles.md) | Users / Release Maintainers | Covered | `kind-dev`, production, and observability install profile matrix |
| [user/admin-api.md](user/admin-api.md) | Users | Basic coverage | Control plane and data plane management interface documentation |
| [../dashboard/README.md](../dashboard/README.md) | Users / Developers | Basic coverage | Next.js/React dashboard local preview, build, Node proxy, and admin API integration guide |
| [user/operations.md](user/operations.md) | Users | Covered | Production deployment, authentication, TLS/mTLS, upgrades, and certificate rotation |
| [user/release-runbook.md](user/release-runbook.md) | Users / Release Maintainers | Covered | Pre-release checks, canary steps, rollout thresholds, and rollback actions |
| [user/compatibility-notes.md](user/compatibility-notes.md) | Users / Release Maintainers | Covered | Release compatibility, upgrade, rollback, and skew notes entry |
| [proposals/README.md](proposals/README.md) | Developers / Maintainers | Covered | Design proposals, decision records, and review trail entry |

Status legend:

- `Covered`: Already supports the primary usage or development workflows for the current phase.
- `Basic coverage`: Has core content, but further details or examples are recommended.

## Developer Documentation

- [Developer Entry](developer/README.md)
- [Roadmap](roadmap.md)
- [Backlog Navigation](backlog/README.md)
- [Development Guide](development.md)
- [Architecture](architecture.md)
- [Design](design.md)
- [Version Skew Contract](skew-compatibility.md)
- [Gateway API Support Matrix](gateway-api-support.md)
- [Implementation Review Packet](implementation-review-packet.md)
- [External Review Evidence Ledger](external-review-evidence.md)
- [Gateway API Status Matrix](status-matrix.md)
- [Test Plan](test/plan.md)
- [Multi-Environment Performance Baseline Standard](test/multi-environment-performance-baseline.md)
- [Structured Security Audit](security/structured-review-2026-03.md)
- [Security Risk Acceptance Ledger](security/risk-register.md)
- [Design Proposal Entry](proposals/README.md)

Suitable for the following scenarios:

- Modifying control plane, data plane, proto, or deployment scripts.
- Adding new Gateway API capabilities, management interfaces, or observability features.
- Designing upgrade, rollback, and controlplane/dataplane skew validation paths.
- Troubleshooting behavioral inconsistencies between control plane and data plane.
- Designing pre-release validation paths, smoke, conformance, and rollback thresholds.

## User Documentation

- [User Entry](user/README.md)
- [Getting Started](user/getting-started.md)
- [Install Profile Matrix](user/install-profiles.md)
- [Management API](user/admin-api.md)
- [Production Operations](user/operations.md)
- [Release and Canary Runbook](user/release-runbook.md)
- [Compatibility Notes](user/compatibility-notes.md)
- [Gateway API Support Matrix](gateway-api-support.md)
- [Implementation Review Packet](implementation-review-packet.md)
- [Public Adopter / Case Study / Compatibility Matrix](adopters-and-compatibility.md)

Suitable for the following scenarios:

- Deploying Nantian Gateway for the first time locally or in Kind.
- Verifying whether traffic is being forwarded through Gateway API rules.
- Viewing current snapshots, routes, backends, and node status for control plane and data plane.
- Externally evaluating current public compatibility evidence and whether the repository already has a public adopter entry.

## Recommended Reading Order

If you are developing new features:

1. Start with [Architecture](architecture.md).
2. Then read [Design](design.md).
3. If you need to confirm existing capability boundaries, check the [Gateway API Support Matrix](gateway-api-support.md).
4. If picking tasks from the backlog, start with [Backlog Navigation](backlog/README.md).
5. Finally, follow the [Development Guide](development.md) for local development, testing, and Kind integration.
6. If changes involve cross-module trade-offs, compatibility, or upgrade paths, add a [Design Proposal](proposals/README.md).

If you are deploying and using:

1. Start with the [Gateway API Support Matrix](gateway-api-support.md) to confirm the current support scope.
2. Then read [Getting Started](user/getting-started.md).
3. If choosing a long-term environment or observability entry, see the [Install Profile Matrix](user/install-profiles.md).
4. If preparing for launch or canary rollout, read the [Release and Canary Runbook](user/release-runbook.md).
5. If evaluating upgrade or rollback impact, see [Compatibility Notes](user/compatibility-notes.md).
6. For external technical review or implementation claim preparation, see [Implementation Review Packet](implementation-review-packet.md).
7. To confirm what real evidence is still missing for external review, see [External Review Evidence Ledger](external-review-evidence.md).
8. To see public compatibility evidence and adopter/case study entry, see [Public Adopter / Case Study / Compatibility Matrix](adopters-and-compatibility.md).
9. To see whether performance and capacity already have multi-environment evidence, see [Multi-Environment Performance Baseline Standard](test/multi-environment-performance-baseline.md).
10. Finally, read [Management API](user/admin-api.md).
11. If you need machine-readable management surface path/auth/content-type contracts, see [Admin API Contract](contracts/admin-api-contract.md).

## What's Still Missing

The following documents are currently most worth completing:

- `OpenAPI / Swagger field-level generation pipeline`
   There is already an [Admin API Contract](contracts/admin-api-contract.md) and machine-readable [admin-api-surface.json](contracts/admin-api-surface.json), but no complete field-level OpenAPI / Swagger generation pipeline yet.
- `FAQ / Troubleshooting Guide`
   Currently the quick start and management API docs cover the main workflows, but common failure scenarios have not been consolidated into a dedicated document.
- `Deployment and Configuration Reference`
   There is currently a quick start, but no reference document specifically covering control plane configuration, data plane configuration, environment variables, and deployment manifest differences.

If continuing to add documentation, the recommended priority order is:

1. `FAQ / Troubleshooting Guide`
2. `Deployment and Configuration Reference`
3. More complete `OpenAPI / Swagger` field-level auto-generation pipeline

## Archive

Historical documents preserved for reference. See [archive/README.md](archive/README.md) for the full inventory.
