package admin

import "github.com/nantian-gw/gateway/internal/config"

type DashboardCapabilities struct {
	Overview        bool `json:"overview"`
	Gateways        bool `json:"gateways"`
	Routes          bool `json:"routes"`
	ReferenceGrants bool `json:"referenceGrants"`
	BackendTLS      bool `json:"backendTls"`
	Nodes           bool `json:"nodes"`
	Diagnostics     bool `json:"diagnostics"`
	Observability   bool `json:"observability"`
	Settings        bool `json:"settings"`
	AIOverview      bool `json:"aiOverview"`
	AIServices      bool `json:"aiServices"`
	AITokenPolicies bool `json:"aiTokenPolicies"`
	AICost          bool `json:"aiCost"`
	AITraces        bool `json:"aiTraces"`
	AIUsage         bool `json:"aiUsage"`
	WasmPlugins     bool `json:"wasmPlugins"`
	Chatbot         bool `json:"chatbot"`
}

func ResolveDashboardCapabilities(cfg *config.Config) DashboardCapabilities {
	if cfg == nil {
		return DashboardCapabilities{
			Overview:        true,
			Gateways:        true,
			Routes:          true,
			ReferenceGrants: true,
			BackendTLS:      true,
			Nodes:           true,
			Diagnostics:     true,
			Observability:   true,
			Settings:        true,
		}
	}

	if !cfg.DashboardEnabled() {
		return DashboardCapabilities{}
	}

	uiCaps := cfg.DashboardCapabilities()
	return DashboardCapabilities{
		Overview:        true,
		Gateways:        true,
		Routes:          true,
		ReferenceGrants: true,
		BackendTLS:      true,
		Nodes:           true,
		Diagnostics:     true,
		Observability:   true,
		Settings:        true,
		AIOverview:      uiCaps.AIOverview && cfg.Features.EnableAiGateway,
		AIServices:      uiCaps.AIServices && cfg.Features.EnableAiGateway,
		AITokenPolicies: uiCaps.AITokenPolicies && cfg.Features.EnableExperimentalGateway,
		AICost:          uiCaps.AICost && cfg.Features.EnableAiGateway,
		AITraces:        uiCaps.AITraces && cfg.Features.EnableAiGateway,
		AIUsage:         uiCaps.AIUsage && cfg.Features.EnableAiGateway,
		WasmPlugins:     uiCaps.WasmPlugins && cfg.Features.EnableExperimentalGateway,
		Chatbot:         uiCaps.Chatbot && cfg.Features.EnableAiGateway,
	}
}
