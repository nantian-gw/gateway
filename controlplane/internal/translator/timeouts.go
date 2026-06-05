package translator

import (
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func httpRouteTimeouts(timeouts *gatewayv1.HTTPRouteTimeouts) *ir.RouteTimeouts {
	if timeouts == nil {
		return nil
	}

	out := &ir.RouteTimeouts{
		Request:        parseGatewayDuration(timeouts.Request),
		BackendRequest: parseGatewayDuration(timeouts.BackendRequest),
	}
	if out.Request == nil && out.BackendRequest == nil {
		return nil
	}

	return out
}

func parseGatewayDuration(value *gatewayv1.Duration) *time.Duration {
	if value == nil {
		return nil
	}

	duration, err := time.ParseDuration(string(*value))
	if err != nil {
		return nil
	}

	return &duration
}
