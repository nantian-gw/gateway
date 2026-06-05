package metrics

import (
	"context"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMetricsConfigNamespace = "aether-gateway"
	defaultMetricsConfigSecret    = "aether-gateway-metrics-config"
)

// MetricsConfig holds the Prometheus integration configuration loaded from a
// Kubernetes Secret.
type MetricsConfig struct {
	PrometheusURL string `json:"prometheusUrl"`
}

// Validate returns an error when required fields are missing or invalid.
func (c *MetricsConfig) Validate() error {
	if c.PrometheusURL == "" {
		return fmt.Errorf("metrics config: prometheusUrl is required")
	}
	u, err := url.Parse(c.PrometheusURL)
	if err != nil {
		return fmt.Errorf("metrics config: invalid prometheusUrl: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("metrics config: prometheusUrl scheme must be http or https, got %q", u.Scheme)
	}
	return nil
}
// "aether-gateway-metrics-config" in the specified namespace.
func LoadConfig(ctx context.Context, cl client.Client, namespace string) (*MetricsConfig, error) {
	if namespace == "" {
		namespace = defaultMetricsConfigNamespace
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      defaultMetricsConfigSecret,
	}

	if err := cl.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("metrics config: failed to read Secret %s/%s: %w", namespace, defaultMetricsConfigSecret, err)
	}

	if secret.Data == nil {
		return nil, fmt.Errorf("metrics config: Secret %s/%s has no data", namespace, defaultMetricsConfigSecret)
	}

	cfg := &MetricsConfig{}

	if v, ok := secret.Data["prometheus-url"]; ok {
		cfg.PrometheusURL = string(v)
	}

	return cfg, nil
}

// SaveConfig creates or updates the Kubernetes Secret with the given
// configuration.
func SaveConfig(ctx context.Context, cl client.Client, namespace string, cfg *MetricsConfig) error {
	if namespace == "" {
		namespace = defaultMetricsConfigNamespace
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: namespace, Name: defaultMetricsConfigSecret}

	if err := cl.Get(ctx, key, secret); err == nil {
		secret.Data["prometheus-url"] = []byte(cfg.PrometheusURL)
		if err := cl.Update(ctx, secret); err != nil {
			return fmt.Errorf("metrics config: failed to update Secret %s/%s: %w", namespace, defaultMetricsConfigSecret, err)
		}
		return nil
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultMetricsConfigSecret,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"prometheus-url": []byte(cfg.PrometheusURL),
		},
	}
	if err := cl.Create(ctx, secret); err != nil {
		return fmt.Errorf("metrics config: failed to create Secret %s/%s: %w", namespace, defaultMetricsConfigSecret, err)
	}
	return nil
}