package main

import (
	"io"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/config"
	backendlbv1alpha2 "github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/observability"
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
	server := newMetricsServer(":18082", observability.NewMetrics())

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
