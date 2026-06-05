# AI Gateway Observability

Observability is embedded at every stage of the AI request pipeline. Three signals are produced: Prometheus metrics (counter + histogram), OpenTelemetry tracing (spans with W3C context), and Langfuse ingestion (traces + generations for LLM observability).

## Prometheus Metrics

All metrics live in `AIMetrics` in `aeg-ai/src/observability/metrics.rs`. They are registered with an explicit `prometheus::Registry` (not the global default), so tests can use isolated registries.

### CounterVecs

| Metric Name | Type | Labels | Description |
|---|---|---|---|
| `ai_tokens_total` | IntCounterVec | `model`, `direction` (`prompt`\|`completion`) | Total token counts accumulated from response usage |
| `ai_requests_total` | IntCounterVec | `model`, `format`, `status` (`success`\|`error`) | Total AI request count per model, format, and outcome |
| `ai_stream_events_total` | IntCounterVec | `model`, `event_type` (`content`\|`done`\|`error`) | Streaming event count by type |
| `ai_backend_errors_total` | IntCounterVec | `model`, `status_code` | Backend LLM provider errors by HTTP status code |
| `ai_format_errors_total` | IntCounterVec | `format`, `reason` | Format serialisation/deserialisation errors |
| `ai_langfuse_ingestions_total` | IntCounterVec | `ingestion_type` (`trace`\|`generation`) | Langfuse ingestion counts by payload type |

### Single Counters

| Metric Name | Type | Description |
|---|---|---|
| `ai_otel_spans_exported_total` | IntCounter | Number of OTel spans successfully exported |
| `ai_otel_export_errors_total` | IntCounter | Number of OTel export failures |
| `ai_langfuse_errors_total` | IntCounter | Number of Langfuse ingestion errors |

### HistogramVecs

| Metric Name | Type | Labels | Buckets | Description |
|---|---|---|---|---|
| `ai_request_duration_seconds` | HistogramVec | `model`, `provider` | 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0 | End-to-end request duration in seconds |
| `ai_first_token_latency_seconds` | HistogramVec | `model`, `provider` | 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0 | Time-to-first-token latency in seconds |
| `ai_tokens_per_request` | HistogramVec | `model`, `provider` | 10, 50, 100, 500, 1000, 5000, 10000 | Total tokens (prompt + completion) per request |

### Metric Recording Methods

`AIMetrics` exposes helper methods for recording from the filter:

- `record_tokens(model, prompt_tokens, completion_tokens)` - Increments `ai_tokens_total` for both `prompt` and `completion` directions.
- `record_request(model, format, status, duration_secs)` - Increments `ai_requests_total` and observes `ai_request_duration_seconds`.
- `record_stream_event(model, event_type)` - Increments `ai_stream_events_total`.
- `record_backend_error(model, status_code)` - Increments `ai_backend_errors_total`.
- `record_format_error(format, reason)` - Increments `ai_format_errors_total`.
- `record_langfuse_ingestion(ingestion_type)` - Increments `ai_langfuse_ingestions_total`.
- `record_otel_span_exported()` - Increments `ai_otel_spans_exported_total`.
- `record_otel_export_error()` - Increments `ai_otel_export_errors_total`.
- `record_langfuse_error()` - Increments `ai_langfuse_errors_total`.

The constructor `AIMetrics::new(registry: &Registry)` registers all metrics and returns `Result<Self, prometheus::Error>`.

## OpenTelemetry Tracing

`AITracer` in `aeg-ai/src/observability/tracing.rs` creates and manages OTel spans:

```rust
pub struct AITracer {
    tracer: sdk_trace::Tracer,
    provider: sdk_trace::SdkTracerProvider,
}
```

**Constructor:**
- If `exporter_endpoint` is empty, a noop tracer is created that does not export spans.
- Otherwise, an OTLP gRPC exporter is configured with batch processing and a `Resource` carrying the service name.

**Span creation:**
```rust
pub fn start_span(&self, name: &str, model: &str, format: &str, stream: bool) -> AISpan;
```

Span attributes set at creation:
- `ai.model` (string)
- `ai.format` (string)
- `ai.stream` (boolean)

The span uses `Context::current()` for W3C Trace Context propagation via the `tracer.start_with_context()` method.

**Span completion:**
```rust
pub fn end_span(&self, span_obj: AISpan, prompt_tokens: u64, completion_tokens: u64);
```

Additional attributes set at completion:
- `ai.usage.prompt_tokens` (i64)
- `ai.usage.completion_tokens` (i64)

**`AISpan` struct:**
```rust
pub struct AISpan {
    pub name: String,
    pub start: Instant,
    span: sdk_trace::Span,
}
```

**Filter integration:** In `AIGatewayFilter::post_process()`, the tracer is used after all other processing:
```rust
if let Some(ref tracer) = self.tracer {
    let prompt_tokens = usage.as_ref().map_or(0, |u| u.prompt_tokens);
    let completion_tokens = usage.as_ref().map_or(0, |u| u.completion_tokens);
    let span = tracer.start_span("ai.inference", &model, &format, is_stream);
    tracer.end_span(span, prompt_tokens, completion_tokens);
}
```

## Langfuse Integration

`LangfuseClient` in `aeg-ai/src/observability/langfuse.rs` ingests traces and generations into Langfuse via its public REST API.

### Client Setup

```rust
pub fn new(public_key: &str, secret_key: &str, host: &str) -> Self;
```

If `public_key` is empty, the client operates in **noop mode**: all `ingest_*` methods return `Ok(())` without making HTTP calls. This is the production-safe default when Langfuse is not configured.

When enabled, the client uses HTTP Basic Auth (base64-encoded `public_key:secret_key`) with `Content-Type: application/json`.

### Ingestion API

**Trace ingestion:**
```rust
pub async fn ingest_trace(
    &self, trace_id: &str, user_id: Option<&str>,
    session_id: Option<&str>, metadata: &BTreeMap<String, String>,
) -> Result<(), anyhow::Error>;
```

Posts to `{host}/api/public/traces` with payload:
```json
{
    "traceId": "...",
    "userId": "...",
    "sessionId": "...",
    "metadata": {},
    "timestamp": "2026-05-30T..."
}
```

**Generation ingestion:**
```rust
pub async fn ingest_generation(
    &self, trace_id: &str, model: &str,
    input_tokens: u64, output_tokens: u64, latency_ms: u64,
    input: &serde_json::Value, output: &serde_json::Value,
    metadata: &BTreeMap<String, String>,
) -> Result<(), anyhow::Error>;
```

Posts to `{host}/api/public/generations` with payload:
```json
{
    "traceId": "...",
    "model": "gpt-4o",
    "usage": { "input": 150, "output": 80, "total": 230 },
    "latency": 1.234,
    "input": {...},
    "output": {...},
    "metadata": {}
}
```

Latency is converted from milliseconds to fractional seconds (e.g., 1234ms -> 1.234). Total tokens is computed as `input_tokens + output_tokens`.

**Noop mode:** `LangfuseClient::noop_client()` creates a client with empty credentials. Calling `noop_client()` is equivalent to `LangfuseClient::new("", "", "")`.

### Filter Integration

In `AIGatewayFilter::post_process()`, Langfuse ingestion runs after metrics recording:

1. A new UUID is generated as the trace ID.
2. The raw request body (`ctx.raw_request`) is parsed into a JSON value for the input field.
3. The provider response body is parsed into a JSON value for the output field.
4. `ingest_trace()` is called with no user ID, session ID, or metadata.
5. If usage is available (from either the non-streaming response or accumulated from stream chunks), `ingest_generation()` is called with the model name, token counts, latency in milliseconds, and the JSON input/output values.

Both calls use `let _ = ...` to silently ignore ingestion errors (metrics capture errors separately via `langfuse_errors` counter).

## TokenCounter

`TokenCounter` in `aeg-ai/src/token.rs` accumulates token usage from responses:

```rust
pub struct TokenCounter {
    pub prompt_tokens: u64,
    pub completion_tokens: u64,
    pub total_tokens: u64,
}
```

**Methods:**

- `new()` - Creates a counter with all zero values.
- `record_response(&mut self, usage: &AIUsage)` - Adds usage fields from a non-streaming response.
- `record_stream_chunk(&mut self, chunk: &AIStreamChunk)` - Reads `chunk.usage` (typically only present on the final chunk) and adds its fields.
- `accumulated_usage(&self) -> AIUsage` - Returns current accumulators as an `AIUsage` struct.

**SSE body parsing:**
```rust
pub fn from_sse_body(body: &[u8]) -> Result<(AIUsage, String), AIError>;
```

Parses a raw SSE stream body:
1. Splits on `\n\n` to separate events.
2. For each event, splits on `\n` to find `data: ` lines.
3. Skips empty events and `[DONE]` markers.
4. Parses each `data:` payload as JSON `AIStreamChunk`.
5. Accumulates usage from each chunk via `record_stream_chunk()`.
6. Concatenates delta content strings across all choices.
7. Returns the accumulated usage and the concatenated content.

This method is used for post-hoc analysis of captured SSE bodies. The filter uses the chunk-based approach directly for streaming requests.
