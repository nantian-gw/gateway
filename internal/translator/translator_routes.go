package translator

import (
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gwapi"
	"github.com/nantian-gw/gateway/internal/ir"
)

func translateHTTPRoute(route gatewayv1.HTTPRoute) ir.HTTPRoute {
	return translateHTTPRouteWithResolver(route, extfilter.Resolver{}, nil)
}

func translateHTTPRouteWithResolver(
	route gatewayv1.HTTPRoute,
	resolver extfilter.Resolver,
	rawFilterConfigs rawHTTPRouteFilterConfigs,
) ir.HTTPRoute {
	return translateHTTPRouteWithDefaultGateways(route, resolver, rawFilterConfigs, nil)
}

func translateHTTPRouteWithDefaultGateways(
	route gatewayv1.HTTPRoute,
	resolver extfilter.Resolver,
	rawFilterConfigs rawHTTPRouteFilterConfigs,
	defaultGateways []gatewayv1.Gateway,
) ir.HTTPRoute {
	validation := gwapi.ValidateHTTPRouteRules(route)
	invalidRules := make(map[int]struct{}, len(validation.InvalidRuleIndexes))
	for _, index := range validation.InvalidRuleIndexes {
		invalidRules[index] = struct{}{}
	}
	parentRefs := gwapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)

	out := ir.HTTPRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Hostnames:   hostnames(route.Spec.Hostnames),
		ParentRefs:  gatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      routeStatusSummary(route.Status.Parents, route.Namespace),
	}

	for index, rule := range route.Spec.Rules {
		if _, invalid := invalidRules[index]; invalid {
			continue
		}

		item := ir.HTTPRule{
			Name: stringValue((*string)(rule.Name)),
			Filters: filtersFromHTTPWithResolver(
				rule.Filters,
				route.Namespace,
				resolver,
				extfilter.TargetHTTP,
				rawFilterConfigs,
				index,
			),
			BackendRefs:        backendRefsFromHTTP(rule.BackendRefs, route.Namespace),
			Timeouts:           httpRouteTimeouts(rule.Timeouts),
			Retry:              httpRouteRetry(rule.Retry),
			SessionPersistence: ruleSessionPersistence("http", route.Namespace, route.Name, index, rule.SessionPersistence),
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

func translateGRPCRoute(route gatewayv1.GRPCRoute) ir.GRPCRoute {
	return translateGRPCRouteWithResolver(route, extfilter.Resolver{})
}

func translateGRPCRouteWithResolver(route gatewayv1.GRPCRoute, resolver extfilter.Resolver) ir.GRPCRoute {
	return translateGRPCRouteWithDefaultGateways(route, resolver, nil)
}

func translateGRPCRouteWithDefaultGateways(route gatewayv1.GRPCRoute, resolver extfilter.Resolver, defaultGateways []gatewayv1.Gateway) ir.GRPCRoute {
	parentRefs := gwapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.GRPCRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Hostnames:   hostnames(route.Spec.Hostnames),
		ParentRefs:  gatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      routeStatusSummary(route.Status.Parents, route.Namespace),
	}

	for index, rule := range route.Spec.Rules {
		item := ir.GRPCRule{
			Name:               stringValue((*string)(rule.Name)),
			Filters:            filtersFromGRPCWithResolver(rule.Filters, route.Namespace, resolver, extfilter.TargetGRPC),
			BackendRefs:        backendRefsFromGRPC(rule.BackendRefs, route.Namespace),
			SessionPersistence: ruleSessionPersistence("grpc", route.Namespace, route.Name, index, rule.SessionPersistence),
		}

		for _, match := range rule.Matches {
			grpcMatch := ir.GRPCMatch{}
			if match.Method != nil {
				grpcMatch.Service = stringValue(match.Method.Service)
				grpcMatch.Method = stringValue(match.Method.Method)
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

func translateTCPRoute(route gatewayv1alpha2.TCPRoute) ir.StreamRoute {
	return translateTCPRouteWithDefaultGateways(route, nil)
}

func translateTCPRouteWithDefaultGateways(route gatewayv1alpha2.TCPRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gwapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "TCP",
		ParentRefs:  gatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      routeStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		out.Rules = append(out.Rules, ir.StreamRule{
			Name:        stringValue((*string)(rule.Name)),
			Matches:     []ir.StreamMatch{{}},
			BackendRefs: backendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
		})
	}

	return out
}

func translateUDPRoute(route gatewayv1alpha2.UDPRoute) ir.StreamRoute {
	return translateUDPRouteWithDefaultGateways(route, nil)
}

func translateUDPRouteWithDefaultGateways(route gatewayv1alpha2.UDPRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gwapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "UDP",
		ParentRefs:  gatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      routeStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		out.Rules = append(out.Rules, ir.StreamRule{
			Name:        stringValue((*string)(rule.Name)),
			Matches:     []ir.StreamMatch{{}},
			BackendRefs: backendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
		})
	}

	return out
}

func translateTLSRoute(route gatewayv1alpha2.TLSRoute) ir.StreamRoute {
	return translateTLSRouteWithDefaultGateways(route, nil)
}

func translateTLSRouteWithDefaultGateways(route gatewayv1alpha2.TLSRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gwapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "TLS",
		ParentRefs:  gatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      routeStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		streamRule := ir.StreamRule{
			Name:        stringValue((*string)(rule.Name)),
			BackendRefs: backendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
		}
		for _, hostname := range route.Spec.Hostnames {
			for _, mode := range tlsRouteModesForHostname(route.Namespace, parentRefs, string(hostname), defaultGateways) {
				streamRule.Matches = append(streamRule.Matches, ir.StreamMatch{
					SNIHostname: string(hostname),
					Mode:        mode,
				})
			}
		}
		if len(streamRule.Matches) == 0 {
			for _, mode := range tlsRouteModesForHostname(route.Namespace, parentRefs, "", defaultGateways) {
				streamRule.Matches = append(streamRule.Matches, ir.StreamMatch{
					Mode: mode,
				})
			}
		}
		out.Rules = append(out.Rules, streamRule)
	}

	return out
}

func tlsRouteModesForHostname(
	routeNamespace string,
	parentRefs []gatewayv1.ParentReference,
	hostname string,
	gateways []gatewayv1.Gateway,
) []ir.TlsRouteMode {
	modes := make(map[ir.TlsRouteMode]struct{}, 2)
	gatewayByName := make(map[string]gatewayv1.Gateway, len(gateways))
	for _, gw := range gateways {
		gatewayByName[gw.Namespace+"/"+gw.Name] = gw
	}
	for _, ref := range parentRefs {
		if stringValue(ref.Group) != "" && stringValue(ref.Group) != gatewayv1.GroupName {
			continue
		}
		if stringValue(ref.Kind) != "" && stringValue(ref.Kind) != "Gateway" {
			continue
		}
		key := namespaceOrDefault(ref.Namespace, routeNamespace) + "/" + string(ref.Name)
		gw, ok := gatewayByName[key]
		if !ok {
			continue
		}
		for _, listener := range gwapi.EffectiveListeners(gw) {
			if ref.SectionName != nil && string(*ref.SectionName) != "" &&
				string(*ref.SectionName) != string(listener.Name) {
				continue
			}
			if listener.Protocol != gatewayv1.TLSProtocolType {
				continue
			}
			if !tlsListenerHostnameIntersectsRoute(listener, hostname) {
				continue
			}
			modes[tlsRouteModeForListener(listener)] = struct{}{}
		}
	}
	if len(modes) == 0 {
		return nil
	}

	out := make([]ir.TlsRouteMode, 0, len(modes))
	if _, ok := modes[ir.TlsRouteModePassthrough]; ok {
		out = append(out, ir.TlsRouteModePassthrough)
	}
	if _, ok := modes[ir.TlsRouteModeTerminate]; ok {
		out = append(out, ir.TlsRouteModeTerminate)
	}
	return out
}

func tlsListenerHostnameIntersectsRoute(listener gatewayv1.Listener, routeHostname string) bool {
	if listener.Hostname == nil || routeHostname == "" {
		return true
	}
	return attachmentHostnamesIntersect(string(*listener.Hostname), routeHostname)
}

func tlsRouteModeForListener(listener gatewayv1.Listener) ir.TlsRouteMode {
	if listener.TLS != nil && listener.TLS.Mode != nil {
		if *listener.TLS.Mode == gatewayv1.TLSModePassthrough {
			return ir.TlsRouteModePassthrough
		}
		if *listener.TLS.Mode == gatewayv1.TLSModeTerminate {
			return ir.TlsRouteModeTerminate
		}
	}
	return ir.TlsRouteModeTerminate
}

func httpRouteRetry(retry *gatewayv1.HTTPRouteRetry) *ir.RetryPolicy {
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
