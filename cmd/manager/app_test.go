package main

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/nantian-gw/gateway/internal/config"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestBuildSchemeIncludesBackendLBPolicy(t *testing.T) {
	cfg := &config.Config{
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: true,
		},
	}
	scheme, err := buildScheme(cfg)
	if err != nil {
		t.Fatalf("buildScheme returned error: %v", err)
	}

	want := schema.GroupVersionKind{
		Group:   backendlbv1alpha2.GroupVersion.Group,
		Version: backendlbv1alpha2.GroupVersion.Version,
		Kind:    "BackendLBPolicy",
	}
	gvks, _, err := scheme.ObjectKinds(&backendlbv1alpha2.BackendLBPolicy{})
	if err != nil {
		t.Fatalf("BackendLBPolicy is not registered in the manager scheme: %v", err)
	}
	for _, gvk := range gvks {
		if gvk == want {
			return
		}
	}

	t.Fatalf("expected BackendLBPolicy GVK %s to be registered, got %v", want, gvks)
}

func TestControlplaneManagerOptionsCachesUnstructuredReads(t *testing.T) {
	opts := controlplaneManagerOptions(
		&config.Config{},
		runtime.NewScheme(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	if opts.Client.Cache == nil {
		t.Fatal("expected manager client cache options to be configured")
	}
	if !opts.Client.Cache.Unstructured {
		t.Fatal("expected manager client to read unstructured resources through the cache")
	}
}

func TestNewMetricsServerAppliesRuntimeTimeouts(t *testing.T) {
	server := newMetricsServer(":18082", observability.NewMetrics(), "", "", nil)

	if server.Addr != ":18082" {
		t.Fatalf("metrics server addr = %q, want :18082", server.Addr)
	}
	if server.Handler == nil {
		t.Fatal("expected metrics server handler")
	}
	if server.ReadHeaderTimeout != defaultMetricsReadHeaderTimeout {
		t.Fatalf("metrics read header timeout = %s, want %s", server.ReadHeaderTimeout, defaultMetricsReadHeaderTimeout)
	}
	if server.ReadTimeout != defaultMetricsReadTimeout {
		t.Fatalf("metrics read timeout = %s, want %s", server.ReadTimeout, defaultMetricsReadTimeout)
	}
	if server.WriteTimeout != defaultMetricsWriteTimeout {
		t.Fatalf("metrics write timeout = %s, want %s", server.WriteTimeout, defaultMetricsWriteTimeout)
	}
	if server.IdleTimeout != defaultMetricsIdleTimeout {
		t.Fatalf("metrics idle timeout = %s, want %s", server.IdleTimeout, defaultMetricsIdleTimeout)
	}
	if server.MaxHeaderBytes != defaultMetricsMaxHeaderBytes {
		t.Fatalf("metrics max header bytes = %d, want %d", server.MaxHeaderBytes, defaultMetricsMaxHeaderBytes)
	}
}

func TestControlplaneTracingConfigUsesNormalizedConfigValues(t *testing.T) {
	ratio := 0.35
	cfg := &config.Config{
		Tracing: config.TracingConfig{
			Enabled:      true,
			Endpoint:     " otel-collector:4317 ",
			Insecure:     true,
			SamplerRatio: &ratio,
			Headers: map[string]string{
				" authorization ": " Bearer token ",
			},
		},
	}

	got := controlplaneTracingConfig(cfg)
	if !got.Enabled {
		t.Fatal("expected tracing to stay enabled")
	}
	if got.Endpoint != "otel-collector:4317" {
		t.Fatalf("unexpected tracing endpoint: %q", got.Endpoint)
	}
	if !got.Insecure {
		t.Fatal("expected insecure tracing transport to be preserved")
	}
	if got.SamplerRatio != 0.35 {
		t.Fatalf("unexpected tracing sampler ratio: %v", got.SamplerRatio)
	}
	if got.Headers["authorization"] != "Bearer token" {
		t.Fatalf("unexpected tracing headers: %#v", got.Headers)
	}
}

func TestLogControlplaneTracingStatusRedactsHeaderValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logControlplaneTracingStatus(logger, observability.TracingConfig{
		Enabled:      true,
		Endpoint:     "otel-collector:4317",
		Insecure:     true,
		SamplerRatio: 0.25,
		Headers: map[string]string{
			"authorization": "Bearer secret-token",
		},
	})

	output := buf.String()
	if !strings.Contains(output, "configured controlplane tracing") {
		t.Fatalf("expected tracing log message, got %q", output)
	}
	if !strings.Contains(output, "header_count=1") {
		t.Fatalf("expected tracing header count in log output, got %q", output)
	}
	if strings.Contains(output, "secret-token") {
		t.Fatalf("expected tracing log to redact header values, got %q", output)
	}
}

func TestBuildSchemeFeaturesDisabled(t *testing.T) {
	cfg := &config.Config{
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: false,
			EnableAiGateway:           false,
		},
	}
	scheme, err := buildScheme(cfg)
	if err != nil {
		t.Fatalf("buildScheme returned error: %v", err)
	}

	// BackendLBPolicy should NOT be registered when feature is disabled
	_, _, err = scheme.ObjectKinds(&backendlbv1alpha2.BackendLBPolicy{})
	if err == nil {
		t.Fatal("BackendLBPolicy should not be registered when enableExperimentalGateway is false")
	}
}

func TestBuildSchemeFeatureFlagsGated(t *testing.T) {
	// Only enableExperimentalGateway, not enableAiGateway
	cfg := &config.Config{
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: true,
			EnableAiGateway:           false,
		},
	}
	scheme, err := buildScheme(cfg)
	if err != nil {
		t.Fatalf("buildScheme returned error: %v", err)
	}

	// BackendLBPolicy should be registered
	_, _, err = scheme.ObjectKinds(&backendlbv1alpha2.BackendLBPolicy{})
	if err != nil {
		t.Fatalf("BackendLBPolicy should be registered: %v", err)
	}
}
