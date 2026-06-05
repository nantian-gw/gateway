# Aether Gateway Security Risk Acceptance Register

This document tracks known, reviewed, and temporarily accepted security residual risks for the current repository.
The goal is not simply to "record issues and call it done," but to ensure every accepted item has an ID, scope, rationale, and follow-up actions.

Last updated: `2026-05-24`

## Currently Accepted Items

| ID | Status | Scope | Current Acceptance Rationale | Follow-up Actions |
| --- | --- | --- | --- | --- |
| `SEC-RA-001` | `Accepted` | controlplane `:18082/metrics` currently does not use Bearer Token, relying on `ClusterIP + NetworkPolicy` instead | Current base manifests and production overlay both constrain the control plane metrics surface within the cluster, and only allow access from controlled sources by default; this is a known design boundary of the current repository and Prometheus scrape model. | If cross-namespace or off-cluster scraping is required in the future, controlled proxies, additional `NetworkPolicy`, or a more formal auth model must be added synchronously, and the item must be re-reviewed. |
| `SEC-RA-002` | `Accepted` | Long-term xDS TLS / mTLS rotation, certificate expiry, long-connection recovery, and fault injection have not yet formed more complete automation | Current TLS / mTLS configuration surface and basic unit tests already exist, but stronger cluster-level drills remain in the subsequent `P0/P1` plan. | Add dedicated verification for certificate rotation, expiry, handshake failure, and recovery behavior, and include it in pre-release-candidate checks. |
| `SEC-RA-003` | `Accepted` | HTTP request parsing boundaries have not yet filled gaps in systematic fuzzing, and slow body / idle / flood automation | `validate-http-security.sh` already covers common high-risk anomalous messages, but is not yet a substitute for more systematic parser and resource exhaustion drills. | Continue expanding parser fuzzing, slow body, idle connections, and higher-concurrency flood tests into standard scripts, and retain long-term evidence in staging environments. |
| `SEC-RA-004` | `Accepted` | dataplane dependency graph still contains `protobuf < 3.7.2`, sourced from upstream `nantian-core 0.8.0 -> prometheus 0.13.x -> protobuf 2.x` | This is the only remaining GitHub / Dependabot medium alert on the default branch. The repository's own dataplane `/metrics` output is first-party Prometheus text rendering and does not parse external protobuf payloads; the risk primarily comes from transitive dependencies in upstream Rust proxy crates, not the repository's own protocol surface. Investigation shows that `prometheus 0.14.0` has switched to `protobuf ^3.7.2`, but `pingora 0.8.0` on crates.io still pins `prometheus 0.13`, and directly switching to upstream unreleased Rust proxy `main` would bind the default branch to an unreleased runtime — the current risk/reward ratio does not warrant it. | Keep the explicit exemption for `RUSTSEC-2024-0437` in `dataplane/deny.toml`, and use `scripts/run-dataplane-guardrails.sh` to regularly verify that "the only currently allowed protobuf 2.x path remains `nantian-core -> prometheus`"; once a released upstream Rust proxy or equivalent low-risk upgrade path removes this dependency chain, delete the deny exemption, close this accepted item, and re-run dataplane guardrails / release validation. |

## SEC-RA-004 Review Contract

| Field | Value |
| --- | --- |
| Owner | Current repository maintainer / release owner: [@mahmut-Abi](https://github.com/mahmut-Abi) |
| Platform Alert | GitHub Dependabot alert `#7`, `rust/protobuf < 3.7.2`, severity `medium` |
| Current Dependency Chain | `nantian-core 0.8.0 -> prometheus 0.13.4 -> protobuf 2.28.0` |
| Upstream Blocker | The latest `pingora` release on crates.io is still `0.8.0`; this release still uses `prometheus 0.13.x` via `nantian-core`. |
| Review Frequency | `scripts/check-dependabot-alert-triage.sh` must be run before every release candidate; during default branch security maintenance, review at least once a month, or immediately when Dependabot / Rust proxy / prometheus related updates appear. |
| Closure Conditions | A released upstream Rust proxy or equivalent low-risk upgrade path appears, removing `protobuf 2.x` from the default dependency graph; OR GitHub / RustSec raises this risk to `high` / `critical`; OR the repository adds a dataplane path that parses untrusted protobuf payloads. |
| Closure Actions | Delete the `RUSTSEC-2024-0437` exemption in `dataplane/deny.toml`, update this register status, re-run dataplane guardrails, security scans, and release validation. |

`scripts/check-dependabot-alert-triage.sh` maps current platform alerts to `risk-accepted:SEC-RA-004` and fails when unmatched open alerts appear; the release/security workflow reuses this gate via `scripts/run-security-scans.sh`.

## Usage Rules

- New accepted items must have a corresponding audit conclusion first; they cannot be directly added to the register.
- Each accepted item must be traceable to specific implementation boundaries or incomplete automation, not vague statements like "there is still risk."
- Once the corresponding issue is fixed or replaced by a stricter threshold, change the status to `Closed` and do not retain zombie accepted items long-term.

Related audit records:

- [structured-review-2026-03.md](./structured-review-2026-03.md)