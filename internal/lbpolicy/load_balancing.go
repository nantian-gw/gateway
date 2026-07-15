package lbpolicy

import (
	"fmt"
	"strings"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

func ValidateLoadBalancing(policy *backend.LoadBalancingPolicy) error {
	if policy == nil {
		return nil
	}

	switch EffectiveLoadBalancingType(policy) {
	case backend.LoadBalancingStrategyTypeRoundRobin:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy round robin strategy does not accept consistentHash config")
		}
		return nil
	case backend.LoadBalancingStrategyTypeLeastRequest:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy least request strategy does not accept consistentHash config")
		}
		return nil
	case backend.LoadBalancingStrategyTypeRandom:
		if policy.ConsistentHash != nil {
			return fmt.Errorf("BackendLBPolicy random strategy does not accept consistentHash config")
		}
		return nil
	case backend.LoadBalancingStrategyTypeConsistentHash:
		return validateConsistentHash(policy.ConsistentHash)
	default:
		return fmt.Errorf("BackendLBPolicy load balancing type %q is not supported", EffectiveLoadBalancingType(policy))
	}
}

func EffectiveLoadBalancingType(
	policy *backend.LoadBalancingPolicy,
) backend.LoadBalancingStrategyType {
	if policy == nil {
		return ""
	}
	if policy.Type != nil && strings.TrimSpace(string(*policy.Type)) != "" {
		return backend.LoadBalancingStrategyType(strings.TrimSpace(string(*policy.Type)))
	}
	if policy.ConsistentHash != nil {
		return backend.LoadBalancingStrategyTypeConsistentHash
	}
	return backend.LoadBalancingStrategyTypeRoundRobin
}

func EffectiveConsistentHashKeyType(
	policy *backend.ConsistentHashPolicy,
) backend.HashKeyType {
	if policy == nil || policy.KeyType == nil {
		return ""
	}
	return backend.HashKeyType(strings.TrimSpace(string(*policy.KeyType)))
}

func validateConsistentHash(policy *backend.ConsistentHashPolicy) error {
	if policy == nil {
		return fmt.Errorf("BackendLBPolicy consistent hash strategy requires consistentHash config")
	}

	switch EffectiveConsistentHashKeyType(policy) {
	case backend.HashKeyTypeSourceIP, backend.HashKeyTypeHostname:
		if policy.HeaderName != nil && strings.TrimSpace(*policy.HeaderName) != "" {
			return fmt.Errorf("BackendLBPolicy consistent hash %s strategy does not accept headerName", strings.ToLower(string(EffectiveConsistentHashKeyType(policy))))
		}
		return nil
	case backend.HashKeyTypeHeader:
		if policy.HeaderName == nil || strings.TrimSpace(*policy.HeaderName) == "" {
			return fmt.Errorf("BackendLBPolicy consistent hash header strategy requires headerName")
		}
		return nil
	default:
		return fmt.Errorf("BackendLBPolicy consistent hash key type %q is not supported", EffectiveConsistentHashKeyType(policy))
	}
}
