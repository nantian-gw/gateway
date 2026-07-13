package translator

import (
	"log/slog"
	"time"

	"github.com/nantian-gw/gateway/internal/gwexp/aiservice"
	"github.com/nantian-gw/gateway/internal/ir"
)

func translateAIService(svc aiservice.AIService) ir.AIServiceConfig {
	cfg := ir.AIServiceConfig{
		Provider:  svc.Spec.Provider,
		Format:    svc.Spec.Format,
		Model:     svc.Spec.Model,
		Endpoint:  svc.Spec.Endpoint,
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

func translateAIServices(svcs []aiservice.AIService) map[string]ir.AIServiceConfig {
	result := make(map[string]ir.AIServiceConfig, len(svcs))
	for _, svc := range svcs {
		key := backendObjectKey(svc.Namespace, svc.Name)
		result[key] = translateAIService(svc)
	}
	return result
}