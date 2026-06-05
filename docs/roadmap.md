# Nantian Gateway Roadmap

This roadmap describes externally communicable version directions and exit conditions. The full engineering backlog still uses the root-level
`ROADMAP.md` as the source of truth; [`docs/backlog/`](backlog/README.md) provides topic views split by control plane, data plane, release, and security.

## Roadmap Principles

- Do not write `GatewayClass.status.supportedFeatures` as a production-ready promise.
- Fill in evidence first, then expand the capability surface.
- Stabilize the control plane, data plane, release, and security baselines first, then push forward with HTTP/3, Helm/Operator, or more Experimental features.
- Every cross-module capability should have a proposal, ADR, or backlog acceptance criteria before entering implementation.

## v0.2 / Implementation Claim Baseline

Goal: Enable the project to externally communicate with clear evidence that "this is a converging Gateway API implementation," without exaggerating production maturity.

Current status: Basic implementation claim materials are largely in place. The repository's default Gateway API version is `v1.5.1`, currently declaring `55` `supportedFeatures`, and the support matrix is expressed in four tiers: declared / implemented / tested / production-validated; the most recent clean full-suite conformance baseline is `2026-05-08-3af22b42-full`, and current `latest/` points to `2026-05-12-0355945e-kind-mesh-profile-current` mesh profile.

Remaining priorities:

- Re-archive full-suite conformance for any new commit intended for external reference rather than reusing old commit evidence.
- `ListenerSet` is still not implemented; HTTP/3 / QUIC, TLSRoute terminate/mixed mode, and complete BackendLBPolicy are still treated as unsupported boundaries.
- External documentation should keep README, support matrix, conformance reports, community readiness, implementation review packet, and compatibility notes referencing the same baseline set.

Exit conditions:

- `docs/gateway-api-support.md`, `reports/conformance/`, `docs/community-readiness.md` reference the same current baseline set.
- Users can find support scope, installation, validation, troubleshooting, and uninstall entry points via README.
- Undeclared or partially implemented Gateway API subsets are not written as production-ready in public materials.
- High-priority P1 backlog items have clear verification commands, evidence boundaries, and release gate boundaries.

## v0.3 / Production Evidence Baseline

Goal: Fill in long-term environment, release, and operational evidence so the project can more seriously evaluate a production pilot.

Remaining priorities:

- `24h` soak, node drain, and apiserver/watch turbulence drills upgraded to release gates.
- Establish multi-environment performance and capacity comparison baselines beyond Kind, covering p95/p99/p999, success rate, RSS/CPU, and reload-under-load.
- Continue keeping Secret, certificate, admin token, mTLS, and session persistence secret rotation in sync with e2e/operations evidence.
- Release candidate evidence must distinguish between "the repository has verified at some point" and "the current candidate commit has been verified."

Existing foundations:

- Production Kustomize overlay, `kind-dev`, `kind-hostnetwork-perf`, `single-cluster-prod`, `multi-replica-prod`, `observability-enabled` install profile matrix already exists.
- Release assets already have image digest, SBOM, provenance, and operator-facing release notes foundations.
- Release evidence alignment, security scan, Dependabot triage, conformance summary, and report archive guardrails already have script entry points.

Exit conditions:

- At least one round of archivable, reusable `24h` soak, node drain, apiserver/watch turbulence drill, and release validation evidence.
- Release notes, compatibility notes, conformance, performance, security, and soak evidence can be correlated by release tag.
- Production overlay profile, Secret/mTLS/admin auth, and observability entry points have consistent install/upgrade/rollback documentation.
- Release candidates have no unreviewed high-severity alerts or "platform has an alert, but repository has no explanation" status.

## v0.4 / Community And Expansion Baseline

Goal: After core production evidence stabilizes, expand to broader protocol surfaces and more formal community collaboration.

Candidate directions:

- HTTP/3 / QUIC downstream capability.
- Helm chart and Operator packaging; Kustomize base / overlays already exist, with subsequent focus on parallel maintenance of multiple install forms and field source consistency.
- More formal load balancing, backend selection, and policy capabilities.
- Multi-maintainer collaboration, public design reviews, external adopter / case study feedback loops.
- Gateway API official implementation claim and more Experimental features.

Entry conditions:

- Core evidence from v0.2 / v0.3 is stable.
- New directions have proposals describing support boundaries, rollback conditions, test matrices, and release impact.
- Do not expand `supportedFeatures` simply because there is a backlog entry.

## Expansion Guardrails

The default strategy before v0.2 / v0.3 is "fill in evidence first, then expand capabilities." The following directions may retain design records, research, and small-scale experiments, but will not enter default install profiles, release gates, or external support declarations:

| Direction | Why Deferred | Reopening Conditions |
| --- | --- | --- |
| HTTP/3 / QUIC | Would introduce a new downstream protocol stack, certificate/ALPN/listener behavior, and conformance interpretation cost | HTTP/TCP/UDP/TLS current capabilities, real forwarding performance, and long-stability evidence are stable |
| Helm / Operator | Would change install, upgrade, rollback, and permission models; Kustomize overlay already exists, no need to re-prove | Kustomize / release manifests, profile matrix, canary/rollback, and release evidence are stable |
| Complex incremental xDS patch | Could significantly increase control plane and data plane state machine complexity | Reload-under-load reports prove that full snapshot swaps are the primary bottleneck |
| Default allocator switch | Could affect RSS, fragmentation, tail latency, and platform compatibility | Multi-environment performance reports prove benefit, and release notes can explain risk and rollback |
| Large plugin system | Would expand API, security, and support boundaries | Core Gateway API support declarations, governance, and security policies are sufficiently stable |
| More Experimental features | Risk of accidentally writing unverified capabilities as production-ready | Each feature has a proposal, test evidence, support matrix, and withdrawal path |

If reopening the above directions, proposals or ADRs must first be added, clearly defining support boundaries, test matrices, rollback strategies, and release impact.

## Proposal And Review Flow

`docs/roadmap.md` only handles direction and phases, not complete design details.

When any of the following applies, first add a proposal or ADR:

- Changes to external contracts, compatibility, upgrade, or rollback paths.
- Involves multi-module coordination across control plane, data plane, deploy, and reports.
- Changes to Gateway API support declarations, release gates, or production overlay defaults.

Entry points:

- [Design proposals](proposals/README.md)
- [Architecture decision records](adr/README.md)
- [Backlog index](backlog/README.md)