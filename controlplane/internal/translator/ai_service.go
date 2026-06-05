package translator

import (
	"time"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/aiservicev1alpha1"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func translateAIService(svc aiservicev1alpha1.AIService) ir.AIServiceConfig {
	cfg := ir.AIServiceConfig{
		Provider:  svc.Spec.Provider,
		Format:    svc.Spec.Format,
		Model:     svc.Spec.Model,
		Auth: ir.AIServiceAuth{
			Type:      svc.Spec.Auth.Type,
			SecretRef: svc.Namespace + "/" + svc.Spec.Auth.Secret,
			Header:    svc.Spec.Auth.Header,
		},
	}
	if svc.Spec.Timeout != "" {
		if d, err := time.ParseDuration(svc.Spec.Timeout); err == nil {
			cfg.Timeout = d
		}
	}
	return cfg
}

func translateAIServices(svcs []aiservicev1alpha1.AIService) map[string]ir.AIServiceConfig {
	result := make(map[string]ir.AIServiceConfig, len(svcs))
	for _, svc := range svcs {
		key := backendObjectKey(svc.Namespace, svc.Name)
		result[key] = translateAIService(svc)
	}
	return result
}