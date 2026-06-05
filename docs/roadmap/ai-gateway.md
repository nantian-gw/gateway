# AI Gateway Roadmap

Incrementally build an AI Gateway on top of aether-gateway's existing Gateway API proxy infrastructure.

---

## Design Principles

| Principle | Description |
|---|---|
| **Gateway API Compliant** | All CRDs follow Gateway API standards (conditions, status, validation); new groups do not pollute `gateway.networking.k8s.io` |
| **Test-Driven** | Write tests before implementation for each feature. Unit tests → integration tests → Kind e2e → conformance |
| **Reasonable Code Structure** | Separation of concerns: filters as independent crates, CRD translators as independent packages, dashboard components as atomic units |
| **Complete Documentation** | Every CRD has an API reference, every filter has a design doc, every endpoint has a contract |
| **Consistent Dashboard Style** | Unified color tokens, typography, spacing, component library — consistent across pages |
| **Multi-API Format** | Unified internal IR supporting automatic conversion between OpenAI / Anthropic / Ollama / vLLM / Google AI and other formats |
| **Observability-First** | OpenTelemetry full-chain tracing + Prometheus metrics + Langfuse LLM observability, embedded from Phase 1 |

---

## Existing Foundation (Directly Reusable)

| Capability | Location | AI Gateway Usage |
|---|---|---|
| HTTP/HTTPS proxy | `dataplane/crates/aeg-http/` | LLM API forwarding (multi-format compatible) |
| SSE / chunked transfer | HTTP runtime | `/v1/chat/completions` streaming |
| gRPC proxy | gRPC filter | Internal model inference services (Triton, vLLM gRPC) |
| Rate limiting | `runtimeProtection` | token-based + request-based dual-layer rate limiting |
| Health checks | active health check | Model backend availability probing |
| Circuit breaking | circuit breaker | Model service degradation |
| Session persistence | session persistence | Stateful model routing |
| Access logging | access log (JSON + sampling) | Token usage auditing + Langfuse export |
| BackendLBPolicy | `LeastRequest` / `Random` | Model backend intelligent load balancing |
| TCP/UDP proxy | `dataplane/crates/aeg-stream/` | Vector database direct connection (pgvector / Qdrant) |
| Dashboard | Next.js / React 19 | AI Gateway management panel |
| Gateway API | K8s native CRD | Declarative AI routing configuration |
| Prometheus metrics | `dataplane/crates/aeg-observability/` | Metrics instrumentation foundation |
| OpenTelemetry log | `dataplane/crates/aeg-observability/` | OTel log export foundation |

---

## Overall Architecture

```
                    ┌──────────────────────────────────────────┐
                    │        Dashboard (AI Gateway UI)          │
                    │  usage · cost · model · traces · alerts   │
                    │  Langfuse prompt playground · eval view   │
                    └──────────────┬───────────────────────────┘
                                   │
┌──────────────────────────────────┼────────────────────────────────────────────┐
│                       Control Plane (Go)                                      │
│                                                                               │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │ AIService   │  │ AIGateway    │  │ TokenPolicy  │  │ Observability    │  │
│  │ CRD         │  │ CRD          │  │ CRD          │  │ Config CRD       │  │
│  │ (provider,  │  │ (global      │  │ (tokens/min, │  │ (Langfuse,       │  │
│  │  endpoint,  │  │  config,     │  │  api_key,    │  │  OTel endpoint,  │  │
│  │  model,     │  │  api formats)│  │  model)      │  │  sampling)       │  │
│  │  api_key,   │  │              │  │              │  │                   │  │
│  │  format)    │  │              │  └──────────────┘  └───────────────────┘  │
│  └─────────────┘  └──────────────┘                                           │
│                                                                               │
│  Translator: CRDs → IR → proto snapshot → dataplane                          │
└──────────────────────────────────┬────────────────────────────────────────────┘
                                   │ xDS (proto snapshot)
┌──────────────────────────────────┼────────────────────────────────────────────┐
│                       Data Plane (Rust)                                       │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                     HTTP Filter Chain                                    │ │
│  │                                                                          │ │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────┐ ┌───────────┐ ┌─────────────┐ │ │
│  │  │ API Key  │ │ Format   │ │ Prompt    │ │ Semantic  │ │ Token       │ │ │
│  │  │ Auth     │ │ Adapter  │ │ Guard     │ │ Cache     │ │ Counter     │ │ │
│  │  └──────────┘ └──────────┘ └───────────┘ └───────────┘ └─────────────┘ │ │
│  │                                                                          │ │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────────────┐ │ │
│  │  │ Model Router │ │ OTel Trace   │ │ Langfuse Export                  │ │ │
│  │  │ (fallback)   │ │ (W3C ctx)    │ │ (trace + generation + score)     │ │ │
│  │  └──────────────┘ └──────────────┘ └──────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                    AI Observability Layer                                │ │
│  │                                                                          │ │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────────────────┐  │ │
│  │  │ Prometheus │ │ OpenTele-  │ │ Langfuse   │ │ Structured Access    │  │ │
│  │  │ Metrics    │ │ metry Span │ │ Ingestion  │ │ Logs (JSON)          │  │ │
│  │  └────────────┘ └────────────┘ └────────────┘ └──────────────────────┘  │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────────────────┘
```

---

## Cross-Phase Infrastructure (Throughout All Phases)

These capabilities need to be designed from Phase 1 and gradually refined.

### Multi-API Format Support

Internal unified `AIRequest` / `AIResponse` IR, with each provider format converted via adapter:

| Provider | Format | Request Path | Special Handling |
|---|---|---|---|
| **OpenAI** | OpenAI Chat Completions API | `/v1/chat/completions` | Baseline format |
| **Anthropic** | Anthropic Messages API | `/v1/messages` | system as top-level field, stop_sequences |
| **Ollama** | Ollama API | `/api/generate`, `/api/chat` | Local deployment, no auth |
| **vLLM** | OpenAI-compatible | `/v1/chat/completions` | Same as OpenAI, custom sampling params |
| **Google AI** | Gemini API | `/v1/models/{model}:generateContent` | contents array structure |
| **Mistral** | OpenAI-compatible | `/v1/chat/completions` | La Plateforme compatible |
| **Groq** | OpenAI-compatible | `/v1/chat/completions` | Ultra-low latency LPU |
| **DeepSeek** | OpenAI-compatible | `/v1/chat/completions` | Chinese model |
| **Azure OpenAI** | OpenAI-compatible | `/{deployment}/chat/completions` | api-version query param |
| **Custom** | OpenAI-compatible | Configurable | Any compatible endpoint |

```
Client ──▶ Gateway (/v1/chat/completions)
               │
               ▼
          Format Adapter (auto-selected based on AIService.spec.format)
               │
               ├── OpenAI    → Pass-through (internal baseline format)
               ├── Anthropic → Messages API format conversion
               ├── Ollama    → /api/chat format conversion
               └── Custom    → Template conversion
```

### Observability Matrix

| Signal | Implementation | Phase 1 | Phase 2 | Phase 3 |
|---|---|---|---|---|
| **Metrics** | Prometheus (histogram + counter + gauge) | Basic token metrics | Cost + cache | A/B experiments |
| **Tracing** | OpenTelemetry (W3C Trace Context) | Span propagation | Full trace context | trace→metric correlation |
| **LLM Observability** | Langfuse SDK | trace + generation | score + prompt | eval datasets |
| **Structured Logging** | JSON access log | Token auditing | PII redacted logs | Compliance export |
| **Dashboards** | Grafana + Langfuse UI | Usage panel | Cost panel | Experiment panel |

### OpenTelemetry Evolution Path

```
Phase 1: Span propagation + basic traces
  ├── Inject W3C traceparent header to backend requests
  ├── Create span per request (model, tokens, latency as attributes)
  ├── OTLP gRPC export to OTel Collector
  └── Reuse existing otel log infrastructure

Phase 2: Full-chain traces
  ├── Independent span per filter chain step
  ├── upstream connect + first token child span
  ├── Streaming chunk span (batched)
  └── Trace sampling strategy (by model / error / latency)

Phase 3: Advanced tracing features
  ├── Baggage propagation (user_id, session_id)
  ├── trace→metric correlation (exemplar)
  ├── Span Events (cache hit/miss, guard block, fallback)
  └── Custom OTel Sampler (by token budget)
```

### Langfuse Integration

```
Phase 1: Basic ingestion
  ├── Trace: conversation level (trace_id = request_id)
  ├── Generation: each LLM call (input/output tokens, model, latency)
  ├── Observation: streaming chunks aggregation
  └── Config: LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_HOST

Phase 2: Evaluation + prompt management
  ├── Score: automatic quality scoring (latency, cost, guard triggers)
  ├── Prompt: Langfuse Prompt Management integration
  └── Custom metadata (user_id, session_id, route)

Phase 3: Advanced features
  ├── Dataset: automatic eval dataset recording
  ├── Experiment: A/B test result correlation
  └── Webhook: anomaly alerts (cost spike, guard trigger rate)
```

---

## Phase 1: Minimum Viable AI Gateway (2-3 weeks)

> **Status: COMPLETE** (2026-05-30)
>
> All 17 tasks completed. Deliverables: `aeg-ai` Rust crate with 3 format adapters (OpenAI, Anthropic, Ollama), unified AI IR, `AIGatewayFilter` filter chain, `TokenCounter`, Prometheus metrics, OTel tracing, Langfuse ingestion, `AIService` CRD with Go translator, and AI overview dashboard. Design documentation published in `docs/design/ai-gateway/` and `docs/developer/ai-gateway-testing.md`.

### Goal
OpenAI-compatible API proxy + multi-format adaptation (OpenAI/Anthropic/Ollama) + token counting + basic OTel + Langfuse + Dashboard

### Code Structure

```
dataplane/crates/
├── aeg-ai/                       # New: AI Gateway core crate
│   ├── Cargo.toml
│   ├── src/
│   │   ├── lib.rs                # crate root
│   │   ├── format/               # Multi-format adaptation
│   │   │   ├── mod.rs
│   │   │   ├── openai.rs         # OpenAI Chat Completions
│   │   │   ├── anthropic.rs      # Anthropic Messages
│   │   │   ├── ollama.rs         # Ollama API
│   │   │   └── ir.rs             # Unified IR (AIRequest / AIResponse)
│   │   ├── token.rs              # Token counting + usage parsing
│   │   ├── filter.rs             # Rust proxy HTTP filter (pre + post processing)
│   │   ├── observability/        # Observability
│   │   │   ├── mod.rs
│   │   │   ├── metrics.rs        # Prometheus metrics
│   │   │   ├── tracing.rs        # OTel span
│   │   │   └── langfuse.rs       # Langfuse ingestion
│   │   ├── types.rs              # Shared types
│   │   └── error.rs              # Error types
│   └── tests/
│       ├── format_openai.rs
│       ├── format_anthropic.rs
│       ├── format_ollama.rs
│       ├── token_counter.rs
│       ├── filter_integration.rs
│       └── observability.rs

controlplane/
├── config/crd/bases/
│   └── gateway.aether.dev_aiservices.yaml  # AIService CRD
├── internal/
│   ├── translator/
│   │   └── ai_service.go         # AIService CRD → IR translation
│   └── ...
└── ...

dashboard/src/
├── app/[locale]/ai/              # AI Gateway pages
│   ├── overview/page.tsx         # Overview: model list, usage overview
│   ├── services/page.tsx         # AIService management
│   ├── usage/page.tsx            # Token usage analysis
│   └── traces/page.tsx           # Trace viewer (Langfuse embedded or self-built)
├── components/ai/                # AI-related components
│   ├── model-card.tsx
│   ├── token-chart.tsx
│   ├── latency-chart.tsx
│   └── trace-viewer.tsx
└── hooks/use-api/
    └── use-ai.ts                 # AI metrics API hooks

docs/
├── roadmap/ai-gateway.md         # This document
├── design/ai-gateway/            # Design documents
│   ├── architecture.md           # Architecture design
│   ├── multi-format.md           # Multi-format adaptation design
│   ├── observability.md          # Observability design
│   └── crd-aiservice.md          # AIService CRD API reference
└── developer/
    └── ai-gateway-testing.md     # Testing guide

tests/
└── e2e/
    ├── validate-ai-gateway-openai.sh
    ├── validate-ai-gateway-anthropic.sh
    ├── validate-ai-gateway-ollama.sh
    ├── validate-ai-gateway-streaming.sh
    └── validate-ai-observability.sh
```

### Tasks

| # | Task | Location | Effort | Tests | Status |
|---|---|---|---|---|---|
| 1 | **Unified AI IR definition** — `AIRequest`, `AIResponse`, `AIUsage`, `AIStreamChunk` | `aeg-ai/src/format/ir.rs` | 1d | Unit tests | ✅ |
| 2 | **OpenAI format adapter** — request/response → IR bidirectional conversion | `aeg-ai/src/format/openai.rs` | 1d | Unit tests + snapshot | ✅ |
| 3 | **Anthropic format adapter** — Messages API → IR | `aeg-ai/src/format/anthropic.rs` | 1d | Unit tests + snapshot | ✅ |
| 4 | **Ollama format adapter** — `/api/chat` → IR | `aeg-ai/src/format/ollama.rs` | 1d | Unit tests + snapshot | ✅ |
| 5 | **Token Counter filter** — stream/non-stream response body `usage` field parsing | `aeg-ai/src/token.rs` | 1d | Unit tests | ✅ |
| 6 | **AIGatewayFilter** — Rust proxy HTTP filter with pre + post processing | `aeg-ai/src/filter.rs` | 2d | Integration tests | ✅ |
| 7 | **Prometheus metrics** — histogram + counter | `aeg-ai/src/observability/metrics.rs` | 1d | Integration tests | ✅ |
| 8 | **OTel tracing** — span propagation + parent span creation | `aeg-ai/src/observability/tracing.rs` | 1d | Integration tests | ✅ |
| 9 | **Langfuse integration** — trace + generation ingestion | `aeg-ai/src/observability/langfuse.rs` | 2d | Integration tests | ✅ |
| 10 | **AIService CRD** — provider, endpoint, model, api_key, format definition | `controlplane/config/crd/bases/` | 1d | Unit tests | ✅ |
| 11 | **AIService translator** — CRD → IR | `controlplane/internal/translator/ai_service.go` | 2d | Unit tests | ✅ |
| 12 | **AI Gateway Dashboard** — overview page | `dashboard/src/app/[locale]/ai/overview/` | 2d | Visual testing | ✅ |
| 13 | **AI Gateway Dashboard** — service management page | `dashboard/src/app/[locale]/ai/services/` | 1d | Visual testing | ✅ |
| 14 | **AI Gateway Dashboard** — usage analysis page | `dashboard/src/app/[locale]/ai/usage/` | 1d | Visual testing | ✅ |
| 15 | **AI Gateway Dashboard** — trace viewer page | `dashboard/src/app/[locale]/ai/traces/` | 1d | Visual testing | ✅ |
| 16 | **E2E tests** — OpenAI, Anthropic, Ollama, streaming, observability | `tests/e2e/validate-ai-*.sh` | 2d | E2E | ✅ |
| 17 | **Documentation** — architecture, multi-format, observability, CRD, testing | `docs/design/ai-gateway/`, `docs/developer/` | 2d | Doc review | ✅ |

### New Metrics (Phase 1)

```
# Token usage
aeg_ai_requests_total{model, provider, format}                          counter
aeg_ai_input_tokens_total{model, provider}                              counter
aeg_ai_output_tokens_total{model, provider}                             counter
aeg_ai_latency_seconds{model, provider}                                 histogram
aeg_ai_first_token_latency_seconds{model, provider}                     histogram

# Streaming
aeg_ai_streaming_requests_total{model, provider}                        counter

# Format adapter
aeg_ai_format_adapter_errors_total{format, error_type}                  counter

# Observability
aeg_ai_langfuse_export_errors_total{error_type}                         counter
aeg_ai_langfuse_export_latency_seconds                                  histogram
```

---

## Phase 2: Production-Ready AI Gateway (4-6 weeks)

### Goal
Token-based rate limiting, prompt guard, semantic cache, model fallback, cost tracking, API key management, full-chain OTel trace, Langfuse score + prompt

| # | Task | Description | Effort | Tests |
|---|---|---|---|---|---|
| 18 | **TokenPolicy CRD** | token/min rate limiting, supports api_key / model granularity | 3d | Unit + integration |
| 19 | **Token Rate Limiter filter** | Parse response `usage.total_tokens`, sliding window rate limiting | 4d | Unit + integration + e2e |
| 20 | **Prompt Guard filter** | Regex + keyword detection of injection attacks (DAN, ignore instructions, etc.), optional LLM detection | 3d | Unit + integration |
| 21 | **Semantic Cache filter** | embedding similarity matching → cache hit returns directly (pgvector / qdrant / redis) | 5d | Unit + integration |
| 22 | **Model Fallback** | Main model timeout/error → auto-degrade to fallback (AIService.spec.fallbacks[]) | 3d | Unit + integration |
| 23 | **Cost Tracker** | `tokens × $/1K tokens` real-time cost, Prometheus counter + Langfuse score | 2d | Unit |
| 24 | **API Key Management** | Gateway-layer API key verification → mapped to backend key (key ring rotation) | 3d | Unit + integration |
| 25 | **Cost Dashboard** | Cost analysis by model/user/time + Langfuse panel | 2d | Component tests |
| 26 | **OTel full-chain trace** | Per-step span in filter chain + upstream connect + first token child span | 2d | Integration tests |
| 27 | **Langfuse score + prompt** | Automatic scoring + Prompt Management integration | 2d | Integration tests |

### New Metrics (Phase 2)

```
# Cost
aeg_ai_cost_dollars_total{model, provider, user}                      counter
aeg_ai_cost_per_request_dollars{model}                                histogram

# Cache
aeg_ai_cache_hits_total{model}                                        counter
aeg_ai_cache_misses_total{model}                                      counter
aeg_ai_cache_latency_seconds                                          histogram

# Guard
aeg_ai_prompt_guard_blocks_total{reason, model}                       counter

# Fallback
aeg_ai_fallback_total{from_model, to_model, reason}                   counter

# Rate limit
aeg_ai_token_rate_limit_hits_total{model, scope="api_key|model"}      counter
```

---

## Phase 3: Enterprise AI Gateway (8-12 weeks)

### Goal
PII redaction, content safety, multi-model routing, A/B testing, multi-tenancy, complete OTel

| # | Task | Description | Effort |
|---|---|---|---|
| 28 | **PII Redaction** | Auto-masking of email, phone, ID card, credit card in requests/responses | 4d |
| 29 | **Content Safety** | Output detection (NSFW, violence, bias, jailbreak) → block or flag | 5d |
| 30 | **Multi-Model Routing** | Auto-route by query complexity: simple → cheap model, complex → expensive model | 5d |
| 31 | **Prompt Template Injection** | Gateway-layer injection of system prompt, RAG context, few-shot examples | 3d |
| 32 | **A/B Testing** | Distribute traffic proportionally to different model versions, compare quality metrics | 4d |
| 33 | **Multi-Tenancy** | Isolate quota, cost, models by namespace / api_key | 5d |
| 34 | **OTel Advanced Features** | Baggage propagation + exemplar + Span Events + custom Sampler | 3d |
| 35 | **Langfuse Advanced Features** | Dataset + Experiment + Webhook alerts | 3d |
| 36 | **Gateway API conformance** | Submit AI Gateway profile (if accepted by SIG) | 5d |

### New Metrics (Phase 3)

```
# PII
aeg_ai_pii_detected_total{type="email|phone|id_card|credit_card"}     counter

# Content safety
aeg_ai_content_safety_total{category, verdict="block|flag|pass"}      counter

# Model routing
aeg_ai_model_route_decision_total{complexity="simple|medium|complex", chosen_model}

# A/B testing
aeg_ai_ab_test_request_total{experiment, variant}                     counter
aeg_ai_ab_test_latency_seconds{experiment, variant}                   histogram
aeg_ai_ab_test_quality_score{experiment, variant}                     gauge
```

---

## Implementation Priority Summary

```
Phase 1 Week 1-2 (P0 - Core Features)
├── Unified AI IR + OpenAI format adapter
├── Anthropic + Ollama format adapters
├── AIService CRD + translator
├── Token Counter filter
├── Format Adapter filter
└── Unit tests + snapshot tests

Phase 1 Week 2-3 (P1 - Observability + Dashboard)
├── AI metrics (Prometheus)
├── OTel span propagation
├── Langfuse integration
├── AI Gateway Dashboard (overview + services + usage + traces)
├── Dashboard style unification
├── e2e tests (OpenAI/Anthropic/Ollama/streaming/observability)
└── Documentation (architecture, multi-format, observability, CRD, testing)

Phase 2 (Production Ready)
├── Token Policy + Rate Limiter
├── Prompt Guard
├── Semantic Cache
├── Model Fallback
├── Cost Tracker + Dashboard
├── API Key Management
├── OTel full-chain trace
└── Langfuse score + prompt

Phase 3 (Enterprise)
├── PII Redaction + Content Safety
├── Multi-Model Routing + Prompt Templates
├── A/B Testing + Multi-Tenancy
├── OTel Advanced Features
├── Langfuse Advanced Features
└── Gateway API conformance
```

---

## Key Design Decisions

| Decision | Choice | Reason |
|---|---|---|
| CRD group | `gateway.aether.dev` | Does not pollute `gateway.networking.k8s.io`, follows Gateway API extension conventions |
| Internal IR | Unified `AIRequest`/`AIResponse`/`AIStreamChunk` | All format adapters output unified IR, filter chain only processes IR |
| Filter implementation | Rust proxy HTTP filter + pre/post processing | Reuses existing proxy filter chain, request/response body is interceptable |
| Format adapter | Independent per-provider modules | Separation of concerns, adding a provider only requires one adapter file |
| Token parsing | response body JSON parse | SSE scenarios require aggregating chunks before parsing |
| Auth injection | Translator layer injects Bearer token into IR backendRef | Does not expose API key to dataplane config |
| Cache backend | Pluggable (pgvector / Qdrant / Redis) | Different scales choose different solutions |
| Metrics | Prometheus histogram + counter | Reuses existing `aeg-observability` infrastructure |
| Tracing | OpenTelemetry (W3C Trace Context + OTLP gRPC) | Standard protocol, shares collector with existing otel log |
| LLM Observability | Langfuse SDK (Rust side) + Langfuse UI | Dedicated LLM observability platform, integrated trace/generation/score/eval |
| Dashboard style | Unified design tokens (color, typography, spacing, radius) | Cross-page consistency, easy theme switching |
| Testing | TDD-first, independent testing per layer | Unit → snapshot → integration → e2e → visual |
| Documentation | Every CRD has an API ref, every filter has a design doc | Code as documentation, but architectural decisions need explicit recording |