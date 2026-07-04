# Nantian Gateway Roadmap

> Updated: 2026-07-03

## Recently Completed ✅

- [x] Native JWT authentication filter (JWKS, claims validation, headers passthrough)
- [x] E2E test framework (11 scenarios, Go testing + client-go, CI-integrated)
- [x] Performance optimizations: ArcStr, batched stats buffering, notify-based config watcher
- [x] Sentry integration (error tracking, tracing)
- [x] NodePort conflict resolution for Kind clusters
- [x] Platform-release log governance (raw logs → artifacts, git stays lean)
- [x] Performance regression gating in nightly CI

## In Progress 🔧

- [ ] JWT filter security hardening (alg enforcement, exp validation, key rotation handling)
- [ ] Dashboard test coverage expansion (target: 60% line coverage)

## Upcoming (Next 3 Months)

### Gateway Features
- [ ] OAuth2/OIDC authentication filter
- [ ] Backend TLS policy enhancements
- [ ] Rate limiting improvements (per-route, sliding window)

### Dataplane Performance
- [ ] Flamegraph-driven hot-path optimization
- [ ] Wasm hook instance pool (reduce per-request instantiation)
- [ ] TCP upstream connection pool sharding
- [ ] Fat LTO evaluation for release builds

### AI Gateway
- [ ] Embeddings endpoint routing + caching
- [ ] AI cost budget alerts (exceed → block + notify)
- [ ] Model marketplace UI (dashboard-based provider management)
- [ ] Streaming (SSE) first-class support across all AI features

### Dashboard
- [ ] Component test coverage for critical forms (routes, backends, policies)
- [ ] Live playground for AI Gateway (test prompts directly in dashboard)
- [ ] SLO dashboard with nightly performance baseline comparison

### Platform
- [ ] SBOM generation + container image signing for releases
- [ ] Helm chart template unit tests
- [ ] Publish Gateway API conformance report as a static page
- [ ] Slack/Discord community channel

## Longer-Term

- [ ] Multi-cluster / federation support
- [ ] gRPC reflection and API documentation (protodoc)
- [ ] Plugin SDK documentation and examples for Wasm developers
- [ ] Performance benchmarks published as nightly website

## How to Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and PR guidelines.

Feature requests and bug reports: open a [GitHub Discussion](https://github.com/nantian-gw/gateway/discussions) or [Issue](https://github.com/nantian-gw/gateway/issues).
