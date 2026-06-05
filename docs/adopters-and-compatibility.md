# Public Adopter, Case Study, and Compatibility Matrix

## English Summary

This page is the public entrypoint for adopters, case studies, and compatibility evidence.
It currently provides a compatibility matrix and contribution format, but it does not yet list any named external adopters or public case studies.

Do not treat "page exists" as "adopter evidence exists". Compatibility claims below only reflect documented repository evidence and explicitly stated support boundaries.

---

This page addresses three external questions:

- Whether the current repository already has a public adopter / case study page.
- What compatibility and validation evidence the repository is willing to publicly commit to.
- What format external users should follow if they want to add their deployment or case study to the repository.

A clear distinction is needed:

- "A public page already exists" does not equal "sufficient adopter evidence already exists."
- "Technical evidence already exists in the repository" does not equal "all environments have been publicly declared as supported."

As of `2026-05-12`:

- This page already exists as a public repository entry.
- There are currently no named external adopter entries.
- There are currently no public case study entries.
- This means the historical backlog item "establish public adopter / case study / compatibility matrix page" can be closed;
  but "adopter evidence is sufficient to support more formal external review" still cannot be considered complete.

## 1. Current Public Status

| Item | Current Public Status | Notes |
| --- | --- | --- |
| Public adopter page | `Established` | This page is the public entry. |
| Named adopter | `No public entries yet` | The current repository has no adopter list with public citation permission. |
| Public case study | `No public entries yet` | The current repository has no compiled and publicly published case articles or deployment retrospectives. |
| Public compatibility matrix | `Established` | See matrix below; only records existing evidence and explicit declarations in the repository, does not write "future plans" as "current support." |

## 2. How to Add an Adopter or Case Study

If you want to add your deployment, validation results, or case study to the public page, two approaches are recommended:

1. Submit a PR directly modifying this page.
2. Submit an issue asking maintainers to add the public information on your behalf.

It is recommended to include at least the following information:

- Organization or project name
- Whether public naming is permitted
- Deployment environment
   e.g.: local Kind, bare-metal Kubernetes, self-hosted cluster, managed Kubernetes
- Usage pattern
   e.g.: north-south ingress, mesh/service parent, HTTP only, including gRPC/TCP/UDP
- Version used
   Recommend providing release tag, commit, or archived report reference
- Validated scope
   e.g.: full-suite conformance, upgrade/rollback, performance baseline, production canary
- Public links
   e.g.: blog, talk, repository, internal platform public article

If the organization name cannot be publicly disclosed for now, an anonymous compatibility note can also be added.
However, anonymous entries should not be treated as "named adopter evidence" and can only serve as supplementary compatibility notes.

## 3. Public Compatibility Matrix

The matrix below only records content for which "public evidence or explicit documentation declarations already exist in the repository."
For items without evidence, it is better to write "not yet publicly declared" rather than packaging it as default support.

This page follows the four-tier terminology from [Gateway API Support Matrix](gateway-api-support.md):

- `declared`: Has been publicly declared via `GatewayClass.status.supportedFeatures` and conformance profile.
- `implemented`: Control plane translation, status judgment, gRPC/xDS distribution, and data plane main path exist.
- `tested`: Conformance, e2e, unit, smoke, benchmark, or specialized script evidence exists.
- `production-validated`: Production overlay, long-term soak, fault injection, upgrade/rollback, and capacity evidence exists; most capabilities can currently only be marked as `limited` or `no`.

| Dimension | Current Public Status | Public Evidence | Notes |
| --- | --- | --- | --- |
| Gateway API code and CRD baseline | `Aligned to v1.5.1` | [gateway-api-support.md](gateway-api-support.md) | Control plane dependencies, CRD bundle, most recent clean full-suite baseline, and current latest conformance archive are aligned to `v1.5.1` per documentation. |
| Gateway API support declaration | `declared=55 features` | [gateway-api-support.md](gateway-api-support.md), [supported features source](../controlplane/internal/gatewayapi/supported_features.go) | `declared` only means entry into `GatewayClass.status.supportedFeatures` and full-suite profile, not that all scenarios are production-ready. |
| full-suite conformance | `Archived PASS` | [gateway-api-support.md](gateway-api-support.md), [reports/conformance/README.md](../reports/conformance/README.md) | Most recent clean commit full-suite baseline is `2026-05-08` at `3af22b42`, Gateway API `v1.5.1`; current `latest/` is `2026-05-12` at `0355945e` kind mesh profile, should not be misread as full-suite. If a newer commit is to be used as an external reference baseline, full-suite should be re-archived. |
| Release-level automation sample | `A1 sample exists` | [test/latest-baseline.md](test/latest-baseline.md) | Most recent complete `A1` one-click sample recorded as `2026-03-27`. |
| kind performance / chaos / soak evidence | `Comprehensive archive exists` | [test/latest-baseline.md](test/latest-baseline.md), [comprehensive validation report](../reports/validation/runs/2026-04-30-1bc4aea-comprehensive/summary.md) | Most recent comprehensive archive is `2026-04-30` at `1bc4aea`, including kind smoke, metrics/admin surface, performance, fault injection, and 10m soak pilot. |
| Long-duration soak | `Not yet publicly declared complete` | [test/latest-baseline.md](test/latest-baseline.md), [community-readiness.md](community-readiness.md) | Currently only a `10m soak pilot` sample exists; cannot be externally represented as `24h/72h` soak completed. |
| node drain / apiserver-watch jitter specialized | `kind release-gate archived, production-validated=limited` | [release backlog](backlog/release.md), [test/latest-baseline.md](test/latest-baseline.md), [chaos report](../reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults/summary.md) | `2026-05-14` completed full fault injection release gate in Kind; production-approximate environments and long-duration fault storms still need subsequent evidence. |
| controlplane/dataplane/proto skew window | `Documented` | [skew-compatibility.md](skew-compatibility.md) | Current public stance is same-tag formal support; `N` and `N-1` are only temporarily supported within upgrade/rollback windows. |
| `HTTPRoute` / `GRPCRoute` | `declared + implemented + tested, production-validated=limited` | [gateway-api-support.md](gateway-api-support.md), [reports/conformance/README.md](../reports/conformance/README.md), [comprehensive validation report](../reports/validation/runs/2026-04-30-1bc4aea-comprehensive/summary.md) | Main path has conformance, e2e, unit, and performance evidence; production overlay and short-duration fault/soak cannot substitute for long-term production validation. |
| `UDPRoute` | `declared + implemented + tested, production-validated=limited` | [gateway-api-support.md](gateway-api-support.md), [reports/conformance/README.md](../reports/conformance/README.md) | Gateway API `v1.5.1` official harness already covers `UDPRoute`; still lacks production-grade throughput, packet loss, and long-duration churn evidence. |
| `TCPRoute` | `implemented + tested, not declared` | [gateway-api-support.md](gateway-api-support.md), [tests/e2e/smoke.yaml](../tests/e2e/smoke.yaml) | Upstream `v1.5.1` has no TCPRoute conformance feature / test cases; currently proves repository implementation boundary via kind smoke and unit tests. |
| `TLSRoute` passthrough | `declared + implemented + tested, production-validated=limited` | [gateway-api-support.md](gateway-api-support.md), [tests/e2e/smoke.yaml](../tests/e2e/smoke.yaml) | Currently declares core passthrough; TLS termination / mixed mode is not publicly supported. |
| `Service` parent / mesh frontend | `declared + implemented + tested, production-validated=limited` | [gateway-api-support.md](gateway-api-support.md), [reports/conformance/README.md](../reports/conformance/README.md), [comprehensive validation report](../reports/validation/runs/2026-04-30-1bc4aea-comprehensive/summary.md) | `Mesh` and currently declared mesh HTTPRoute extended features have `2026-05-12` kind mesh profile and targeted conformance; still lacks east-west long-running and multi-environment evidence. |
| `BackendTLSPolicy` | `declared + implemented + tested, production-validated=limited` | [gateway-api-support.md](gateway-api-support.md), [reports/conformance/README.md](../reports/conformance/README.md) | Already covers `v1` compatible access, CA, SAN, basic status, conformance, and CA bundle rotation; long-term CA operation evidence is still insufficient. |
| `BackendLBPolicy` | `partial implemented + tested, not declared` | [gateway-api-support.md](gateway-api-support.md), [docs/user/operations.md](user/operations.md) | Only publicly exposes current `sessionPersistence` and repo-specific `loadBalancing.consistentHash` subset; does not declare full BackendLBPolicy. |
| Kind trial path | `Publicly supported` | [user/getting-started.md](user/getting-started.md) | This is the most direct and recommended trial method in the repository. |
| Local process debug path | `Publicly supported` | [user/getting-started.md](user/getting-started.md) | For advanced debugging scenarios, not the recommended first-time trial path. |
| Kubernetes manifest installation | `Publicly supported` | [user/getting-started.md](user/getting-started.md), [deploy/kubernetes/overlays/production/README.md](../deploy/kubernetes/overlays/production/README.md) | Repository maintains base manifests and production overlay. |
| Production overlay | `Publicly supported` | [deploy/kubernetes/overlays/production/README.md](../deploy/kubernetes/overlays/production/README.md) | Currently has clearly defined default security boundaries, mTLS, and admin authentication model. |
| Managed Kubernetes distribution matrix | `Not yet publicly declared` | This page | Currently no public compatibility entries for GKE / EKS / AKS / OpenShift. |
| HTTP/3 / QUIC downstream capability | `Not yet publicly supported` | [roadmap](roadmap.md) | Still an explicitly incomplete item; only enters proposal after production evidence stabilizes. |
| Named adopter | `No public entries yet` | This page | Page is established, but currently no publicly named adopter. |
| Public case study | `No public entries yet` | This page | Page is established, but currently no publicly published case articles or retrospectives. |

The closure thresholds for named adopters, public case studies, external contributors, and maintainer evidence
are recorded in [External Review Evidence Ledger](external-review-evidence.md). This page only
records public compatibility and adopter entry; anonymous feedback, internal trials, or usage without
public permission must not be treated as named adopter evidence.

## 4. Recommended Public Case Study Template

If you want to add a public case study, it is recommended to directly use the following template:

```md
## <Organization or Project Name>

- Public status: Public naming permitted / Anonymous summary only
- Version used: `vX.Y.Z` or commit `abcdef0`
- Deployment environment: Kind / Self-hosted Kubernetes / Managed Kubernetes
- Usage pattern: north-south ingress / mesh / mixed
- Validated scope: conformance / rollout / rollback / perf / soak / security
- Public link: <url>
- Notes: <optional supplement>
```

## 5. Page Maintenance Rules

To prevent this page from degrading back into a slogan page, the following rules are recommended for maintenance:

- Only write facts that already have evidence in the repository or are externally and publicly verifiable.
- Evidence should point to repository archives, fixed script entry points, or public links as much as possible, not verbal descriptions.
- If a capability has only been validated in Kind or short-duration samples, explicitly state this boundary.
- If adopter information does not yet have public permission, do not write it as a named entry.
- If public adopters / case studies appear later, update this page first, then sync the judgment in [community-readiness.md](community-readiness.md).

## 6. Relationship with Other Documents

- To see the Gateway API support scope, feature matrix, and conformance profile, see [Gateway API Support Matrix](gateway-api-support.md).
- To see multi-maintainer, external contributor, and public review evidence thresholds, see [External Review Evidence Ledger](external-review-evidence.md).
- To see community readiness judgment and CNCF entry conditions, see [community-readiness.md](community-readiness.md).
- To see external technical review and implementation claim preparation materials, see [Implementation Review Packet](implementation-review-packet.md).
