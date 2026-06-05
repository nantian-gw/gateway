# Community Readiness Checklist

## English Summary

Aether Gateway has enough technical and documentation structure to be reviewed as a Gateway API implementation in progress: it has a split control/data plane, archived conformance evidence, a support matrix, governance documents, maintainer information, security policy, release notes, and an implementation review packet.

It is not claiming official Gateway API recognition or mature CNCF community status yet. The remaining gaps are mostly non-code evidence: a final submission-target full-suite conformance run if the target moves, long-running production-like evidence, multiple active human maintainers, external contributors, named adopters, case studies, and a stable public release history.

---

This document answers two distinct questions:

- How far is the current repository from being claimed as an official Gateway API implementation.
- How far is the current repository from being ready as a more formal CNCF community project.

These two things are related, but they are not the same threshold.

## 1. Gateway API Official Implementation Readiness

The repository already has a strong technical foundation:

- Independent control plane and data plane already exist.
- Already integrated with the official conformance harness.
- Full-suite conformance reports are archived in the repository.
- `docs/gateway-api-support.md` documents the support matrix, gaps, and current boundaries.
- `docs/implementation-review-packet.md` consolidates governance, support matrix, conformance, release evidence, security, and adopter status needed for external review into a single entry point.
- The data plane has implemented security hardening: warnings for unconfigured auth, CRLF injection defense, request body size limits (default 10MB), and connection keepalive limits.
- Observability has been migrated to the Prometheus standard architecture, with Dashboard supporting Prometheus data source configuration via the Settings page.
- An experimental AI Chatbot feature supports natural language management of Gateway API resources.

To get closer to an official implementation claim, the following should still be completed:

- If using the current pending commit as the external reference baseline, re-archive a full-suite conformance report with `ALL_FEATURES=true` for that commit.
- Ensure the support matrix, community readiness documentation, and conformance archive README are consistent with the most recent externally referenced commit baseline.
- Clarify which features are part of the declared support scope and which are just implementation-specific extensions.
- Confirm that version support, status semantics, and the support matrix are consistent with the repository's actual behavior.
- Submit implementation information and conformance results following upstream procedures.

## 2. CNCF Community Project Readiness

Passing technical tests does not mean the project is ready for CNCF community status.

The current documentation-based governance materials are mostly complete, including:

- Contribution guide: [`../CONTRIBUTING.md`](../CONTRIBUTING.md)
- Code of conduct: [`../CODE_OF_CONDUCT.md`](../CODE_OF_CONDUCT.md)
- Maintainer information: [`../MAINTAINERS.md`](../MAINTAINERS.md)
- Governance model: [`../GOVERNANCE.md`](../GOVERNANCE.md)
- Support policy: [`../SUPPORT.md`](../SUPPORT.md)
- Security policy: [`../SECURITY.md`](../SECURITY.md)
- Versioning and release strategy: [`../VERSIONING.md`](../VERSIONING.md)
- Compatibility contract: [`contracts/versioning.md`](contracts/versioning.md)
- Roadmap: [`../ROADMAP.md`](../ROADMAP.md)

But this is not sufficient to prove the project is ready for mature CNCF community status. The following non-documentation evidence is still missing:

- Multiple long-term active human maintainers
- Visible external contributor and review records
- Adopters or real usage cases
- A stable release cadence and version support policy
- More public roadmap / design discussion processes

The closure criteria for these non-code evidence items are documented in the
[External Review Evidence Ledger](external-review-evidence.md). This ledger is an evidence
threshold, not a completion declaration; only when publicly auditable maintainer, reviewer, contributor,
adopter, case study, and release records meet the minimum threshold can the community external evidence items
be marked as complete.

Public page entry points have now been established:

- [Implementation Review Packet](implementation-review-packet.md)
- [Public adopter / case study / compatibility matrix page](adopters-and-compatibility.md)
- [External Review Evidence Ledger](external-review-evidence.md)

As of `2026-05-22`, this page still has no named adopters or public case study entries, so it solves the “no public entry point” problem, but does not mean adopter evidence itself has been completed.

## 3. Current Assessment

As of `2026-05-22`, a more accurate description would be:

<!-- release-evidence:conformance-clean-community:start -->
- The project has the technical foundation to advance as a Gateway API implementation, and has archived full-suite conformance reports based on Gateway API `v1.5.1`; the most recent archived conformance report is the `2026-05-14` full-suite at `90f5126a`. Since that baseline, the data plane has undergone multiple code changes (ExternalAuth forwardBody reimplementation, Prometheus observability migration, security hardening), so it is recommended to re-run `ALL_FEATURES=true` full-suite conformance on the current `main` branch and archive it as the new external reference baseline.
<!-- release-evidence:conformance-clean-community:end -->
- The project's code of conduct, governance, security, versioning, and roadmap documentation are largely complete.
- External technical review materials have a centralized entry point, but it only aggregates existing evidence and does not mean that multi-maintainer or adopter evidence has been completed.
- The external evidence ledger has defined closure criteria, but as of `2026-05-22` there is still no real external evidence meeting those thresholds.
- The project does not yet have the multi-maintainer, external adopter, public collaboration, and stable release evidence typically required for a mature CNCF community project.

It is not recommended to currently state directly that:

- “This is already an officially recognized Gateway API implementation”
- “This already meets CNCF Sandbox project conditions”

## 4. Recommended Priority Order

The following order of convergence is recommended, rather than continuing to expand functionality without boundaries:

1. First, align the support matrix, community readiness documentation, and conformance archive baseline with the current repository state.
2. Then, re-run and archive `ALL_FEATURES=true` full-suite conformance for the commit to be released or externally referenced.
3. Submit implementation information and conformance results following upstream procedures.
4. Then, fill in adopters, version support policy, multi-maintainer collaboration, and more public community evidence.

## 5. Minimum External Communication Guidelines

If you need to describe the current project status externally, the following phrasing is recommended:

- “Aether Gateway is a Kubernetes Gateway API implementation in progress with archived full-suite conformance results, documented support boundaries, and baseline governance materials.”
- “The project is not yet claiming official implementation recognition or mature CNCF community status; maintainer diversity, adopter evidence, and release history are still being expanded.”
