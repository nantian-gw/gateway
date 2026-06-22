package lbpolicy

import (
	"fmt"
	"strings"

	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
)

func ValidateLoadBalancing(policy *backendlb.LoadBalancingPolicy) error {
	if policy == nil {
		return nil
	}

	switch EffectiveLoadBalancingType(policy) {
	case backendlb.LoadBalancingStrategyTypeRoundRobin:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy round robin strategy does not accept consistentHash config")
		}
		return nil
	case backendlb.LoadBalancingStrategyTypeLeastRequest:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy least request strategy does not accept consistentHash config")
		}
		return nil
	case backendlb.LoadBalancingStrategyTypeRandom:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy random strategy does not accept consistentHash config")
		}
		return nil
	case backendlb.LoadBalancingStrategyTypeConsistentHash:
		return validateConsistentHash(policy.ConsistentHash)
	default:
		return fmt.Errorf("BackendLBPolicy load balancing type %q is not supported", EffectiveLoadBalancingType(policy))
	}
}

func EffectiveLoadBalancingType(
	policy *backendlb.LoadBalancingPolicy,
) backendlb.LoadBalancingStrategyType {
	if policy == nil {
		return ""
	}
	if policy.Type != nil && strings.TrimSpace(string(*policy.Type)) != "" {
		return backendlb.LoadBalancingStrategyType(strings.TrimSpace(string(*policy.Type)))
	}
	if policy.ConsistentHash != nil {
		return backendlb.LoadBalancingStrategyTypeConsistentHash
	}
	return backendlb.LoadBalancingStrategyTypeRoundRobin
}

func EffectiveConsistentHashKeyType(
	policy *backendlb.ConsistentHashPolicy,
) backendlb.HashKeyType {
	if policy == nil || policy.KeyType == nil {
		return ""
	}
	return backendlb.HashKeyType(strings.TrimSpace(string(*policy.KeyType)))
}

func validateConsistentHash(policy *backendlb.ConsistentHashPolicy) error {
	if policy == nil {
		return fmt.Errorf("BackendLBPolicy consistent hash strategy requires consistentHash config")
	}

	switch EffectiveConsistentHashKeyType(policy) {
	case backendlb.HashKeyTypeSourceIP, backendlb.HashKeyTypeHostname:
		if policy.HeaderName != nil && strings.TrimSpace(*policy.HeaderName) != "" {
			return fmt.Errorf("BackendLBPolicy consistent hash %s strategy does not accept headerName", strings.ToLower(string(EffectiveConsistentHashKeyType(policy))))
		}
		return nil
	case backendlb.HashKeyTypeHeader:
		if policy.HeaderName == nil || strings.TrimSpace(*policy.HeaderName) == "" {
			return fmt.Errorf("BackendLBPolicy consistent hash header strategy requires headerName")
		}
		return nil
	default:
		return fmt.Errorf("BackendLBPolicy consistent hash key type %q is not supported", EffectiveConsistentHashKeyType(policy))
	}
}
