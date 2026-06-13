package admin

import (
	"testing"

	"github.com/nantian-gw/gateway/internal/config"
)

func TestResolveDashboardCapabilitiesHonorsDashboardAndRuntimeFlags(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: false,
			EnableAiGateway:           true,
		},
	}

	got := ResolveDashboardCapabilities(cfg)
	want := DashboardCapabilities{
		Overview:        true,
		Gateways:        true,
		Routes:          true,
		ReferenceGrants: true,
		BackendTLS:      true,
		Nodes:           true,
		Diagnostics:     true,
		Observability:   true,
		Settings:        true,
		AIOverview:      true,
		AIServices:      true,
		AITokenPolicies: false,
		AICost:          true,
		AITraces:        true,
		AIUsage:         true,
		WasmPlugins:     false,
		Chatbot:         true,
	}

	if got != want {
		t.Fatalf("unexpected dashboard capabilities: got %+v want %+v", got, want)
	}
}

func TestResolveDashboardCapabilitiesDisablesEverythingWhenDashboardIsDisabled(t *testing.T) {
	t.Parallel()

	enabled := false
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{
			Enabled: &enabled,
		},
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: true,
			EnableAiGateway:           true,
		},
	}

	got := ResolveDashboardCapabilities(cfg)

	if got != (DashboardCapabilities{}) {
		t.Fatalf("dashboard.enabled=false must disable all page groups: %+v", got)
	}
}

func TestResolveDashboardCapabilitiesNilMatchesZeroValueConfig(t *testing.T) {
	t.Parallel()

	got := ResolveDashboardCapabilities(nil)
	want := ResolveDashboardCapabilities(&config.Config{})

	if got != want {
		t.Fatalf("nil config should resolve like zero-value config: got %+v want %+v", got, want)
	}
}

func TestResolveDashboardCapabilitiesRespectsExplicitCapabilityOverrides(t *testing.T) {
	t.Parallel()

	enabled := true
	aiOverview := false
	wasmPlugins := false
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{
			Enabled: &enabled,
			Capabilities: config.DashboardCapabilitiesConfig{
				AIOverview:  &aiOverview,
				WasmPlugins: &wasmPlugins,
			},
		},
		Features: config.FeaturesConfig{
			EnableExperimentalGateway: true,
			EnableAiGateway:           true,
		},
	}

	got := ResolveDashboardCapabilities(cfg)
	want := DashboardCapabilities{
		Overview:        true,
		Gateways:        true,
		Routes:          true,
		ReferenceGrants: true,
		BackendTLS:      true,
		Nodes:           true,
		Diagnostics:     true,
		Observability:   true,
		Settings:        true,
		AIOverview:      false,
		AIServices:      true,
		AITokenPolicies: true,
		AICost:          true,
		AITraces:        true,
		AIUsage:         true,
		WasmPlugins:     false,
		Chatbot:         true,
	}

	if got != want {
		t.Fatalf("unexpected dashboard capabilities with explicit overrides: got %+v want %+v", got, want)
	}
}
