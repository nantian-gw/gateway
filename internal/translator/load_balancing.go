package translator

import (
	"strings"

	"github.com/nantian-gw/gateway/internal/lbpolicy"
	backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
)

func backendLoadBalancing(source *backendlb.LoadBalancingPolicy) *ir.LoadBalancingPolicy {
	if source == nil {
		return nil
	}
	if err := lbpolicy.ValidateLoadBalancing(source); err != nil {
		return nil
	}

	out := &ir.LoadBalancingPolicy{
		Type: string(lbpolicy.EffectiveLoadBalancingType(source)),
	}

	if out.Type == string(backendlb.LoadBalancingStrategyTypeConsistentHash) {
		hash := source.ConsistentHash
		headerName := ""
		if hash != nil && hash.HeaderName != nil {
			headerName = strings.TrimSpace(*hash.HeaderName)
		}
		out.ConsistentHash = &ir.ConsistentHashPolicy{
			KeyType:    string(lbpolicy.EffectiveConsistentHashKeyType(hash)),
			HeaderName: headerName,
		}
	}

	return out
}
