// Package chatbot provides an LLM-powered assistant for Gateway API resources.
package chatbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultChatbotConfigNamespace = "aether-gateway"
	defaultChatbotConfigSecret    = "chatbot-config"

	defaultTemperature = 0.1
	defaultModelOpenAI = "gpt-4o"
	defaultModel       = defaultModelOpenAI
)

// ChatbotConfig holds the LLM provider configuration loaded from a Kubernetes Secret.
type ChatbotConfig struct {
	Provider    string  `json:"provider"`
	APIEndpoint string  `json:"apiEndpoint"`
	APIKey      string  `json:"apiKey"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

// Validate returns an error when required fields are missing or invalid.
func (c *ChatbotConfig) Validate() error {
	if c.Provider == "" {
		return errors.New("chatbot config: provider is required")
	}
	if c.APIEndpoint == "" {
		return errors.New("chatbot config: apiEndpoint is required")
	}
	if c.APIKey == "" {
		return errors.New("chatbot config: apiKey is required")
	}
	if c.Model == "" {
		return errors.New("chatbot config: model is required")
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("chatbot config: temperature %.2f is out of range [0, 2]", c.Temperature)
	}
	return nil
}

// LoadConfig reads the chatbot configuration from the Kubernetes Secret
// "chatbot-config" in the specified namespace. It falls back to
// defaultChatbotConfigNamespace when namespace is empty.
//
// Required Secret keys:
//
//	provider     - LLM provider name (e.g. "openai", "deepseek", "ollama")
//	api-endpoint - Base URL for the LLM API
//	api-key      - Authentication key
//
// Optional keys (defaults apply when absent):
//
//	model       - Model name (default: "gpt-4o")
//	temperature - Sampling temperature (default: 0.1)
func LoadConfig(ctx context.Context, cl client.Client, namespace string) (*ChatbotConfig, error) {
	if namespace == "" {
		namespace = defaultChatbotConfigNamespace
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      defaultChatbotConfigSecret,
	}

	if err := cl.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("chatbot config: failed to read Secret %s/%s: %w", namespace, defaultChatbotConfigSecret, err)
	}

	if secret.Data == nil {
		return nil, fmt.Errorf("chatbot config: Secret %s/%s has no data", namespace, defaultChatbotConfigSecret)
	}

	cfg := &ChatbotConfig{}

	if v, ok := secret.Data["provider"]; ok {
		cfg.Provider = string(v)
	}
	if v, ok := secret.Data["api-endpoint"]; ok {
		cfg.APIEndpoint = string(v)
	}
	if v, ok := secret.Data["api-key"]; ok {
		cfg.APIKey = string(v)
	}

	// Model – default to "gpt-4o" when empty.
	if v, ok := secret.Data["model"]; ok && len(v) > 0 {
		cfg.Model = string(v)
	} else {
		cfg.Model = defaultModel
		slog.Debug("chatbot config: model not set, defaulting", "model", cfg.Model)
	}

	// Temperature – default to 0.1 when absent or unparseable.
	if v, ok := secret.Data["temperature"]; ok && len(v) > 0 {
		t, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			slog.Warn("chatbot config: invalid temperature, defaulting",
				"raw", string(v),
				"error", err,
				"temperature", defaultTemperature,
			)
			cfg.Temperature = defaultTemperature
		} else {
			cfg.Temperature = t
		}
	} else {
		cfg.Temperature = defaultTemperature
	}

	return cfg, nil
}