package backendlb

import (
	"fmt"
	"strings"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func ValidateLoadBalancing(policy *backendlbv1alpha2.LoadBalancingPolicy) error {
	if policy == nil {
		return nil
	}

	switch EffectiveLoadBalancingType(policy) {
	case backendlbv1alpha2.LoadBalancingStrategyTypeRoundRobin:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy round robin strategy does not accept consistentHash config")
		}
		return nil
	case backendlbv1alpha2.LoadBalancingStrategyTypeLeastRequest:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy least request strategy does not accept consistentHash config")
		}
		return nil
	case backendlbv1alpha2.LoadBalancingStrategyTypeRandom:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy random strategy does not accept consistentHash config")
		}
		return nil
	case backendlbv1alpha2.LoadBalancingStrategyTypeConsistentHash:
		return validateConsistentHash(policy.ConsistentHash)
	default:
		return fmt.Errorf("BackendLBPolicy load balancing type %q is not supported", EffectiveLoadBalancingType(policy))
	}
}

func EffectiveLoadBalancingType(
	policy *backendlbv1alpha2.LoadBalancingPolicy,
) backendlbv1alpha2.LoadBalancingStrategyType {
	if policy == nil {
		return ""
	}
	if policy.Type != nil && strings.TrimSpace(string(*policy.Type)) != "" {
		return backendlbv1alpha2.LoadBalancingStrategyType(strings.TrimSpace(string(*policy.Type)))
	}
	if policy.ConsistentHash != nil {
		return backendlbv1alpha2.LoadBalancingStrategyTypeConsistentHash
	}
	return backendlbv1alpha2.LoadBalancingStrategyTypeRoundRobin
}

func EffectiveConsistentHashKeyType(
	policy *backendlbv1alpha2.ConsistentHashPolicy,
) backendlbv1alpha2.HashKeyType {
	if policy == nil || policy.KeyType == nil {
		return ""
	}
	return backendlbv1alpha2.HashKeyType(strings.TrimSpace(string(*policy.KeyType)))
}

func validateConsistentHash(policy *backendlbv1alpha2.ConsistentHashPolicy) error {
	if policy == nil {
		return fmt.Errorf("BackendLBPolicy consistent hash strategy requires consistentHash config")
	}

	switch EffectiveConsistentHashKeyType(policy) {
	case backendlbv1alpha2.HashKeyTypeSourceIP, backendlbv1alpha2.HashKeyTypeHostname:
		if policy.HeaderName != nil && strings.TrimSpace(*policy.HeaderName) != "" {
			return fmt.Errorf("BackendLBPolicy consistent hash %s strategy does not accept headerName", strings.ToLower(string(EffectiveConsistentHashKeyType(policy))))
		}
		return nil
	case backendlbv1alpha2.HashKeyTypeHeader:
		if policy.HeaderName == nil || strings.TrimSpace(*policy.HeaderName) == "" {
			return fmt.Errorf("BackendLBPolicy consistent hash header strategy requires headerName")
		}
		return nil
	default:
		return fmt.Errorf("BackendLBPolicy consistent hash key type %q is not supported", EffectiveConsistentHashKeyType(policy))
	}
}
