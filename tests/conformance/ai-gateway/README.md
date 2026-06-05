# AI Gateway Conformance Profile

## Declared Feature Set

All features are implemented in the `aeg-ai` Rust crate within the Aether Gateway dataplane.

### Phase 1: Core

| Feature | Status | File |
|---------|--------|------|
| OpenAI format adapter | Implemented | `aeg-ai/src/format/openai.rs` |
| Anthropic format adapter | Implemented | `aeg-ai/src/format/anthropic.rs` |
| Ollama format adapter | Implemented | `aeg-ai/src/format/ollama.rs` |
| Unified AI IR | Implemented | `aeg-ai/src/format/ir.rs` |
| Token Counter | Implemented | `aeg-ai/src/token.rs` |
| AIGatewayFilter | Implemented | `aeg-ai/src/filter.rs` |
| AIService CRD | Implemented | `controlplane/.../aiservicev1alpha1/` |
| Prometheus Metrics | Implemented | `aeg-ai/src/observability/metrics.rs` |
| OTel Tracing | Implemented | `aeg-ai/src/observability/tracing.rs` |
| Langfuse Integration | Implemented | `aeg-ai/src/observability/langfuse.rs` |
| AI Dashboard | Implemented | `dashboard/src/app/[locale]/ai/` |

### Phase 2: Production

| Feature | Status | File |
|---------|--------|------|
| TokenPolicy CRD | Implemented | `controlplane/.../tokenpolicyv1alpha1/` |
| TokenRateLimiter | Implemented | `aeg-ai/src/ratelimit.rs` |
| API Key Management | Implemented | `aeg-ai/src/keyring.rs` |
| Prompt Guard | Implemented | `aeg-ai/src/prompt_guard.rs` |
| Semantic Cache | Implemented | `aeg-ai/src/semantic_cache.rs` |
| Model Fallback | Implemented | `aeg-ai/src/fallback.rs` |
| Cost Tracker | Implemented | `aeg-ai/src/cost.rs` |
| Cost Dashboard | Implemented | `dashboard/src/app/[locale]/ai/cost/` |

### Phase 3: Enterprise

| Feature | Status | File |
|---------|--------|------|
| PII Masking | Implemented | `aeg-ai/src/pii.rs` |
| Content Safety | Implemented | `aeg-ai/src/content_safety.rs` |
| Multi-Tenancy | Implemented | `aeg-ai/src/multitenant.rs` |
| Model Router | Implemented | `aeg-ai/src/model_router.rs` |
| Prompt Template Injection | Implemented | `aeg-ai/src/prompt_template.rs` |
| A/B Testing | Implemented | `aeg-ai/src/ab_test.rs` |
| OTel Advanced | Implemented | `aeg-ai/src/observability/tracing.rs` |
| Langfuse Advanced | Implemented | `aeg-ai/src/observability/langfuse.rs` |

## Running Conformance

```bash
./tests/conformance/ai-gateway/run.sh
```

Requires a Kind cluster with CRDs installed.