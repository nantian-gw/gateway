package backends

import (
	"strings"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/loadbalancing"
)

func BackendLoadBalancing(source *backend.LoadBalancingPolicy) *ir.LoadBalancingPolicy {
	if source == nil {
		return nil
	}
	if err := loadbalancing.ValidateLoadBalancing(source); err != nil {
		return nil
	}

	out := &ir.LoadBalancingPolicy{
		Type: string(loadbalancing.EffectiveLoadBalancingType(source)),
	}

	if out.Type == string(backend.LoadBalancingStrategyTypeConsistentHash) {
		hash := source.ConsistentHash
		headerName := ""
		if hash != nil && hash.HeaderName != nil {
			headerName = strings.TrimSpace(*hash.HeaderName)
		}
		out.ConsistentHash = &ir.ConsistentHashPolicy{
			KeyType:    string(loadbalancing.EffectiveConsistentHashKeyType(hash)),
			HeaderName: headerName,
		}
	}

	return out
}
