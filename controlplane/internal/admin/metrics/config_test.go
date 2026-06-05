package metrics

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func TestLoadConfig(t *testing.T) {
	ctx := context.Background()
	ns := "aether-gateway"

	t.Run("success", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultMetricsConfigSecret,
					Namespace: ns,
				},
				Data: map[string][]byte{
					"prometheus-url": []byte("http://prometheus:9090"),
				},
			}).
			Build()

		cfg, err := LoadConfig(ctx, cl, ns)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if cfg.PrometheusURL != "http://prometheus:9090" {
			t.Errorf("expected http://prometheus:9090, got %s", cfg.PrometheusURL)
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			Build()

		_, err := LoadConfig(ctx, cl, ns)
		if err == nil {
			t.Fatal("expected error for missing Secret")
		}
	})

	t.Run("default namespace", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultMetricsConfigSecret,
					Namespace: defaultMetricsConfigNamespace,
				},
				Data: map[string][]byte{
					"prometheus-url": []byte("http://prometheus:9090"),
				},
			}).
			Build()

		cfg, err := LoadConfig(ctx, cl, "")
		if err != nil {
			t.Fatalf("expected no error with default namespace, got: %v", err)
		}
		if cfg.PrometheusURL != "http://prometheus:9090" {
			t.Errorf("expected http://prometheus:9090, got %s", cfg.PrometheusURL)
		}
	})
}

func TestSaveConfig(t *testing.T) {
	ctx := context.Background()
	ns := "aether-gateway"

	t.Run("create new secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			Build()

		cfg := &MetricsConfig{PrometheusURL: "http://prometheus:9090"}
		if err := SaveConfig(ctx, cl, ns, cfg); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		loaded, err := LoadConfig(ctx, cl, ns)
		if err != nil {
			t.Fatalf("expected loaded config to exist, got: %v", err)
		}
		if loaded.PrometheusURL != "http://prometheus:9090" {
			t.Errorf("expected http://prometheus:9090, got %s", loaded.PrometheusURL)
		}
	})

	t.Run("update existing secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultMetricsConfigSecret,
					Namespace: ns,
				},
				Data: map[string][]byte{
					"prometheus-url": []byte("http://old:9090"),
				},
			}).
			Build()

		cfg := &MetricsConfig{PrometheusURL: "http://new:9090"}
		if err := SaveConfig(ctx, cl, ns, cfg); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		loaded, err := LoadConfig(ctx, cl, ns)
		if err != nil {
			t.Fatalf("expected loaded config to exist, got: %v", err)
		}
		if loaded.PrometheusURL != "http://new:9090" {
			t.Errorf("expected http://new:9090, got %s", loaded.PrometheusURL)
		}
	})

	t.Run("reject empty url", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			Build()

		cfg := &MetricsConfig{PrometheusURL: ""}
		if err := SaveConfig(ctx, cl, ns, cfg); err == nil {
			t.Fatal("expected error for empty URL")
		}
	})

	t.Run("reject invalid scheme", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(testScheme()).
			Build()

		cfg := &MetricsConfig{PrometheusURL: "ftp://prometheus:9090"}
		if err := SaveConfig(ctx, cl, ns, cfg); err == nil {
			t.Fatal("expected error for invalid scheme")
		}
	})
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://prometheus:9090", false},
		{"valid https", "https://prometheus.monitoring:9090", false},
		{"valid with path", "http://prometheus:9090/prometheus", false},
		{"empty", "", true},
		{"missing scheme", "prometheus:9090", true},
		{"ftp scheme", "ftp://prometheus:9090", true},
		{"invalid url", "://bad", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MetricsConfig{PrometheusURL: tt.url}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}