package translator

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nantian-gw/gateway/internal/gatewayapiexperimental/aiservicev1alpha1"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestTranslateAIService_Basic(t *testing.T) {
	svc := aiservicev1alpha1.AIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-gateway",
			Namespace: "default",
		},
		Spec: aiservicev1alpha1.AIServiceSpec{
			Provider: "openai",
			Model:    "gpt-4",
		},
	}
	result := translateAIService(svc)
	if result.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", result.Provider)
	}
	if result.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", result.Model)
	}
	if result.Format != "" {
		t.Errorf("expected empty format, got %s", result.Format)
	}
	if result.Timeout != 0 {
		t.Errorf("expected zero timeout, got %v", result.Timeout)
	}
}

func TestTranslateAIService_WithFormat(t *testing.T) {
	svc := aiservicev1alpha1.AIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "azure-gateway",
			Namespace: "default",
		},
		Spec: aiservicev1alpha1.AIServiceSpec{
			Provider: "azure",
			Format:   "openai",
			Model:    "gpt-4-turbo",
			Auth: aiservicev1alpha1.AIServiceAuth{
				Type:   "api_key",
				Secret: "azure-key",
				Header: "api-key",
			},
			Timeout: "30s",
		},
	}
	result := translateAIService(svc)
	if result.Format != "openai" {
		t.Errorf("expected format openai, got %s", result.Format)
	}
	if result.Auth.Type != "api_key" {
		t.Errorf("expected auth type api_key, got %s", result.Auth.Type)
	}
	if result.Auth.SecretRef != "default/azure-key" {
		t.Errorf("expected secret ref default/azure-key, got %s", result.Auth.SecretRef)
	}
	if result.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", result.Timeout)
	}
}

func TestTranslateAIServiceList(t *testing.T) {
	svcs := []aiservicev1alpha1.AIService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-a",
				Namespace: "ns1",
			},
			Spec: aiservicev1alpha1.AIServiceSpec{
				Provider: "openai",
				Model:    "gpt-4",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-b",
				Namespace: "ns2",
			},
			Spec: aiservicev1alpha1.AIServiceSpec{
				Provider: "anthropic",
				Model:    "claude-3-opus",
			},
		},
	}
	result := translateAIServices(svcs)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if cfg, ok := result["ns1/svc-a"]; !ok {
		t.Errorf("expected ns1/svc-a in results")
	} else if cfg.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", cfg.Provider)
	}
	if cfg, ok := result["ns2/svc-b"]; !ok {
		t.Errorf("expected ns2/svc-b in results")
	} else if cfg.Model != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %s", cfg.Model)
	}
}

func TestTranslateAIService_TimeoutParse(t *testing.T) {
	svc := aiservicev1alpha1.AIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timed-svc",
			Namespace: "default",
		},
		Spec: aiservicev1alpha1.AIServiceSpec{
			Provider: "openai",
			Model:    "gpt-4",
			Timeout:  "invalid",
		},
	}
	result := translateAIService(svc)
	if result.Timeout != 0 {
		t.Errorf("expected zero timeout for invalid duration, got %v", result.Timeout)
	}
}

// Ensure interfaces compile
var _ ir.AIServiceConfig = translateAIService(aiservicev1alpha1.AIService{})