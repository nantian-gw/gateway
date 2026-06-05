# Governance

This project uses a lightweight governance model. Decisions are clear, responsibilities are explicit, and releases are controlled, without unnecessary process overhead.

## Project Stage

The project is in rapid technical convergence with ongoing documentation and test completion. Governance is maintainer-driven.

## Decision Model

Day-to-day decisions are made by maintainers under these principles:

- Correctness takes priority over feature accumulation.
- Validate at the cheapest level first, then escalate only when necessary.
- One change addresses one class of problem. Avoid mixed commits that are hard to roll back.
- The control plane, data plane, protocol, and documentation evolve together.

Changes requiring more careful review: `proto/` compatibility, admin API contracts, Gateway API behavioral semantics, deployment manifests, security boundaries, certificate or permission models, and test framework changes that affect conformance or e2e baselines.

Review routing is documented in [`CODEOWNERS`](CODEOWNERS) and [`MAINTAINERS.md`](MAINTAINERS.md). These define default review assignment, not branch protection enforcement.

## Change Acceptance

A change must have a clear objective with well-scoped impact, validation matched to its scope, documentation, configuration examples, and tests updated together, and no unexplained compatibility breaks. Changes affecting release notes, compatibility notes, admin APIs, deployment defaults, or skew contracts require explicit impact disclosure during submission and review.

## Release and Stability

Before a release: control plane and data plane unit tests pass, required smoke or conformance validation passes, the support matrix and known limitations are current, and risks and rollback procedures are documented.

## Roles

- **Maintainer**: Ongoing review, merge, and release duties with corresponding permissions.
- **Reviewer**: First-pass review within assigned ownership areas. No merge or release permissions by default.

Named roles are listed in [`MAINTAINERS.md`](MAINTAINERS.md). The project remains maintainer-driven.

## Reviewer to Maintainer Path

**Becoming a reviewer** requires consistent, high-quality contributions in a specific ownership area, actionable and verifiable reviews, and familiarity with the project's validate-cheapest-first discipline, compatibility boundaries, and rollback requirements.

**Reviewer responsibilities**: first-pass review of changes in assigned areas, verifying the validation tier matches the change scope, checking that documentation, configuration, and release contracts are updated, and escalating decisions requiring final authority to a maintainer.

**Promotion to maintainer** is signaled by sustained review and issue triage, sound judgment on cross-module and release-impacting changes, the ability to drive a class of changes from implementation through verification, rollback planning, and documentation to closure, and explicit endorsement from an existing maintainer.

**Nomination**: An existing maintainer initiates the nomination. [`MAINTAINERS.md`](MAINTAINERS.md) and ownership records are updated on confirmation.

**Role removal** may occur due to prolonged inactivity, sustained inability to meet review or release duties, or trust or security issues requiring revocation of access.