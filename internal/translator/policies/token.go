package policies

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func TranslateTokenPolicy(policy tokenpolicy.TokenPolicy) ir.TokenPolicyConfig {
	return ir.TokenPolicyConfig{
		TokensPerMinute:   policy.Spec.TokensPerMinute,
		TokensPerHour:     policy.Spec.TokensPerHour,
		RequestsPerMinute: policy.Spec.RequestsPerMinute,
		Scope:             policy.Spec.Scope,
		Burst:             policy.Spec.Burst,
		OnLimit:           policy.Spec.OnLimit,
	}
}

func TranslateTokenPolicies(
	policies []tokenpolicy.TokenPolicy,
	services map[string]struct{},
	serviceImports map[string]struct{},
	httpRoutes map[string][]string,
) map[string]ir.TokenPolicyConfig {
	result := make(map[string]ir.TokenPolicyConfig, len(policies))
	for _, policy := range policies {
		cfg := TranslateTokenPolicy(policy)
		for _, targetRef := range policy.Spec.TargetRefs {
			key := shared.BackendObjectKey(policy.Namespace, string(targetRef.Name))
			switch {
			case targetRef.Kind == "Service" && targetRef.Group == "":
				if _, ok := services[key]; ok {
					result[key] = cfg
				}
			case targetRef.Kind == "ServiceImport" && targetRef.Group == "multicluster.x-k8s.io":
				if _, ok := serviceImports[key]; ok {
					result[key] = cfg
				}
			case targetRef.Kind == "HTTPRoute" && targetRef.Group == "gateway.networking.k8s.io":
				if backendKeys, ok := httpRoutes[key]; ok {
					for _, bk := range backendKeys {
						if _, exists := services[bk]; exists {
							result[bk] = cfg
						}
					}
				}
			}
		}
	}
	return result
}

func ServiceKeySet(services []corev1.Service) map[string]struct{} {
	s := make(map[string]struct{}, len(services))
	for _, svc := range services {
		s[shared.BackendObjectKey(svc.Namespace, svc.Name)] = struct{}{}
	}
	return s
}

func ServiceImportKeySet(serviceImports []mcsv1alpha1.ServiceImport) map[string]struct{} {
	s := make(map[string]struct{}, len(serviceImports))
	for _, si := range serviceImports {
		s[shared.BackendObjectKey(si.Namespace, si.Name)] = struct{}{}
	}
	return s
}

func BuildRouteBackendServices(routes []gatewayv1.HTTPRoute) map[string][]string {
	result := make(map[string][]string)
	for _, route := range routes {
		key := shared.BackendObjectKey(route.Namespace, route.Name)
		var backends []string
		for _, rule := range route.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				ns := route.Namespace
				if ref.Namespace != nil {
					ns = string(*ref.Namespace)
				}
				bk := shared.BackendObjectKey(ns, string(ref.Name))
				backends = append(backends, bk)
			}
		}
		result[key] = backends
	}
	return result
}