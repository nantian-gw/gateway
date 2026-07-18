package routes

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func TranslateGRPCRoute(route gatewayv1.GRPCRoute) ir.GRPCRoute {
	return TranslateGRPCRouteWithResolver(route, extfilter.Resolver{})
}

func TranslateGRPCRouteWithResolver(route gatewayv1.GRPCRoute, resolver extfilter.Resolver) ir.GRPCRoute {
	return TranslateGRPCRouteWithDefaultGateways(route, resolver, nil)
}

func TranslateGRPCRouteWithDefaultGateways(route gatewayv1.GRPCRoute, resolver extfilter.Resolver, defaultGateways []gatewayv1.Gateway) ir.GRPCRoute {
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.GRPCRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Hostnames:   shared.Hostnames(route.Spec.Hostnames),
		ParentRefs:  shared.GatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      shared.RouteStatusSummary(route.Status.Parents, route.Namespace),
	}

	for index, rule := range route.Spec.Rules {
		item := ir.GRPCRule{
			Name:               shared.StringValue((*string)(rule.Name)),
			Filters:            FiltersFromGRPCWithResolver(rule.Filters, route.Namespace, resolver, extfilter.TargetGRPC),
			BackendRefs:        BackendRefsFromGRPC(rule.BackendRefs, route.Namespace),
			SessionPersistence: backends.RuleSessionPersistence("grpc", route.Namespace, route.Name, index, rule.SessionPersistence),
		}

		for _, match := range rule.Matches {
			grpcMatch := ir.GRPCMatch{}
			if match.Method != nil {
				grpcMatch.Service = shared.StringValue(match.Method.Service)
				grpcMatch.Method = shared.StringValue(match.Method.Method)
				if match.Method.Type != nil {
					grpcMatch.MatchType = string(*match.Method.Type)
				}
			}
			for _, header := range match.Headers {
				item := ir.HeaderMatch{
					Name:  string(header.Name),
					Value: header.Value,
				}
				if header.Type != nil {
					item.MatchType = string(*header.Type)
				}
				grpcMatch.Headers = append(grpcMatch.Headers, item)
			}
			item.Matches = append(item.Matches, grpcMatch)
		}

		out.Rules = append(out.Rules, item)
	}

	return out
}
