package aiservice

import (
	"log/slog"
	"time"

	"github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	"github.com/nantian-gw/gateway/internal/ir"
)

// Translate converts an AIService CRD to the IR config.
func Translate(svc aiservice.AIService) ir.AIServiceConfig {
	cfg := ir.AIServiceConfig{
		Provider: svc.Spec.Provider,
		Format:   svc.Spec.Format,
		Model:    svc.Spec.Model,
		Endpoint: svc.Spec.Endpoint,
		Auth: ir.AIServiceAuth{
			Type:      svc.Spec.Auth.Type,
			SecretRef: svc.Namespace + "/" + svc.Spec.Auth.Secret,
			Header:    svc.Spec.Auth.Header,
		},
	}
	if svc.Spec.Timeout != "" {
		if d, err := time.ParseDuration(svc.Spec.Timeout); err == nil {
			cfg.Timeout = d
		} else {
			slog.Warn("ai service has invalid timeout, ignoring", "service", svc.Name, "namespace", svc.Namespace, "timeout", svc.Spec.Timeout)
		}
	}
	return cfg
}

// TranslateAll converts a list of AIService CRDs to a name-indexed IR config map.
func TranslateAll(svcs []aiservice.AIService) map[string]ir.AIServiceConfig {
	result := make(map[string]ir.AIServiceConfig, len(svcs))
	for _, svc := range svcs {
		key := svc.Namespace + "/" + svc.Name
		result[key] = Translate(svc)
	}
	return result
}
