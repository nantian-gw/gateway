package chatbot

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	return s
}

func TestLoadConfig_SecretNotFound(t *testing.T) {
	t.Parallel()

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	_, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err == nil {
		t.Fatal("expected error when secret is missing, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestLoadConfig_NamespaceFallback(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
		Data: map[string][]byte{
			"provider":     []byte("openai"),
			"api-endpoint": []byte("https://api.openai.com/v1"),
			"api-key":      []byte("sk-test-key"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()
	cfg, err := LoadConfig(context.Background(), k8sClient, "", nil)
	if err != nil {
		t.Fatalf("unexpected error with empty namespace (should fallback): %v", err)
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test-key")
	}
}

func TestLoadConfig_ValidSecret(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
		Data: map[string][]byte{
			"provider":     []byte("openai"),
			"api-endpoint": []byte("https://custom.api.com/v1"),
			"api-key":      []byte("sk-test-key"),
			"model":        []byte("gpt-4o-mini"),
			"temperature":  []byte("0.3"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()
	cfg, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "openai")
	}
	if cfg.APIEndpoint != "https://custom.api.com/v1" {
		t.Errorf("APIEndpoint = %q, want %q", cfg.APIEndpoint, "https://custom.api.com/v1")
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test-key")
	}
	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o-mini")
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("Temperature = %v, want 0.3", cfg.Temperature)
	}
}

func TestLoadConfig_DefaultModelAndTemperature(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
		Data: map[string][]byte{
			"provider":     []byte("deepseek"),
			"api-endpoint": []byte("https://api.deepseek.com/v1"),
			"api-key":      []byte("sk-deepseek-key"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()
	cfg, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Temperature != defaultTemperature {
		t.Errorf("Temperature = %v, want %v (default)", cfg.Temperature, defaultTemperature)
	}
	if cfg.Model != defaultModel {
		t.Errorf("Model = %q, want %q (default)", cfg.Model, defaultModel)
	}
}

func TestLoadConfig_EmptySecretData(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()
	_, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err == nil {
		t.Fatal("expected error for secret with nil data, got nil")
	}
	if !strings.Contains(err.Error(), "no data") {
		t.Errorf("expected 'no data' in error, got: %v", err)
	}
}

func TestLoadConfig_InvalidTemperatureFallsBack(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
		Data: map[string][]byte{
			"provider":     []byte("openai"),
			"api-endpoint": []byte("https://api.openai.com/v1"),
			"api-key":      []byte("sk-test-key"),
			"temperature":  []byte("not-a-float"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()
	cfg, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err != nil {
		t.Fatalf("expected no error (invalid temp should fall back to default), got: %v", err)
	}
	if cfg.Temperature != defaultTemperature {
		t.Errorf("Temperature = %v, want %v (fallback default)", cfg.Temperature, defaultTemperature)
	}
}

func TestLoadConfig_CustomNamespace(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "gateway-system",
		},
		Data: map[string][]byte{
			"provider":     []byte("ollama"),
			"api-endpoint": []byte("http://localhost:11434/v1"),
			"api-key":      []byte("sk-ns-key"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(secret).Build()

	_, err := LoadConfig(context.Background(), k8sClient, "nantian-gw", nil)
	if err == nil {
		t.Fatal("expected error when secret is in different namespace")
	}

	cfg, err := LoadConfig(context.Background(), k8sClient, "gateway-system", nil)
	if err != nil {
		t.Fatalf("unexpected error for correct namespace: %v", err)
	}
	if cfg.APIKey != "sk-ns-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-ns-key")
	}
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "ollama")
	}
}

func TestChatbotConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *ChatbotConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     &ChatbotConfig{Provider: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-test", Model: "gpt-4o", Temperature: 0.1},
			wantErr: false,
		},
		{
			name:    "missing provider",
			cfg:     &ChatbotConfig{APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-test", Model: "gpt-4o"},
			wantErr: true,
		},
		{
			name:    "missing endpoint",
			cfg:     &ChatbotConfig{Provider: "openai", APIKey: "sk-test", Model: "gpt-4o"},
			wantErr: true,
		},
		{
			name:    "missing API key",
			cfg:     &ChatbotConfig{Provider: "openai", APIEndpoint: "https://api.openai.com/v1", Model: "gpt-4o"},
			wantErr: true,
		},
		{
			name:    "missing model",
			cfg:     &ChatbotConfig{Provider: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-test"},
			wantErr: true,
		},
		{
			name:    "temperature below zero",
			cfg:     &ChatbotConfig{Provider: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-test", Model: "gpt-4o", Temperature: -0.1},
			wantErr: true,
		},
		{
			name:    "temperature above two",
			cfg:     &ChatbotConfig{Provider: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-test", Model: "gpt-4o", Temperature: 2.1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
