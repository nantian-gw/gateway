package routes

import (
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func TranslateHTTPRoute(route gatewayv1.HTTPRoute) ir.HTTPRoute {
	return TranslateHTTPRouteWithResolver(route, extfilter.Resolver{}, nil)
}

func TranslateHTTPRouteWithResolver(
	route gatewayv1.HTTPRoute,
	resolver extfilter.Resolver,
	rawFilterConfigs RawHTTPRouteFilterConfigs,
) ir.HTTPRoute {
	return TranslateHTTPRouteWithDefaultGateways(route, resolver, rawFilterConfigs, nil)
}

func TranslateHTTPRouteWithDefaultGateways(
	route gatewayv1.HTTPRoute,
	resolver extfilter.Resolver,
	rawFilterConfigs RawHTTPRouteFilterConfigs,
	defaultGateways []gatewayv1.Gateway,
) ir.HTTPRoute {
	validation := gatewayapi.ValidateHTTPRouteRules(route)
	invalidRules := make(map[int]struct{}, len(validation.InvalidRuleIndexes))
	for _, index := range validation.InvalidRuleIndexes {
		invalidRules[index] = struct{}{}
	}
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)

	out := ir.HTTPRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Hostnames:   shared.Hostnames(route.Spec.Hostnames),
		ParentRefs:  shared.GatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      shared.RouteStatusSummary(route.Status.Parents, route.Namespace),
	}

	for index, rule := range route.Spec.Rules {
		if _, invalid := invalidRules[index]; invalid {
			continue
		}

		item := ir.HTTPRule{
			Name: shared.StringValue((*string)(rule.Name)),
			Filters: FiltersFromHTTPWithResolver(
				rule.Filters,
				route.Namespace,
				resolver,
				extfilter.TargetHTTP,
				rawFilterConfigs,
				index,
			),
			BackendRefs:        BackendRefsFromHTTP(rule.BackendRefs, route.Namespace),
			Timeouts:           shared.HTTPRouteTimeouts(rule.Timeouts),
			Retry:              HTTPRouteRetry(rule.Retry),
			SessionPersistence: backends.RuleSessionPersistence("http", route.Namespace, route.Name, index, rule.SessionPersistence),
		}

		for _, match := range rule.Matches {
			httpMatch := ir.HTTPMatch{}
			if match.Path != nil {
				if match.Path.Value != nil {
					httpMatch.Path = *match.Path.Value
				}
				if match.Path.Type != nil {
					httpMatch.PathType = string(*match.Path.Type)
				}
			}
			if match.Method != nil {
				httpMatch.Method = string(*match.Method)
			}
			for _, header := range match.Headers {
				item := ir.HeaderMatch{
					Name:  string(header.Name),
					Value: header.Value,
				}
				if header.Type != nil {
					item.MatchType = string(*header.Type)
				}
				httpMatch.Headers = append(httpMatch.Headers, item)
			}
			for _, query := range match.QueryParams {
				item := ir.QueryMatch{
					Name:  string(query.Name),
					Value: query.Value,
				}
				if query.Type != nil {
					item.MatchType = string(*query.Type)
				}
				httpMatch.QueryParams = append(httpMatch.QueryParams, item)
			}
			item.Matches = append(item.Matches, httpMatch)
		}

		out.Rules = append(out.Rules, item)
	}

	return out
}

func HTTPRouteRetry(retry *gatewayv1.HTTPRouteRetry) *ir.RetryPolicy {
	if retry == nil {
		return nil
	}

	out := &ir.RetryPolicy{}
	for _, code := range retry.Codes {
		out.Codes = append(out.Codes, uint32(code))
	}
	if retry.Attempts != nil {
		out.Attempts = uint32(*retry.Attempts)
	}
	if retry.Backoff != nil {
		duration, err := time.ParseDuration(string(*retry.Backoff))
		if err == nil {
			out.Backoff = &duration
		}
	}

	if len(out.Codes) == 0 && out.Attempts == 0 && out.Backoff == nil {
		return nil
	}

	return out
}
