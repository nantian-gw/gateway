# Changelog

Stable release summary for Aether Gateway. Formal releases must include
key feature and behavior changes, pointers to known compatibility risks,
and links to compatibility notes.

## Unreleased

<!-- release-evidence:changelog-summary:start -->
### Core Gateway

- **Gateway API v1.5.1** with 55 declared supported features. Full conformance baseline
  at `2026-05-08-3af22b42-full`; latest mesh profile at `2026-05-12-0355945e-kind-mesh-profile-current`.
- Split-plane architecture: Go control plane (controller, translator, status, admin, xDS)
  + Rust data plane (Rust proxy HTTP/stream runtime, xDS client).
- Admin APIs on both planes, Prometheus metrics, Grafana dashboard.
- Kustomize base and production overlay with release manifest rendering.
- Kind smoke tests, conformance harness, targeted validation scripts.

### AI Gateway (`aeg-ai`)

- Multi-format AI proxy: OpenAI, Anthropic, Ollama protocols.
- Token counting, rate limiting, API key management.
- Prompt guard, PII masking, content safety filtering.
- Semantic caching, model fallback, cost tracking.
- Multi-tenancy, model routing, A/B testing.
- AI admin API endpoints: `GET /v1/ai/overview`, `/v1/ai/services`,
  `/v1/ai/token-usage`, `/v1/ai/traces`, `/v1/ai/cost`.
- `AIService` and `TokenPolicy` CRDs (experimental).

### Wasm Plugin System (`aeg-wasm`)

- wasmtime-based plugin runtime for custom request/response hooks.
- `aeg-wasm-sdk`: Rust SDK for writing Wasm plugins with host function bindings.
- `PluginManager`: lifecycle management (load, invoke, unload).
- `WasmPlugin` CRD with ConfigMap-based plugin distribution.
- AI inference sandbox for tokenizer/embedder execution in Wasm.
- `AISandbox` tokenizer with prebuilt wasm modules and CI build integration.
- E2E validation pipeline for Wasm plugins.

### Dashboard

- Next.js 16 / React 19 admin console with NextAuth and Tailwind v4.
- Node proxy server for admin API aggregation.
- Route for AI Gateway and Wasm Plugin CRUD.
- Chatbot: LLM-driven Gateway API config generation with dry-run validation.

### Observability & Admin

- Control-plane admin API at `:18081`, data-plane admin API at `127.0.0.1:19080`.
- Admin API contract surface tracked in `docs/contracts/admin-api-surface.json`.
- Prometheus metrics with Grafana dashboard.
- Chatbot config, metrics config, and AI overview endpoints.
<!-- release-evidence:changelog-summary:end -->

No formal releases yet. The project is in pre-release convergence with v0.2
(Implementation Claim Baseline) as the next milestone. See [ROADMAP.md](ROADMAP.md)
and [docs/roadmap.md](docs/roadmap.md) for version targets.

## Release Format

```markdown
## vX.Y.Z - YYYY-MM-DD

### Breaking Changes
- ...

### Upgrade Notes
- See `docs/user/compatibility-notes.md#vx-y-z`

### Security
- ...

### Conformance
- ...

### Performance
- ...

### Known Issues
- ...
```