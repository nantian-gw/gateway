package backends

import (
	"sort"

	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/loadbalancing"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

type translatedBackendLBPolicy struct {
	backendKeys        []string
	sessionPersistence *ir.SessionPersistencePolicy
	loadBalancing      *ir.LoadBalancingPolicy
	circuitBreaker     *ir.CircuitBreakerConfig
	healthCheck        *ir.HealthCheckConfig
	outlierDetection   *ir.OutlierDetectionConfig
	policy             backend.BackendLBPolicy
}

type BackendLBPolicyIndexes struct {
	sessionPersistence map[string]*ir.SessionPersistencePolicy
	loadBalancing      map[string]*ir.LoadBalancingPolicy
	circuitBreaker     map[string]*ir.CircuitBreakerConfig
	healthCheck        map[string]*ir.HealthCheckConfig
	outlierDetection   map[string]*ir.OutlierDetectionConfig
}

func BuildBackendLBPolicyIndexesWithIndexes(
	policies []backend.BackendLBPolicy,
	indexes shared.TranslatorIndexes,
) BackendLBPolicyIndexes {
	translations := make([]translatedBackendLBPolicy, 0, len(policies))
	owners := make(map[string]int)

	for _, policy := range policies {
		sessionPersistence := BackendSessionPersistence(
			policy.Namespace,
			policy.Name,
			policy.Spec.SessionPersistence,
		)
		loadBalancing := BackendLoadBalancing(policy.Spec.LoadBalancing)
		circuitBreaker := backendCircuitBreaker(policy.Spec.CircuitBreaker)
		healthCheck := BackendHealthCheck(policy.Spec.HealthCheck)
		outlierDetection := BackendOutlierDetection(policy.Spec.OutlierDetection)
		if sessionPersistence == nil && loadBalancing == nil && circuitBreaker == nil && healthCheck == nil && outlierDetection == nil {
			continue
		}

		backendKeys, ok := backendLBPolicyBackendKeysWithIndexes(policy, indexes)
		if !ok || len(backendKeys) == 0 {
			continue
		}

		translations = append(translations, translatedBackendLBPolicy{
			backendKeys:        backendKeys,
			sessionPersistence: sessionPersistence,
			loadBalancing:      loadBalancing,
			circuitBreaker:     circuitBreaker,
			healthCheck:        healthCheck,
			outlierDetection:   outlierDetection,
			policy:             policy,
		})
		translationIndex := len(translations) - 1
		for _, backendKey := range backendKeys {
			currentOwner, exists := owners[backendKey]
			if !exists || loadbalancing.PolicyPrecedes(policy, translations[currentOwner].policy) {
				owners[backendKey] = translationIndex
			}
		}
	}

	sessionPersistence := make(map[string]*ir.SessionPersistencePolicy, len(owners))
	loadBalancing := make(map[string]*ir.LoadBalancingPolicy, len(owners))
	circuitBreaker := make(map[string]*ir.CircuitBreakerConfig, len(owners))
	healthCheck := make(map[string]*ir.HealthCheckConfig, len(owners))
	outlierDetection := make(map[string]*ir.OutlierDetectionConfig, len(owners))
	for backendKey, ownerIndex := range owners {
		if item := translations[ownerIndex].sessionPersistence; item != nil {
			copyItem := *item
			sessionPersistence[backendKey] = &copyItem
		}
		if item := translations[ownerIndex].loadBalancing; item != nil {
			copyItem := *item
			if item.ConsistentHash != nil {
				hashCopy := *item.ConsistentHash
				copyItem.ConsistentHash = &hashCopy
			}
			loadBalancing[backendKey] = &copyItem
		}
		if item := translations[ownerIndex].circuitBreaker; item != nil {
			copyItem := *item
			circuitBreaker[backendKey] = &copyItem
		}
		if item := translations[ownerIndex].healthCheck; item != nil {
			copyItem := *item
			healthCheck[backendKey] = &copyItem
		}
		if item := translations[ownerIndex].outlierDetection; item != nil {
			copyItem := *item
			outlierDetection[backendKey] = &copyItem
		}
	}

	return BackendLBPolicyIndexes{
		sessionPersistence: sessionPersistence,
		loadBalancing:      loadBalancing,
		circuitBreaker:     circuitBreaker,
		healthCheck:        healthCheck,
		outlierDetection:   outlierDetection,
	}
}

func backendLBPolicyBackendKeysWithIndexes(
	policy backend.BackendLBPolicy,
	indexes shared.TranslatorIndexes,
) ([]string, bool) {
	keys := make([]string, 0)
	for _, targetRef := range policy.Spec.TargetRefs {
		group := string(targetRef.Group)
		kind := string(targetRef.Kind)

		switch {
		case group == "" && kind == "Service":
			service, ok := indexes.Service(policy.Namespace, string(targetRef.Name))
			if !ok {
				return nil, false
			}
			targetKeys, ok := serviceBackendKeys(
				policy.Namespace,
				service.Name,
				service.Spec.Ports,
				nil,
			)
			if !ok {
				return nil, false
			}
			keys = append(keys, targetKeys...)
		case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
			serviceImport, ok := indexes.ServiceImport(policy.Namespace, string(targetRef.Name))
			if !ok {
				return nil, false
			}
			targetKeys, ok := serviceImportBackendKeys(
				policy.Namespace,
				serviceImport.Name,
				serviceImport.Spec.Ports,
				nil,
			)
			if !ok {
				return nil, false
			}
			keys = append(keys, targetKeys...)
		default:
			return nil, false
		}
	}

	sort.Strings(keys)
	return compactStrings(keys), true
}

func backendCircuitBreaker(cb *backend.CircuitBreakerConfig) *ir.CircuitBreakerConfig {
	if cb == nil || cb.MaxInflightRequests == nil {
		return nil
	}
	return &ir.CircuitBreakerConfig{
		MaxInflightRequests: int(*cb.MaxInflightRequests),
	}
}

func BackendHealthCheck(hc *backend.HealthCheckConfig) *ir.HealthCheckConfig {
	if hc == nil {
		return nil
	}
	out := &ir.HealthCheckConfig{
		Path:               derefStr(hc.Path),
		Interval:           hc.Interval,
		Timeout:            hc.Timeout,
		HealthyThreshold:   derefU32(hc.HealthyThreshold),
		UnhealthyThreshold: derefU32(hc.UnhealthyThreshold),
	}
	if hc.Type != nil {
		out.Type = *hc.Type
	}
	if hc.ExpectedStatus != nil {
		out.ExpectedStatus = *hc.ExpectedStatus
	}
	return out
}

func BackendOutlierDetection(od *backend.OutlierDetectionConfig) *ir.OutlierDetectionConfig {
	if od == nil {
		return nil
	}
	return &ir.OutlierDetectionConfig{
		Consecutive5xx:     derefU32(od.Consecutive5xx),
		Interval:           od.Interval,
		BaseEjectionTime:   od.BaseEjectionTime,
		MaxEjectionPercent: derefU32(od.MaxEjectionPercent),
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefU32(v *uint32) uint32 {
	if v == nil {
		return 0
	}
	return *v
}
