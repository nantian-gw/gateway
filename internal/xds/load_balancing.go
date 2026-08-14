package xds

import (
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func toProtoLoadBalancing(item *ir.LoadBalancingPolicy) *controlv1.LoadBalancingPolicy {
	if item == nil {
		return nil
	}

	out := &controlv1.LoadBalancingPolicy{
		Type: toProtoLoadBalancingType(item.Type),
	}
	if item.ConsistentHash != nil {
		out.ConsistentHash = &controlv1.ConsistentHashPolicy{
			KeyType:    toProtoConsistentHashKeyType(item.ConsistentHash.KeyType),
			HeaderName: item.ConsistentHash.HeaderName,
		}
	}

	return out
}

func toProtoLoadBalancingType(value string) controlv1.LoadBalancingPolicyType {
	switch value {
	case "RoundRobin":
		return controlv1.LoadBalancingPolicyType_LOAD_BALANCING_ROUND_ROBIN
	case "ConsistentHash":
		return controlv1.LoadBalancingPolicyType_LOAD_BALANCING_CONSISTENT_HASH
	default:
		return controlv1.LoadBalancingPolicyType_LOAD_BALANCING_UNSPECIFIED
	}
}

func toProtoConsistentHashKeyType(value string) controlv1.ConsistentHashKeyType {
	switch value {
	case "SourceIP":
		return controlv1.ConsistentHashKeyType_CONSISTENT_HASH_SOURCE_IP
	case "Header":
		return controlv1.ConsistentHashKeyType_CONSISTENT_HASH_HEADER
	case "Hostname":
		return controlv1.ConsistentHashKeyType_CONSISTENT_HASH_HOSTNAME
	default:
		return controlv1.ConsistentHashKeyType_CONSISTENT_HASH_UNSPECIFIED
	}
}
