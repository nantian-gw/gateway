# AI Gateway Testing Guide

## Rust Tests (aeg-ai)

All Rust tests live inline in the `dataplane/crates/aeg-ai/` crate using `#[cfg(test)]` modules.

```bash
# Run all aeg-ai tests
cargo test -p aeg-ai --manifest-path dataplane/Cargo.toml

# Run tests for a specific module
cargo test -p aeg-ai --manifest-path dataplane/Cargo.toml -- format
cargo test -p aeg-ai --manifest-path dataplane/Cargo.toml -- token
cargo test -p aeg-ai --manifest-path dataplane/Cargo.toml -- metrics
```

### Test Areas

| Area | Source File | Test Focus |
|---|---|---|
| Format detection | `src/format/mod.rs` | `detect_format()` path matching for all providers |
| Adapter registry | `src/format/mod.rs` | Register, get, and default registry behavior |
| OpenAI adapter | `src/format/openai.rs` | Parse request/response, serialize response/stream, error format |
| Anthropic adapter | `src/format/anthropic.rs` | System message prepend, content block handling, bidirectional conversion |
| Ollama adapter | `src/format/ollama.rs` | Options extraction, usage mapping from eval counts, stream serialization |
| Token counter | `src/token.rs` | Accumulator behavior, SSE body parsing with realistic chunks |
| Prometheus metrics | `src/observability/metrics.rs` | Isolated registry tests, label value recording |

## Go Tests (Control-Plane)

```bash
# Run translator tests (includes AIService translation tests)
cd controlplane && go test ./internal/translator/...

# Run all control-plane tests
cd controlplane && go test ./...
```

### Key Test Files

- `controlplane/internal/translator/ai_service_test.go` - Tests the `translateAIService()` function: field mapping, namespace/secret assembly, timeout parsing (valid and invalid values), and the translator's handling of omitted optional fields.

## Dashboard Tests

```bash
# Run dashboard validation (type-checking + lint)
cd dashboard && npm run check

# Run dashboard tests
cd dashboard && npm test
```

Dashboard tests cover component rendering, API hook mocking, and page-level integration for the AI pages.

## Test Patterns

### Format Adapter Round-Trip Tests

The most important test pattern for format adapters is round-trip verification: a provider-specific JSON payload is parsed into AI IR, then serialized back to provider JSON, and the output is compared to the expected result.

```rust
#[test]
fn test_openai_round_trip() {
    let adapter = OpenAIAdapter;

    // Parse a request
    let request_body = r#"{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}"#;
    let ir = adapter.parse_request(request_body.as_bytes()).unwrap();
    assert_eq!(ir.model, "gpt-4o");
    assert_eq!(ir.messages.len(), 1);

    // Parse a response
    let response_body = r#"{
        "id":"chatcmpl-123",
        "model":"gpt-4o",
        "choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
        "usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
    }"#;
    let ai_response = adapter.parse_response(response_body.as_bytes()).unwrap();

    // Serialize back
    let serialized = adapter.serialize_response(&ai_response).unwrap();
    let back: serde_json::Value = serde_json::from_slice(&serialized).unwrap();
    assert_eq!(back["model"], "gpt-4o");
}
```

### TokenCounter SSE Parsing

Tests for SSE body parsing verify correct accumulation from realistic streaming payloads:

```rust
#[test]
fn test_from_sse_body() {
    let sse = "\
data: {\"id\":\"1\",\"model\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\
\n\
data: {\"id\":\"2\",\"model\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\
\n\
data: [DONE]\n\
\n";

    let (usage, content) = TokenCounter::from_sse_body(sse.as_bytes()).unwrap();
    assert_eq!(content, "Hello world");
    assert_eq!(usage.prompt_tokens, 10);
    assert_eq!(usage.completion_tokens, 2);
    assert_eq!(usage.total_tokens, 12);
}
```

Key test cases for SSE:
- Single chunk with usage
- Multiple chunks with usage only on final chunk
- `[DONE]` terminator handling
- Empty events (blank lines between events)
- Malformed JSON in a data line (expects error)

### Filter Integration Tests

Integration tests exercise the full `AIGatewayFilter` flow:

```rust
#[test]
fn test_filter_non_streaming() {
    let mut registry = AdapterRegistry::new();
    registry.register("openai", Arc::new(OpenAIAdapter));
    let reg = Arc::new(registry);

    let prom_registry = Registry::new();
    let metrics = Arc::new(AIMetrics::new(&prom_registry).unwrap());

    let filter = AIGatewayFilter::new(
        reg, metrics, None, None  // No Langfuse, no OTel
    );

    // Test pre_process
    let ctx = tokio_test::block_on(
        filter.pre_process("/v1/chat/completions", REQUEST_JSON.as_bytes())
    ).unwrap();
    assert_eq!(ctx.format, "openai");
    assert_eq!(ctx.request.model, "gpt-4o");

    // Test post_process
    let output = tokio_test::block_on(
        filter.post_process(ctx, RESPONSE_JSON.as_bytes(), 200)
    ).unwrap();
    let output_val: serde_json::Value = serde_json::from_slice(&output).unwrap();
    assert_eq!(output_val["model"], "gpt-4o");
}

#[test]
fn test_filter_streaming() {
    // Similar setup, verify SSE chunk parsing, reformatting, and usage accumulation
}
```

### Metric Verification with Isolated Registry

Always use a fresh `prometheus::Registry` for metric tests to avoid state leakage:

```rust
#[test]
fn test_ai_metrics_recording() {
    let registry = Registry::new();
    let metrics = AIMetrics::new(&registry).unwrap();

    metrics.record_tokens("gpt-4o", 100, 50);
    metrics.record_request("gpt-4o", "openai", "success", 1.5);

    // Gather and verify
    let families = registry.gather();
    // Find and assert on specific metric families
}
```

## Adding a New Format Adapter: Test Checklist

When adding a new format adapter, create tests for:

1. **Request parsing** - Valid JSON produces correct `AIRequest` with all fields populated.
2. **Request parsing - minimal** - Request with only required fields (model, messages) parses without error.
3. **Response parsing** - Valid response JSON produces correct `AIResponse` with usage.
4. **Response parsing - no usage** - Response without usage field produces `AIResponse` with `None` usage.
5. **Response serialization** - `AIResponse` re-serializes to valid provider-format JSON.
6. **Stream chunk serialization** - `AIStreamChunk` yields correct SSE or newline-delimited format.
7. **Error response** - `error_response()` produces expected error JSON.
8. **Invalid JSON** - Malformed request/response bodies produce `AIError::FormatParse`.
9. **Content edge cases** - Empty content, null content, multi-part content where applicable.

## End-to-End Tests

E2E test scripts are in `tests/e2e/` and validate full pipeline behavior in a Kind cluster:

```bash
# Individual validation scripts
./tests/e2e/validate-ai-gateway-openai.sh
./tests/e2e/validate-ai-gateway-anthropic.sh
./tests/e2e/validate-ai-gateway-ollama.sh
./tests/e2e/validate-ai-gateway-streaming.sh
./tests/e2e/validate-ai-observability.sh

# Full AI Gateway e2e suite
SKIP_BUILD=true ./tests/e2e/run-kind.sh
```

E2E tests require a Kind cluster with the gateway deployed. These are the most expensive tests and should only be run when format adapter or filter changes affect the full proxy pipeline.