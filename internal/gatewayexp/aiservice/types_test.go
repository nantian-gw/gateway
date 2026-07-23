package aiservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAIServiceDeepCopy_Nil(t *testing.T) {
	var svc *AIService
	assert.Nil(t, svc.DeepCopy())
	assert.Nil(t, svc.DeepCopyObject())
}

func TestDeepCopyRoundtrip(t *testing.T) {
	original := &AIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ai",
			Namespace: "default",
		},
		Spec: AIServiceSpec{
			Provider: "openai",
			Format:   "openai",
			Model:    "gpt-4",
			Auth: AIServiceAuth{
				Type:   "apiKey",
				Secret: "my-secret",
				Header: "Authorization",
			},
			Timeout: "30s",
			Retry: AIRetryConfig{
				MaxRetries: 3,
				Backoff:    "exponential",
			},
		},
	}
	copied := original.DeepCopy()
	assert.Equal(t, original, copied)
	assert.NotSame(t, original, copied)
}

func TestAIServiceDeepCopy_WithAuth(t *testing.T) {
	svc := &AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "openai-gpt4", Namespace: "default"},
		Spec: AIServiceSpec{
			Provider: "openai",
			Format:   "openai",
			Model:    "gpt-4o",
			Auth: AIServiceAuth{
				Type:   "apiKey",
				Secret: "openai-secret",
				Key:    "Authorization",
				Header: "api.openai.com",
			},
			Timeout: "30s",
		},
	}
	copied := svc.DeepCopy()
	assert.Equal(t, svc.Spec.Auth.Type, copied.Spec.Auth.Type)
	assert.Equal(t, svc.Spec.Provider, copied.Spec.Provider)
	assert.NotSame(t, svc, copied)
}

func TestAIServiceDeepCopy_WithObservability(t *testing.T) {
	svc := &AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "with-obs", Namespace: "default"},
		Spec: AIServiceSpec{
			Provider: "anthropic",
			Model:    "claude-3",
			Observability: AIObservabilityConfig{
				Langfuse: LangfuseConfig{
					Host:      "https://cloud.langfuse.com",
					PublicKey: "pk-xxx",
					SecretKey: "sk-xxx",
				},
				OTel: OTelConfig{
					Endpoint:    "http://otel-collector:4317",
					ServiceName: "ai-gateway",
				},
			},
		},
	}
	copied := svc.DeepCopy()
	assert.Equal(t, svc.Spec.Observability.Langfuse.Host, copied.Spec.Observability.Langfuse.Host)
	assert.Equal(t, svc.Spec.Observability.OTel.Endpoint, copied.Spec.Observability.OTel.Endpoint)
}

func TestAIServiceDeepCopy_WithRetry(t *testing.T) {
	svc := &AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "with-retry", Namespace: "default"},
		Spec: AIServiceSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Retry: AIRetryConfig{
				MaxRetries: 3,
				Backoff:    "exponential",
			},
		},
	}
	copied := svc.DeepCopy()
	assert.Equal(t, uint32(3), copied.Spec.Retry.MaxRetries)
}

func TestAIServiceListDeepCopy(t *testing.T) {
	list := &AIServiceList{
		Items: []AIService{
			{ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns1"}, Spec: AIServiceSpec{Provider: "openai", Model: "gpt-4o"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "svc2", Namespace: "ns2"}, Spec: AIServiceSpec{Provider: "anthropic", Model: "claude-3"}},
		},
	}
	copied := list.DeepCopy()
	assert.Equal(t, 2, len(copied.Items))
	assert.Equal(t, list.Items[0].Name, copied.Items[0].Name)
}

func TestAIServiceListDeepCopy_Nil(t *testing.T) {
	var list *AIServiceList
	assert.Nil(t, list.DeepCopy())
	assert.Nil(t, list.DeepCopyObject())
}

func TestAIServiceDeepCopyInto(t *testing.T) {
	src := &AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "src", Namespace: "ns"},
		Spec: AIServiceSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Auth:     AIServiceAuth{Type: "apiKey", Secret: "s", Key: "k", Header: "h"},
			Timeout:  "60s",
			Retry:    AIRetryConfig{MaxRetries: 5, Backoff: "linear"},
		},
	}
	dst := &AIService{}
	src.DeepCopyInto(dst)
	assert.Equal(t, "src", dst.Name)
	assert.Equal(t, "openai", dst.Spec.Provider)
	assert.Equal(t, uint32(5), dst.Spec.Retry.MaxRetries)
}

func TestAIServiceSpecDeepCopy(t *testing.T) {
	spec := &AIServiceSpec{
		Provider: "anthropic",
		Format:   "anthropic",
		Model:    "claude-3",
		Auth:     AIServiceAuth{Type: "apiKey", Secret: "sec"},
		Timeout:  "30s",
		Retry:    AIRetryConfig{MaxRetries: 3, Backoff: "exponential"},
		Observability: AIObservabilityConfig{
			Langfuse: LangfuseConfig{Host: "host", PublicKey: "pk", SecretKey: "sk"},
			OTel:     OTelConfig{Endpoint: "ep", ServiceName: "sn"},
		},
	}
	copied := spec.DeepCopy()
	assert.Equal(t, spec.Provider, copied.Provider)
	assert.Equal(t, spec.Observability.Langfuse.Host, copied.Observability.Langfuse.Host)
}

func TestAIServiceSpecDeepCopy_Nil(t *testing.T) {
	var spec *AIServiceSpec
	assert.Nil(t, spec.DeepCopy())
}

func TestAIServiceStatusDeepCopy(t *testing.T) {
	status := &AIServiceStatus{
		Conditions: []metav1.Condition{
			{Type: "Accepted", Status: "True", Reason: "Valid", Message: "ok"},
		},
	}
	copied := status.DeepCopy()
	assert.Equal(t, 1, len(copied.Conditions))
	assert.Equal(t, "Accepted", copied.Conditions[0].Type)
	assert.NotSame(t, &status.Conditions, &copied.Conditions)
}

func TestAIServiceStatusDeepCopy_Nil(t *testing.T) {
	var status *AIServiceStatus
	assert.Nil(t, status.DeepCopy())
}
