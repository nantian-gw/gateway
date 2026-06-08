package translator

import (
	"strings"

	"github.com/nantian-gw/gateway/internal/backendlb"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/internal/ir"
)

func backendLoadBalancing(source *backendlbv1alpha2.LoadBalancingPolicy) *ir.LoadBalancingPolicy {
	if source == nil {
		return nil
	}
	if err := backendlb.ValidateLoadBalancing(source); err != nil {
		return nil
	}

	out := &ir.LoadBalancingPolicy{
		Type: string(backendlb.EffectiveLoadBalancingType(source)),
	}

	if out.Type == string(backendlbv1alpha2.LoadBalancingStrategyTypeConsistentHash) {
		hash := source.ConsistentHash
		headerName := ""
		if hash != nil && hash.HeaderName != nil {
			headerName = strings.TrimSpace(*hash.HeaderName)
		}
		out.ConsistentHash = &ir.ConsistentHashPolicy{
			KeyType:    string(backendlb.EffectiveConsistentHashKeyType(hash)),
			HeaderName: headerName,
		}
	}

	return out
}
