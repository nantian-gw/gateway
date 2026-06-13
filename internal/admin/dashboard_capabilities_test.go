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

	if !got.Overview || !got.Gateways || !got.Settings {
		t.Fatalf("core dashboard pages should be enabled: %+v", got)
	}
	if !got.AIOverview || !got.AIServices || !got.Chatbot {
		t.Fatalf("AI capabilities should follow enableAiGateway=true: %+v", got)
	}
	if got.AITokenPolicies || got.WasmPlugins {
		t.Fatalf("experimental dashboard capabilities should remain disabled: %+v", got)
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

	if got.Overview || got.AIOverview || got.WasmPlugins || got.Chatbot {
		t.Fatalf("dashboard.enabled=false must disable all page groups: %+v", got)
	}
}
