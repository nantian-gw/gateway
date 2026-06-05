# AIService CRD Reference

- **API Group:** `gateway.nantian.dev`
- **API Version:** `v1alpha1`
- **Kind:** `AIService`
- **Scope:** `Namespaced`
- **Package:** `controlplane/internal/gatewayapiexperimental/aiservicev1alpha1/types.go`

The `AIService` CRD declares an AI provider backend for the AI Gateway. It is referenced from `HTTPRoute` `backendRefs` just like a `Service`, but with `group: gateway.nantian.dev` and `kind: AIService`.

## Full YAML Example

```yaml
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIService
metadata:
  name: gpt4o
  namespace: ai-prod
spec:
  provider: openai
  format: openai
  model: gpt-4o
  auth:
    type: Bearer
    secret: openai-api-key
    key: api-key
    header: Authorization
  timeout: 60s
  retry:
    maxRetries: 3
    backoff: exponential
  observability:
    langfuse:
      host: https://cloud.langfuse.com
      publicKey: pk-lf-xxxxx
      secretKey: sk-lf-xxxxx
    otel:
      endpoint: http://otel-collector.observability:4317
      serviceName: ai-gateway
status:
  conditions:
    - type: Accepted
      status: "True"
      reason: Accepted
      message: AI service is valid
      lastTransitionTime: "2026-05-30T00:00:00Z"
    - type: ResolvedRefs
      status: "True"
      reason: ResolvedRefs
      message: All references resolved
      lastTransitionTime: "2026-05-30T00:00:00Z"
```

## Spec Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `provider` | string | Yes | Provider name: `openai`, `anthropic`, `ollama`, `vllm`, etc. Used for backend routing and identification. |
| `format` | string | No | API format: `openai`, `anthropic`, `ollama`, `openai-compat`. Defaults to the provider value. Controls which `FormatAdapter` is used for request/response conversion. |
| `model` | string | Yes | Model name passed to the provider (e.g., `gpt-4o`, `claude-3-opus-20240229`, `llama3`). |
| `auth` | AIServiceAuth | No | Authentication configuration for the provider API. |
| `timeout` | string | No | Request timeout as a Go duration string (e.g., `60s`, `5m`). Parsed by `time.ParseDuration()`. If unset or unparseable, the default backend timeout is used. |
| `retry` | AIRetryConfig | No | Retry policy for failed requests. |
| `observability` | AIObservabilityConfig | No | Observability integration settings (Langfuse + OpenTelemetry). |

### AIServiceAuth

| Field | Type | Description |
|---|---|---|
| `type` | string | Auth type: `Bearer`, `Basic`, `ApiKey`, `Custom`. |
| `secret` | string | Name of a Kubernetes Secret in the same namespace containing the credential. |
| `key` | string | Key within the Secret data. |
| `header` | string | HTTP header to inject the credential into (e.g., `Authorization`, `x-api-key`). |

### AIRetryConfig

| Field | Type | Description |
|---|---|---|
| `maxRetries` | uint32 | Maximum retry attempts. |
| `backoff` | string | Backoff strategy: `exponential`, `linear`, `fixed`. |

### AIObservabilityConfig

| Field | Type | Description |
|---|---|---|
| `langfuse` | LangfuseConfig | Langfuse integration configuration. |
| `otel` | OTelConfig | OpenTelemetry export configuration. |

### LangfuseConfig

| Field | Type | Description |
|---|---|---|
| `host` | string | Langfuse server URL (e.g., `https://cloud.langfuse.com`). |
| `publicKey` | string | Langfuse public key for authentication. |
| `secretKey` | string | Langfuse secret key for authentication. |

### OTelConfig

| Field | Type | Description |
|---|---|---|
| `endpoint` | string | OTLP gRPC collector endpoint (e.g., `http://otel-collector:4317`). |
| `serviceName` | string | Service name included in the OTel resource. |

## Status Conditions

The status subresource follows Gateway API conventions. Two conditions are set:

- `Accepted` - The AIService resource is syntactically valid and the translator recognized it.
- `ResolvedRefs` - All referenced secrets exist and are readable.

Both conditions use `metav1.Condition` with standard fields: `type`, `status`, `reason`, `message`, `observedGeneration`, `lastTransitionTime`.

## Go Type Definitions

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type AIService struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              AIServiceSpec   `json:"spec,omitempty"`
    Status            AIServiceStatus `json:"status,omitempty"`
}

type AIServiceSpec struct {
    Provider      string                `json:"provider"`
    Format        string                `json:"format,omitempty"`
    Model         string                `json:"model"`
    Auth          AIServiceAuth         `json:"auth,omitempty"`
    Timeout       string                `json:"timeout,omitempty"`
    Retry         AIRetryConfig         `json:"retry,omitempty"`
    Observability AIObservabilityConfig `json:"observability,omitempty"`
}

type AIServiceStatus struct {
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Scheme registration:
```go
var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

func AddToScheme(scheme *runtime.Scheme) error {
    scheme.AddKnownTypes(GroupVersion, &AIService{}, &AIServiceList{})
    metav1.AddToGroupVersion(scheme, GroupVersion)
    return nil
}
```

## Translator Behavior

The translator in `controlplane/internal/translator/ai_service.go` converts an `AIService` CRD into the control-plane IR:

```go
func translateAIService(svc aiservicev1alpha1.AIService) ir.AIServiceConfig {
    cfg := ir.AIServiceConfig{
        Provider:  svc.Spec.Provider,
        Format:    svc.Spec.Format,
        Model:     svc.Spec.Model,
        Auth: ir.AIServiceAuth{
            Type:      svc.Spec.Auth.Type,
            SecretRef: svc.Namespace + "/" + svc.Spec.Auth.Secret,
            Header:    svc.Spec.Auth.Header,
        },
    }
    if svc.Spec.Timeout != "" {
        if d, err := time.ParseDuration(svc.Spec.Timeout); err == nil {
            cfg.Timeout = d
        }
    }
    return cfg
}
```

Key behaviors:
- The `Auth.SecretRef` is assembled as `namespace/name` from the CRD's namespace and the secret name.
- The `Timeout` string is parsed with `time.ParseDuration()`. Unparseable values are silently ignored (the timeout field is left zero-valued).
- The translator is called from the main snapshot build flow, and results are stored in `BackendCluster.AIService` as an optional `*AIServiceConfig`.

### IR Types

```go
type BackendCluster struct {
    // ... standard fields ...
    AIService *AIServiceConfig `json:"aiService,omitempty"`
}

type AIServiceConfig struct {
    Provider string        `json:"provider"`
    Format   string        `json:"format,omitempty"`
    Model    string        `json:"model"`
    Auth     AIServiceAuth `json:"auth,omitempty"`
    Timeout  time.Duration `json:"timeout,omitempty"`
}

type AIServiceAuth struct {
    Type      string `json:"type,omitempty"`
    SecretRef string `json:"secretRef,omitempty"`
    Header    string `json:"header,omitempty"`
}
```

## Proto Message

The IR is encoded into the xDS proto snapshot as part of the `BackendCluster` message:

```protobuf
message BackendCluster {
    // ... fields 1-10 ...
    AIServiceConfig ai_service = 11;
}

message AIServiceConfig {
    string provider = 1;
    string format = 2;
    string model = 3;
    AIServiceAuthConfig auth = 4;
    google.protobuf.Duration timeout = 5;
}

message AIServiceAuthConfig {
    string type = 1;
    string secret_ref = 2;
    string header = 3;
}
```

The `Retry` and `Observability` fields from the CRD spec are not currently propagated through the IR or proto. They exist on the CRD type for future use.