# Roadmap

Canonical roadmap content with exit criteria, milestones, and proposal flow
lives in [docs/roadmap.md](docs/roadmap.md). This file is the stable entrypoint
for users, maintainers, and external links.

## Current Status (June 2026)

The project is converging toward v0.2 (Implementation Claim Baseline).
Gateway API v1.5.1 with 55 supported features is documented. AI Gateway
(`aeg-ai`) and Wasm Plugin system (`aeg-wasm`) are implemented and integrated.

## Milestones

### v0.2 / Implementation Claim Baseline

**Status:** In progress — core materials are in place.

- Gateway API v1.5.1 with 55 supported features, documented support matrix
- Clean full-suite conformance baseline archived
- User-facing docs: README, support matrix, conformance reports, admin API, getting-started
- Backlog with P0/P1 acceptance criteria, verification commands, and evidence boundaries

See [docs/roadmap.md#v02--implementation-claim-baseline](docs/roadmap.md#v02--implementation-claim-baseline).

### v0.3 / Production Evidence Baseline

**Status:** Planned — production readiness evidence collection.

- 24h soak, node drain, apiserver watch disruption as release gates
- Multi-environment performance baseline (p95/p99/p999, success rate, RSS/CPU, reload-under-load)
- Release notes, compatibility notes, conformance, performance, security, soak evidence keyed by release tag
- Production overlay, Secret/mTLS/admin auth with consistent install/upgrade/rollback docs

See [docs/roadmap.md#v03--production-evidence-baseline](docs/roadmap.md#v03--production-evidence-baseline).

### v0.4 / Community And Expansion Baseline

**Status:** Post-v0.3 planning — dependent on production evidence and community growth.

- HTTP/3 / QUIC downstream support
- Helm chart and Operator packaging (Kustomize overlays already exist)
- Formal load balancing, backend selection, and policy capabilities
- Multi-maintainer governance, public design review, external adopter / case study feedback loops
- Gateway API version upgrade and extended experimental feature support

See [docs/roadmap.md#v04--community-and-expansion-baseline](docs/roadmap.md#v04--community-and-expansion-baseline).

## Current Feature Surface

| Area | Status | Docs |
|---|---|---|
| Gateway API v1.5.1 (55 features) | ✅ Conformance baseline | [support matrix](docs/gateway-api-support.md) |
| Split-plane (Go cplane + Rust dplane) | ✅ Implemented + tested | [architecture](docs/architecture.md) |
| Admin API (both planes) | ✅ Implemented | [admin-api](docs/user/admin-api.md) |
| Prometheus + Grafana | ✅ Implemented | [operations](docs/user/operations.md) |
| Kustomize overlays (production) | ✅ Implemented | [deploy](deploy

...(output truncated for display)