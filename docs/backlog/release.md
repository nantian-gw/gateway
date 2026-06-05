# Release And Packaging Backlog

This document tracks work related to release gates, install profiles, upgrade/rollback, report archiving, SBOM, provenance, and digests.

## P1: Long-Running Release Gates

Goal: Upgrade `24h` soak, node drain, and apiserver/watch disruption into formal release gates.

Background:

- Short-duration kind A4 and conformance cannot replace long-term RSS / FD / goroutine / thread / reconnect / watch stability evidence.
- The current 10m soak pilot does not mean the 24h baseline is complete.

Status: The release gate mechanism is complete, and commit `bb72c8f7` has archived a full round of fault injection release gate evidence: `release_gate_status=pass` in `reports/chaos/runs/2026-05-14-123117-bb72c8f7-kind-faults/`, traffic SLO gate is `pass`, and required scenarios `controlplane-leader-switch`, `dataplane-pod-restart`, `node-drain`, and `apiserver-watch-disruption` are all `pass`. `scripts/run-kind-soak.sh` now supports `SUMMARY_ONLY=true` to generate release evidence summaries offline from existing soak run artifacts, covering p99/p999, RSS/FD/thread slope, xDS reconnect/NACK delta, snapshot ACK wait p99, and ready replica first/min/max/last; both live runs and offline summaries write audit metadata including run id, output directory, tree state, duration, sampling interval, SLO thresholds, and `require_24h`/`min_required_duration_seconds` guardrail when `REQUIRE_24H=true`. `scripts/run-kind-fault-injection.sh` also writes `git_tree_state`/`code_tree_state` for both live runs and `SUMMARY_ONLY=true` offline summaries, facilitating release evidence auditing. `scripts/verify-release-evidence.sh` will still reject performance evidence lacking complete `throughput-report.json` coverage or source kind A4 SLO gate `pass`, soak metadata lacking `duration_seconds>=86400`, chaos/soak metadata lacking clean `code_tree_state`, incomplete chaos release gate summary, chaos traffic SLO gate not `pass`, or soak traffic SLO gate not `pass`; `scripts/refresh-release-evidence.sh` auto-discovery scans both old `*kind-a4*` and new `*a4-profiles`/`*a4-scenarios` performance archives, skipping dirty/missing code-tree, incomplete performance coverage, source SLO failed, non-24h or traffic SLO failed performance/chaos/soak archives. Old dirty/missing code-tree evidence can only be explicitly risk-accepted. A real `24h` soak on the same candidate commit is still required; subsequent runtime code or deployment semantic changes also require regenerating fault evidence under the same conditions.

Acceptance:

- [x] `scripts/run-kind-soak.sh` artifacts included in release evidence.
- [x] Report includes RSS slope, FD slope, thread/goroutine slope, p99/p999, xDS reconnect, snapshot ACK latency, and ready replica changes.
- [x] Node drain and apiserver/watch disruption have independent conclusions, not solely relying on soak logs.
- [x] Release candidate must state whether these evidence items pass, are risk-accepted, or block.

Unresolved real evidence thresholds:

- Soak archive on the same candidate commit must record `duration_seconds>=86400` and traffic SLO gate `pass`.
- Performance archive on the same candidate commit must include a complete `throughput-report.json` with empty missing lists for protocols, scenarios, and reload live-traffic; the source kind A4 `slo-gate.json` overall status and per-profile status must also be `pass`.
- Fault injection archive after subsequent runtime code or deployment semantic changes must include conclusions for `controlplane-leader-switch`, `dataplane-pod-restart`, `node-drain`, and `apiserver-watch-disruption`, with summary `pass` or explicit risk acceptance.

## P1: Production Install Profiles

Goal: Productize the production overlay into a formal installation entry point and establish an install profile matrix.

Status: Complete. See the [Install Profile Matrix](../user/install-profiles.md) for the current profile matrix; the release manifest rendering entry point supports `--profile`, and release assets default to `single-cluster-prod`.

Suggested profiles:

- `kind-dev`
- `kind-hostnetwork-perf`
- `single-cluster-prod`
- `multi-replica-prod`
- `observability-enabled`

Acceptance:

- [x] Each profile specifies Secrets, NetworkPolicy, Services, ports, HPA, PDB, and resource requests/limits.
- [x] Add upgrade / rollback / canary GatewayClass runbook.
- [x] Release manifest, Kustomize overlay, and production README maintain the same field source without configuration drift.

## P2: Traffic Profile Examples

Goal: Expand external documentation and examples for north-south / east-west profiles, avoiding confusion between install profiles, traffic entry points, and Gateway API support declarations.

Status: Complete. See [Traffic Profile Examples](../user/traffic-profiles.md) for current examples, covering north-south HTTP/gRPC, north-south TCP/UDP, and east-west service parent.

Acceptance:

- [x] Distinguish between install profiles and traffic profiles.
- [x] Provide north-south `Gateway` + `HTTPRoute` examples.
- [x] Provide north-south TCP / UDP listener with `TCPRoute` / `UDPRoute` examples.
- [x] Provide east-west `Service` parent examples, clarifying this is a repo extension capability, not equivalent to a full official Gateway profile declaration.
- [x] Add minimal troubleshooting commands and deploy/user documentation entry points.

## P1: SBOM / Provenance / Digest Pin

Goal: Complete the release pipeline with SBOM, provenance, digest pin, and operator-facing release notes.

Status: Complete. The release workflow generates SBOM and provenance attestation for both controlplane and dataplane images; the release asset packaging entry point generates `image-digests.txt` and categorized `RELEASE_NOTES.md`, using the notes file in GitHub releases.

Acceptance:

- [x] Both controlplane and dataplane images have SBOM and provenance.
- [x] Release assets record image digests, not just tags.
- [x] Release notes are categorized by breaking changes, upgrade notes, security, conformance, performance, and known issues.
- [x] Release validation clearly lists the conformance, e2e, security, performance, and soak evidence executed this release.

## P2: Gateway API Upgrade Cadence

Goal: Upgrade to a higher version of Gateway API and establish a regular upgrade audit cadence.

Requirements:

- Add `docs/gateway-api-version-audit.md` or equivalent audit record before upgrading.
- Synchronously update controlplane, conformance harness, kind/e2e default version, release validation default version, and status supported version.
- Re-run targeted conformance or full-suite and archive reports.
- Support matrix only declares features that have been truly verified.

## P2: Packaging Options

Goal: Add Helm chart / Operator packaging alongside existing Kustomize base / overlays and release manifest rendering entry points, maintaining field source consistency across multiple installation forms.

Boundaries:

- Kustomize base / overlays and release manifest rendering entry points already exist; the remaining focus of this item is Helm chart, Operator, and field source consistency when maintaining multiple installation forms in parallel.
- Operator should not enter the default path until release evidence, upgrade/rollback processes, and permission models are stable.
- Helm chart must reuse the same set of profile field sources to avoid drift from base/production overlay.

## P2: Multi-Environment Performance

Goal: Establish multi-environment comparison baselines for performance and capacity, rather than only retaining kind samples.

Suggested dimensions:

- kind local
- single-node production-like cluster
- multi-node cluster
- observability enabled
- different allocator profiles

Acceptance:

- Report can compare RPS, p99, success rate, CPU, RSS, FD, and threads.
- Release notes can explain whether performance changes are expected gains, acceptable regressions, or risks to be fixed.

## CI Platform Follow-Up

Pending external resolution:

- GitHub Actions billing / spending limit issues are not a repository code regression.
- After billing is restored, re-trigger `main` CI to confirm jobs can actually start.
- After platform recovery, reassess whether repo-level workflow / test failures still exist.

## References

- [Release runbook](../user/release-runbook.md)
- [Latest baseline](../test/latest-baseline.md)
- [Performance reports](../../reports/performance/README.md)
- [Chaos reports](../../reports/chaos/README.md)
- [Soak reports](../../reports/soak/README.md)
