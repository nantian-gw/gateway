# Multi-Format Adapter Design

The AI Gateway converts between provider-specific API formats and a unified internal representation (IR). This design lets clients send requests in any supported format and receive responses in that same format, regardless of which backend provider handles the request.

## FormatAdapter Trait

All format adapters implement the `FormatAdapter` trait defined in `aeg-ai/src/format/mod.rs`:

```rust
#[async_trait]
pub trait FormatAdapter: Send + Sync {
    /// Returns the adapter name, e.g. "openai", "anthropic", "ollama".
    fn name(&self) -> &'static str;

    /// Parse a provider-specific request body into AIRequest IR.
    fn parse_request(&self, body: &[u8]) -> Result<AIRequest, AIError>;

    /// Parse a provider-specific response body into AIResponse IR.
    fn parse_response(&self, body: &[u8]) -> Result<AIResponse, AIError>;

    /// Serialize an AIResponse IR back into the provider's response format.
    fn serialize_response(&self, response: &AIResponse) -> Result<Vec<u8>, AIError>;

    /// Serialize an AIStreamChunk IR back into the provider's SSE chunk format.
    fn serialize_stream_chunk(&self, chunk: &AIStreamChunk) -> Result<String, AIError>;

    /// Build an error response in the provider's expected error format.
    fn error_response(&self, status: u16, message: &str) -> Result<Vec<u8>, AIError>;
}
```

Each method maps between a provider's native JSON shapes and the unified IR types. The trait is object-safe through `async_trait`, and adapters must be `Send + Sync` so they can be shared across threads via `Arc`.

## Unified AI IR Types

Defined in `aeg-ai/src/format/ir.rs`, these are the canonical types that all adapters and filters consume:

### AIRequest

```rust
pub struct AIRequest {
    pub messages: Vec<AIMessage>,
    pub model: String,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
    pub top_p: Option<f32>,
    pub stop: Vec<String>,
    pub stream: bool,
    pub user: Option<String>,
    pub extra: BTreeMap<String, serde_json::Value>,
}
```

The `extra` field preserves provider-specific parameters not covered by the common fields, enabling lossless round-trip conversion.

### AIMessage

```rust
pub struct AIMessage {
    pub role: AIRole,
    pub content: AIContent,
    pub name: Option<String>,
    pub tool_calls: Vec<AIToolCall>,
    pub tool_call_id: Option<String>,
}
```

The `tool_calls` field stores function calls (OpenAI format). The `tool_call_id` field is populated on tool result messages. Both fields skip serialization when empty.

### AIRole

```rust
pub enum AIRole {
    System,
    User,
    Assistant,
    Tool,
}
```

Serialized with `#[serde(rename_all = "lowercase")]` so JSON uses `"system"`, `"user"`, `"assistant"`, `"tool"`.

### AIContent

```rust
pub enum AIContent {
    Text(String),
    MultiPart(Vec<AIContentPart>),
    None,
}
```

This `#[serde(untagged)]` enum handles three content shapes:

- **Text** - A plain string (most common).
- **MultiPart** - An array of content parts, each with a `type` field and optional `text` or `image_url` fields. Used for multimodal messages (images, mixed content).
- **None** - Represents a `null` content value. Used when an assistant message has tool calls but no text content. A custom `deserialize_null_as_none` deserializer maps JSON `null` to this variant, and the variant does not serialize at all (it produces no JSON value, which `#[serde(untagged)]` correctly handles).

### AIResponse

```rust
pub struct AIResponse {
    pub id: String,
    pub model: String,
    pub choices: Vec<AIChoice>,
    pub usage: Option<AIUsage>,
    pub created: Option<u64>,
    pub extra: BTreeMap<String, serde_json::Value>,
}
```

### AIStreamChunk

```rust
pub struct AIStreamChunk {
    pub id: String,
    pub model: String,
    pub choices: Vec<AIStreamChoice>,
    pub usage: Option<AIUsage>,
    pub created: Option<u64>,
}
```

### AIUsage

```rust
pub struct AIUsage {
    pub prompt_tokens: u64,
    pub completion_tokens: u64,
    pub total_tokens: u64,
}
```

### Related Types

- `AIChoice` - Contains `index: u32`, `message: AIMessage`, `finish_reason: Option<String>`.
- `AIStreamChoice` - Contains `index: u32`, `delta: AIStreamDelta`, `finish_reason: Option<String>`.
- `AIStreamDelta` - Contains `role: Option<AIRole>`, `content: Option<String>`, `tool_calls: Vec<AIToolCall>`.
- `AIToolCall` - Contains `id: String`, `call_type: String` (serialized as `"type"`), `function: AIToolCallFunction`.
- `AIToolCallFunction` - Contains `name: String`, `arguments: String`.
- `AIContentPart` - Contains `content_type: String` (`"type"`), `text: Option<String>`, `image_url: Option<AIImageUrl>`.
- `AIImageUrl` - Contains `url: String`, `detail: Option<String>`.

## AdapterRegistry and Auto-Detection

`AdapterRegistry` is a `HashMap<String, Arc<dyn FormatAdapter>>` that maps format names to adapter instances:

```rust
pub struct AdapterRegistry {
    adapters: HashMap<String, Arc<dyn FormatAdapter>>,
}

impl AdapterRegistry {
    pub fn new() -> Self;
    pub fn register(&mut self, name: impl Into<String>, adapter: Arc<dyn FormatAdapter>);
    pub fn get(&self, name: &str) -> Option<&dyn FormatAdapter>;
    pub fn names(&self) -> Vec<&str>;
}
```

Adapters are registered by name. The same adapter implementation can be registered under multiple names (e.g., `OpenAIAdapter` registered as both `"openai"` and `"vllm"`).

`detect_format()` inspects the request URL path to determine which adapter to use:

```rust
pub fn detect_format(path: &str) -> Option<&'static str> {
    if path.contains("/v1/chat/completions")
        || path.contains("/v1/completions")
        || path.contains("/chat/completions")
    {
        return Some("openai");
    }
    if path.contains("/v1/messages") {
        return Some("anthropic");
    }
    if path.contains("/api/chat") || path.contains("/api/generate") {
        return Some("ollama");
    }
    None
}
```

The function uses substring matching rather than exact prefix matching, so paths like `/openai/deployments/gpt4o/chat/completions` (Azure) correctly match `"openai"`.

## Per-Adapter Details

### OpenAI (OpenAIAdapter)

- **Name:** `"openai"`
- **Request path:** `/v1/chat/completions`
- **Request format:** Messages array with `role` (string), `content` (string or null), optional `tool_calls`, `tool_call_id`, `name`.
- **Response format:** `id`, `object`, `model`, `created`, `choices[]` with `message` and `finish_reason`, optional `usage`.
- **Streaming:** SSE lines with `data: ` prefix and `[DONE]` terminator. Stream chunk uses `delta` (not `message`) with optional `role`, `content`, `tool_calls`.
- **Error format:** `{"error": {"message": "...", "type": "invalid_request_error", "code": 500}}`.
- **Null content handling:** The adapter's `OpenAIMessage.content` field is `Option<serde_json::Value>`. When null, the adapter maps it to `AIContent::None` via a custom deserialization function.
- **vLLM/OpenAI-compat:** vLLM and any other OpenAI-compatible endpoint use the same `OpenAIAdapter` struct. The adapter is registered under a separate name (e.g., `"vllm"`) in the `AdapterRegistry` for backend routing differentiation, but the parsing/serialization logic is identical since the JSON API is the same.

### Anthropic (AnthropicAdapter)

- **Name:** `"anthropic"`
- **Request path:** `/v1/messages`
- **Request format:** Top-level `model`, `messages[]`, `max_tokens` (required), optional `system` (text string or array of content blocks), `stop_sequences`, `stream`, `temperature`, `top_p`.
- **Key conversion:** The Anthropic `system` field is a top-level request parameter, not a message. The adapter converts it into an `AIMessage` with `AIRole::System` prepended to the messages list before building the `AIRequest`.
- **Content blocks:** Messages use content blocks with `type` and `text` fields. Single text blocks become `AIContent::Text`. Multiple blocks are joined with newlines into `AIContent::Text`. Empty blocks map to `AIContent::None`.
- **Response format:** `id`, `type` ("message"), `role`, `model`, `content[]` (blocks), `stop_reason`, `usage` with `input_tokens` and `output_tokens`.
- **Streaming:** SSE events with `event: content_block_delta` and `event: message_delta`. Chunks carry `delta` with `type` and `text`.
- **Error format:** `{"error": {"type": "error", "message": "..."}}`.

### Ollama (OllamaAdapter)

- **Name:** `"ollama"`
- **Request path:** `/api/chat`, `/api/generate`
- **Request format:** `model`, `messages[]` with `role` (string) and `content` (string), optional `stream`, `options` object with `temperature`, `top_p`, `num_predict`, `stop`.
- **Response format:** `model`, `created_at`, `message` (role + content), `done` (boolean), `total_duration`, `eval_count`, `prompt_eval_count`.
- **Usage mapping:** Ollama doesn't report prompt/completion/total tokens directly. The adapter maps `prompt_eval_count` to `prompt_tokens`, `eval_count` to `completion_tokens`, and computes `total_tokens = prompt_eval_count + eval_count`.
- **Streaming:** Newline-delimited JSON objects (one per line, no SSE wrapper). Each chunk has `message`, `done`, and optional usage fields on the final chunk.
- **Error format:** `{"error": "..."}`.
- **Ollama adapter limitations:** `OllamaAdapter` doesn't support tool calls, image content, or system `name` fields. All messages are treated as simple text-only role/content pairs.

## AIContent::None for Tool Calls

When an assistant message contains tool calls but no textual content, the raw JSON has `"content": null`. The `#[serde(untagged)]` enum `AIContent` cannot directly deserialize `null` because the `None` variant has no data. A custom deserializer handles this:

```rust
fn deserialize_null_as_none<'de, D>(deserializer: D) -> Result<(), D::Error>
where
    D: serde::Deserializer<'de>,
{
    struct NullVisitor;
    impl de::Visitor<'_> for NullVisitor {
        type Value = ();
        fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
            f.write_str("null")
        }
        fn visit_unit<E: de::Error>(self) -> Result<Self::Value, E> {
            Ok(())
        }
    }
    deserializer.deserialize_unit(NullVisitor)?;
    Ok(())
}
```

This is attached to the `None` variant via `#[serde(deserialize_with = "deserialize_null_as_none")]`. During serialization, `None` produces no output, which is the correct behavior for an `#[serde(untagged)]` enum with a unit variant.

## Adding a New Format Adapter

To add support for a new provider format (e.g., Google AI, Mistral, Groq):

1. Create a new file in `dataplane/crates/aeg-ai/src/format/` (e.g., `gemini.rs`).
2. Define provider-specific request/response structs with serde derives.
3. Implement `From<ProviderRequest> for AIRequest` and `From<ProviderResponse> for AIResponse`.
4. Implement `FormatAdapter` for a unit struct (e.g., `GeminiAdapter`).
5. Add `pub mod gemini;` to `format/mod.rs`.
6. Register the adapter in the `AdapterRegistry` for each applicable path pattern.
7. If the provider uses a new path pattern, add a match arm to `detect_format()`.
8. Write round-trip tests: request -> parse -> AIRequest -> check fields; AIResponse -> serialize -> check output matches expected format.

For OpenAI-compatible providers (vLLM, Mistral, Groq, DeepSeek, Azure OpenAI), no new adapter code is needed. Register `OpenAIAdapter` under a distinct name.