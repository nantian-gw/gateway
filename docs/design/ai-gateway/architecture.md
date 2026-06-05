# AI Gateway Architecture

The AI Gateway extends aether-gateway's HTTP proxy with multi-format AI API support, unified internal representation (IR), and embedded observability. It lets clients send requests in any supported format (OpenAI, Anthropic, Ollama) and routes them to backend LLM providers with automatic format conversion.

## Request Flow

Each AI request passes through a filter chain built on `AIGatewayFilter` in the `aeg-ai` crate:

```
Client Request
     |
     v
OTel Span (root)            - Start "ai.inference" span with model, format, stream attributes
     |
     v
detect_format(path)         - Determine format from URL path
     |
     v
Adapter.parse_request()     - Deserialize provider JSON into AIRequest IR
     |
     v
Upstream (provider backend) - Forward original request body to the configured LLM provider
     |
     v
Adapter.parse_response()    - Parse provider response into AIResponse IR
     |
     v
Token Counter               - Accumulate prompt/completion/total token counts
     |
     v
Prometheus Metrics          - Record tokens_total, requests_total, request_duration, etc.
     |
     v
Langfuse Ingestion          - Ingest trace and generation to Langfuse API
     |
     v
OTel Span (end)             - Set prompt_tokens, completion_tokens attributes; export
     |
     v
Adapter.serialize_response() - Convert AIResponse IR back to the client's expected format
     |
     v
Client Response
```

For streaming requests, the response body is parsed as SSE chunks, each chunk is deserialized into `AIStreamChunk`, token usage is accumulated from the final chunk, and each chunk is re-serialized through the format adapter before being returned to the client.

## Crate Structure

The AI Gateway lives in `dataplane/crates/aeg-ai/`:

```
aeg-ai/
  src/
    lib.rs              - Crate root, forbids unsafe code
    format/
      mod.rs            - FormatAdapter trait, AdapterRegistry, detect_format()
      ir.rs             - Unified IR types: AIRequest, AIResponse, AIStreamChunk, etc.
      openai.rs         - OpenAIAdapter for /v1/chat/completions
      anthropic.rs      - AnthropicAdapter for /v1/messages
      ollama.rs         - OllamaAdapter for /api/chat
    filter.rs            - AIGatewayFilter: pre_process() + post_process()
    token.rs             - TokenCounter: usage accumulation and SSE body parsing
    observability/
      mod.rs             - Module re-exports
      metrics.rs         - AIMetrics: Prometheus counters, histograms
      tracing.rs         - AITracer: OpenTelemetry spans via OTLP gRPC
      langfuse.rs        - LangfuseClient: trace + generation ingestion
    types.rs             - AIProviderInfo struct
    error.rs             - AIError enum
```

## Filter Chain Details

`AIGatewayFilter` holds references to the adapter registry, metrics, Langfuse client, and OTel tracer. It exposes two methods:

**`pre_process(path, body)`** - Runs before the upstream call:
1. Calls `detect_format(path)` to map the request path to a format string (`"openai"`, `"anthropic"`, `"ollama"`).
2. Looks up the matching adapter from `AdapterRegistry`.
3. Calls `adapter.parse_request(body)` to produce an `AIRequest` IR.
4. Returns `AIContext` containing the format string, parsed `AIRequest`, start time, and raw request bytes.

**`post_process(ctx, response_body, response_status)`** - Runs after the upstream responds:
1. Gets the adapter from `ctx.format`.
2. For streaming requests: parses SSE chunks via `parse_sse_chunks()`, accumulates token usage with `TokenCounter`, re-serializes chunks through the adapter using `serialize_stream_chunk()`.
3. For non-streaming: parses the response via `adapter.parse_response()`, extracts usage, serializes through `adapter.serialize_response()`.
4. Records metrics: `tokens_total`, `requests_total`, `request_duration`, `format_errors_total`, `stream_events_total`, `backend_errors_total`.
5. If a Langfuse client is configured: generates a UUID trace ID, ingests a trace and generation with model, usage, latency, input, and output payloads.
6. If an OTel tracer is configured: creates and ends an `"ai.inference"` span with `ai.model`, `ai.format`, `ai.stream`, `ai.usage.prompt_tokens`, and `ai.usage.completion_tokens` attributes.

## Control-Plane Integration

AI services are configured declaratively through Kubernetes CRDs:

```
AIService CRD (gateway.nantian.dev/v1alpha1)
     |
     v
Translator (ai_service.go)
     - AIService.Spec -> ir.AIServiceConfig
     - Resolves auth secrets from namespace/secret name
     - Parses timeout string into time.Duration
     |
     v
IR Snapshot (BackendCluster.AIService)
     - Optional field on BackendCluster
     - Contains: provider, format, model, auth, timeout
     |
     v
Proto Encoding (control.proto)
     - BackendCluster.ai_service is AIServiceConfig message
     - Fields: provider, format, model, auth (AIServiceAuthConfig), timeout (Duration)
     |
     v
Data Plane (aeg-ir)
     - Ingest from proto snapshot, build runtime indexes
     - AIServiceConfig used to configure aeg-ai filter per-backend
```

The `BackendCluster` IR struct has an optional `AIService *AIServiceConfig` field. When present, the data plane attaches the `AIGatewayFilter` to the corresponding backend's HTTP proxy pipeline.

## Format Auto-Detection

`detect_format()` maps URL paths to format names without requiring the client to declare the format:

| Path Pattern | Format |
|---|---|
| `/v1/chat/completions`, `/v1/completions`, `/chat/completions` | `openai` |
| `/v1/messages` | `anthropic` |
| `/api/chat`, `/api/generate` | `ollama` |

vLLM and other OpenAI-compatible providers use the same `OpenAIAdapter` registered under a separate name (e.g., `"vllm"`) for routing differentiation. Since they speak the same JSON API, no code adapter changes are needed.

## Dashboard Integration

The dashboard surfaces AI Gateway metrics through dedicated pages under `dashboard/src/app/[locale]/ai/`:

- `overview/page.tsx` - KPI cards (total services, total tokens, total requests, average latency), token usage trend chart, latency chart
- Components: `token-chart.tsx`, `latency-chart.tsx`, loaded dynamically with SSR disabled
- Data fetched through `useAIOverview()` hook from `@/hooks/use-api`
- Supports i18n via `next-intl` translations
