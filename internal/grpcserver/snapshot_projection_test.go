package grpcserver

import (
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestProjectedSnapshotLegacyFallbackRemovesUnsupportedHardSemantics(t *testing.T) {
	t.Parallel()

	projected := buildProjectedProtoSnapshot(
		projectionTestSnapshot(),
		effectiveProjectionProfile(nil),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	if got, want := projected.GetCompatibilityProfile(), compatibilityProfileLegacyPreNegotiationV1; got != want {
		t.Fatalf("compatibility profile = %q, want %q", got, want)
	}

	wantFeatures := []string{
		featureCoreV1,
		featureBackendAIServiceV1,
		featureBackendTokenPolicyV1,
		featureBackendWasmPluginV1,
	}
	if !reflect.DeepEqual(projected.GetRequiredFeatures(), wantFeatures) {
		t.Fatalf("required features = %#v, want %#v", projected.GetRequiredFeatures(), wantFeatures)
	}

	if len(projected.GetBackends()) != 4 {
		t.Fatalf("expected 4 backends in legacy projection, got %d", len(projected.GetBackends()))
	}
	if projected.GetHttpRoutes()[0].GetLabels() != nil {
		t.Fatalf("expected legacy projection to strip route labels, got %#v", projected.GetHttpRoutes()[0].GetLabels())
	}
	if projected.GetGrpcRoutes()[0].GetLabels() != nil {
		t.Fatalf("expected legacy projection to strip grpc route labels, got %#v", projected.GetGrpcRoutes()[0].GetLabels())
	}
	if projected.GetStreamRoutes()[0].GetLabels() != nil {
		t.Fatalf("expected legacy projection to strip stream route labels, got %#v", projected.GetStreamRoutes()[0].GetLabels())
	}

	aiBackend := findProjectedBackend(t, projected, "ai-backend")
	if aiBackend.GetAiService() == nil {
		t.Fatal("expected ai service backend to remain in legacy projection")
	}
	tokenBackend := findProjectedBackend(t, projected, "token-backend")
	if tokenBackend.GetTokenPolicy() == nil {
		t.Fatal("expected token policy backend to remain in legacy projection")
	}
	wasmBackend := findProjectedBackend(t, projected, "wasm-backend")
	if wasmBackend.GetWasmPlugin() == nil {
		t.Fatal("expected wasm plugin backend to remain in legacy projection")
	}
}

func TestProjectedSnapshotFullProfilePreservesAIServiceTokenPolicyWasmAndLabels(t *testing.T) {
	t.Parallel()

	projected := buildProjectedProtoSnapshot(
		projectionTestSnapshot(),
		effectiveProjectionProfile([]string{
			featureCoreV1,
			featureRouteLabelsV1,
			featureBackendAIServiceV1,
			featureBackendTokenPolicyV1,
			featureBackendWasmPluginV1,
		}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	wantFeatures := []string{
		featureCoreV1,
		featureRouteLabelsV1,
		featureBackendAIServiceV1,
		featureBackendTokenPolicyV1,
		featureBackendWasmPluginV1,
	}
	if !reflect.DeepEqual(projected.GetRequiredFeatures(), wantFeatures) {
		t.Fatalf("required features = %#v, want %#v", projected.GetRequiredFeatures(), wantFeatures)
	}
	if got, want := projected.GetCompatibilityProfile(), compatibilityProfileFullV1; got != want {
		t.Fatalf("compatibility profile = %q, want %q", got, want)
	}

	httpRoute := findProjectedHTTPRoute(t, projected, "http-labeled")
	if got := httpRoute.GetLabels()["env"]; got != "prod" {
		t.Fatalf("http route labels = %#v, want env=prod", httpRoute.GetLabels())
	}
	grpcRoute := findProjectedGRPCRoute(t, projected, "grpc-token")
	if got := grpcRoute.GetLabels()["tier"]; got != "gold" {
		t.Fatalf("grpc route labels = %#v, want tier=gold", grpcRoute.GetLabels())
	}
	streamRoute := findProjectedStreamRoute(t, projected, "stream-wasm")
	if got := streamRoute.GetLabels()["protocol"]; got != "tls" {
		t.Fatalf("stream route labels = %#v, want protocol=tls", streamRoute.GetLabels())
	}

	aiBackend := findProjectedBackend(t, projected, "ai-backend")
	if aiBackend.GetAiService() == nil || aiBackend.GetAiService().GetModel() != "gpt-4.1-mini" {
		t.Fatalf("ai service backend = %#v, want populated ai service config", aiBackend.GetAiService())
	}
	tokenBackend := findProjectedBackend(t, projected, "token-backend")
	if tokenBackend.GetTokenPolicy() == nil || tokenBackend.GetTokenPolicy().GetTokensPerMinute() != 1200 {
		t.Fatalf("token backend = %#v, want populated token policy config", tokenBackend.GetTokenPolicy())
	}
	wasmBackend := findProjectedBackend(t, projected, "wasm-backend")
	if wasmBackend.GetWasmPlugin() == nil || wasmBackend.GetWasmPlugin().GetSha256() != "abc123" {
		t.Fatalf("wasm backend = %#v, want populated wasm plugin config", wasmBackend.GetWasmPlugin())
	}
}

func projectionTestSnapshot() *ir.Snapshot {
	return &ir.Snapshot{
		ID:          "projection-snapshot",
		GeneratedAt: time.Unix(1_700_000_000, 0).UTC(),
		Listeners: []ir.Listener{
			{
				Name:           "listener-main",
				Address:        "0.0.0.0",
				Port:           8443,
				Protocol:       "HTTP",
				AttachedRoutes: []string{"http-labeled", "http-direct-response", "grpc-token", "stream-wasm"},
			},
			{
				Name:           "listener-pruned",
				Address:        "0.0.0.0",
				Port:           9443,
				Protocol:       "HTTP",
				AttachedRoutes: []string{"http-ai-only"},
			},
		},
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "http-labeled",
				Namespace: "default",
				Labels:    map[string]string{"env": "prod"},
				Rules: []ir.HTTPRule{{
					Name: "plain",
					BackendRefs: []ir.BackendRef{{
						Name:      "plain-backend",
						Namespace: "default",
						Port:      8080,
					}},
				}},
			},
			{
				Name:      "http-direct-response",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					Name: "direct-response",
					Filters: []ir.Filter{{
						Type: "ExtensionRef",
						Config: map[string]any{
							"extensionType": "DirectResponse",
						},
					}},
					BackendRefs: []ir.BackendRef{{
						Name:      "ai-backend",
						Namespace: "default",
						Port:      8080,
					}},
				}},
			},
			{
				Name:      "http-ai-only",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					Name: "ai-only",
					BackendRefs: []ir.BackendRef{{
						Name:      "ai-backend",
						Namespace: "default",
						Port:      8080,
					}},
				}},
			},
		},
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc-token",
			Namespace: "default",
			Labels:    map[string]string{"tier": "gold"},
			Rules: []ir.GRPCRule{{
				Name: "grpc",
				BackendRefs: []ir.BackendRef{{
					Name:      "token-backend",
					Namespace: "default",
					Port:      9000,
				}},
			}},
		}},
		StreamRoutes: []ir.StreamRoute{{
			Name:      "stream-wasm",
			Namespace: "default",
			Kind:      "TLS",
			Labels:    map[string]string{"protocol": "tls"},
			Rules: []ir.StreamRule{{
				Name: "stream",
				BackendRefs: []ir.BackendRef{{
					Name:      "wasm-backend",
					Namespace: "default",
					Port:      9443,
				}},
			}},
		}},
		Backends: []ir.BackendCluster{
			{
				Name:           "plain-backend",
				Namespace:      "default",
				Protocol:       "HTTP",
				ConnectTimeout: 5 * time.Second,
				Endpoints: []ir.BackendEndpoint{{
					Address: "10.0.0.1",
					Port:    8080,
					Healthy: true,
				}},
			},
			{
				Name:           "ai-backend",
				Namespace:      "default",
				Protocol:       "HTTP",
				ConnectTimeout: 5 * time.Second,
				AIService: &ir.AIServiceConfig{
					Provider: "openai",
					Format:   "openai",
					Model:    "gpt-4.1-mini",
					Auth: ir.AIServiceAuth{
						Type:      "Bearer",
						SecretRef: "default/openai-key",
						Header:    "Authorization",
					},
					Timeout: 2 * time.Second,
				},
				Endpoints: []ir.BackendEndpoint{{
					Address: "10.0.0.2",
					Port:    8080,
					Healthy: true,
				}},
			},
			{
				Name:           "token-backend",
				Namespace:      "default",
				Protocol:       "GRPC",
				ConnectTimeout: 5 * time.Second,
				TokenPolicy: &ir.TokenPolicyConfig{
					TokensPerMinute:   1200,
					TokensPerHour:     72000,
					RequestsPerMinute: 60,
					Scope:             "consumer",
					Burst:             1.5,
					OnLimit:           "Throttle",
				},
				Endpoints: []ir.BackendEndpoint{{
					Address: "10.0.0.3",
					Port:    9000,
					Healthy: true,
				}},
			},
			{
				Name:           "wasm-backend",
				Namespace:      "default",
				Protocol:       "TLS",
				ConnectTimeout: 5 * time.Second,
				WasmPlugin: &ir.WasmPluginConfig{
					Name:       "authz",
					Namespace:  "default",
					WasmBytes:  []byte("plugin"),
					SHA256:     "abc123",
					Hooks:      []string{"request"},
					ConfigJSON: "{\"mode\":\"strict\"}",
					Sandbox: ir.WasmSandboxConfig{
						MaxMemoryBytes:     4096,
						MaxExecutionTimeMs: 25,
						AllowNetwork:       false,
						AllowFileSystem:    false,
					},
				},
				Endpoints: []ir.BackendEndpoint{{
					Address: "10.0.0.4",
					Port:    9443,
					Healthy: true,
				}},
			},
		},
	}
}

func findProjectedBackend(t *testing.T, snapshot *controlv1.ConfigSnapshot, name string) *controlv1.BackendCluster {
	t.Helper()

	for _, backend := range snapshot.GetBackends() {
		if backend.GetName() == name {
			return backend
		}
	}
	t.Fatalf("backend %q not found in snapshot", name)
	return nil
}

func findProjectedHTTPRoute(t *testing.T, snapshot *controlv1.ConfigSnapshot, name string) *controlv1.HttpRoute {
	t.Helper()

	for _, route := range snapshot.GetHttpRoutes() {
		if route.GetName() == name {
			return route
		}
	}
	t.Fatalf("http route %q not found in snapshot", name)
	return nil
}

func hasProjectedHTTPRoute(snapshot *controlv1.ConfigSnapshot, name string) bool {
	for _, route := range snapshot.GetHttpRoutes() {
		if route.GetName() == name {
			return true
		}
	}
	return false
}

func findProjectedGRPCRoute(t *testing.T, snapshot *controlv1.ConfigSnapshot, name string) *controlv1.GrpcRoute {
	t.Helper()

	for _, route := range snapshot.GetGrpcRoutes() {
		if route.GetName() == name {
			return route
		}
	}
	t.Fatalf("grpc route %q not found in snapshot", name)
	return nil
}

func findProjectedStreamRoute(t *testing.T, snapshot *controlv1.ConfigSnapshot, name string) *controlv1.StreamRoute {
	t.Helper()

	for _, route := range snapshot.GetStreamRoutes() {
		if route.GetName() == name {
			return route
		}
	}
	t.Fatalf("stream route %q not found in snapshot", name)
	return nil
}
