# Security And Credentials Backlog

This document tracks supply chain, RBAC, Secret/token/certificate rotation, mTLS, admin authentication, and risk acceptance items.

## P1: Dependabot And Supply Chain Triage

Goal: Incorporate GitHub / Dependabot security alerts on the default branch and release candidates into continuous triage, rather than relying solely on static scanning entry points.

Current known status:

- `2026-05-12` review of default branch GitHub Dependabot open alerts: only `#7 rust/protobuf < 3.7.2` medium remains, `scripts/check-dependabot-alert-triage.sh` outputs `open_alert_count=1`, `unreviewed_count=0`.
- `2026-05-24` review of default branch GitHub Dependabot open alerts: still only `#7 rust/protobuf < 3.7.2` medium remains; `scripts/check-dependabot-alert-triage.sh --github-repository mahmut-Abi/aether-gateway` outputs `reviewed open alerts: 1` and passes.
- The alert comes from the `aether-core 0.8.0 -> prometheus 0.13.x -> protobuf 2.x` transitive dependency.
- `SEC-RA-004` has recorded risk acceptance; `dataplane/deny.toml` has tightened the exemption rationale to the reviewed dependency chain.
- `scripts/run-dataplane-guardrails.sh` requires that if protobuf 2.x still exists, it must still be this reviewed dependency chain.
- `scripts/check-dependabot-alert-triage.sh` has been integrated into `scripts/run-security-scans.sh`, classifying current open Dependabot alerts as `fixed-awaiting-platform-refresh`, `risk-accepted:SEC-RA-004`, or failing `unreviewed`; current platform alerts for `openssl`, `dompurify`, and `postcss` have disappeared, with only `protobuf < 3.7.2` passing via `SEC-RA-004` risk acceptance.
- In `docs/security/risk-register.md`, `SEC-RA-004` has been updated with owner, review frequency, upstream blocker, expiry conditions, and closure actions, serving as a traceable mapping from platform alerts to repository risk records.

Follow-up actions:

- Continuously track upstream Rust proxy removal path.
- Before releasing a release candidate, `scripts/check-dependabot-alert-triage.sh` must pass, ensuring no unreviewed high-severity alerts or a state where only GitHub platform alerts exist without in-repo explanations.
- GitHub platform alerts must have explanations in repository documentation or the risk register; external alerts without in-repo context are not allowed.
- If the `SEC-RA-004` expiry condition is triggered, the `dataplane/deny.toml` exception must be removed and dataplane guardrails, security scans, and release validation must be re-run.

## P1: Secret / Certificate / Token Rotation

Goal: Add restart-free e2e tests for Secret, certificate, and token rotation.

Scope:

- controlplane admin token
- dataplane admin token
- controlplane gRPC TLS / mTLS
- dataplane xDS TLS / mTLS
- dataplane session persistence secret
- Gateway TLS cert
- BackendTLSPolicy CA bundle
- backend client cert

Acceptance:

- Verify how long after update it takes effect.
- Explain how old connections are handled.
- Whether last-good is preserved on failure.
- Rotation tests must not require restarting controlplane or dataplane.

## P1: RBAC Audit Maintenance

Completed baseline:

- `scripts/audit-controlplane-rbac.sh` compares `deploy/kubernetes/base/rbac.yaml` with `docs/security/controlplane-rbac-required.json`.
- The controlplane ClusterRole has been categorized by resource and had unused `patch` and unnecessary Secret/Pod/Namespace write permissions removed.
- `scripts/run-security-scans.sh` has integrated RBAC audit.

Ongoing requirements:

- When adding informer, watch, or client verbs, `controlplane-rbac-required.json` and usage descriptions must be updated synchronously.
- Before release, check that new permissions have documentation and test justifications.
- In the production profile, NetworkPolicy, ServiceAccount, and Secret permission boundaries must align with RBAC documentation.

## P1: mTLS And Admin Auth Boundary

Completed baseline:

- The production overlay requires a controlplane admin Bearer Token by default.
- Both the controlplane and dataplane admin APIs support Bearer Token.
- The management plane contract already has route contract tests.

Ongoing requirements:

- controlplane/dataplane xDS mTLS rotation e2e.
- admin token rotation e2e.
- Documentation must not imply that the admin API can be directly exposed to untrusted networks.

## P2: Risk Register Hygiene

Goal: Security risk acceptance items are traceable, auditable, and revocable.

Requirements:

- Each risk acceptance item includes owner, scope, rationale, compensating controls, and expiry or review conditions.
- Dependency risks must be verifiable by scripts as still being the reviewed chain.
- Release notes describe security risks affecting user deployments and mitigation approaches.

## References

- [Security risk register](../security/risk-register.md)
- [Controlplane RBAC](../security/controlplane-rbac.md)
- [Structured security review](../security/structured-review-2026-03.md)
- [Operations guide](../user/operations.md)
