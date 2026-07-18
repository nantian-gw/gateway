package routes

import (
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func TranslateTCPRoute(route gatewayv1alpha2.TCPRoute) ir.StreamRoute {
	return TranslateTCPRouteWithDefaultGateways(route, nil)
}

func TranslateTCPRouteWithDefaultGateways(route gatewayv1alpha2.TCPRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "TCP",
		ParentRefs:  shared.GatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      shared.RouteStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		out.Rules = append(out.Rules, ir.StreamRule{
			Name:        shared.StringValue((*string)(rule.Name)),
			Matches:     []ir.StreamMatch{{}},
			BackendRefs: BackendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
		})
	}

	return out
}

func TranslateUDPRoute(route gatewayv1alpha2.UDPRoute) ir.StreamRoute {
	return TranslateUDPRouteWithDefaultGateways(route, nil)
}

func TranslateUDPRouteWithDefaultGateways(route gatewayv1alpha2.UDPRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "UDP",
		ParentRefs:  shared.GatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      shared.RouteStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		out.Rules = append(out.Rules, ir.StreamRule{
			Name:        shared.StringValue((*string)(rule.Name)),
			Matches:     []ir.StreamMatch{{}},
			BackendRefs: BackendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
		})
	}

	return out
}

func TranslateTLSRoute(route gatewayv1alpha2.TLSRoute) ir.StreamRoute {
	return TranslateTLSRouteWithDefaultGateways(route, nil)
}

func TranslateTLSRouteWithDefaultGateways(route gatewayv1alpha2.TLSRoute, defaultGateways []gatewayv1.Gateway) ir.StreamRoute {
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.Spec.ParentRefs,
		route.Namespace,
		route.Spec.UseDefaultGateways,
		defaultGateways,
	)
	out := ir.StreamRoute{
		Name:        route.Name,
		Namespace:   route.Namespace,
		Kind:        "TLS",
		ParentRefs:  shared.GatewayParents(parentRefs, route.Namespace),
		Labels:      route.Labels,
		Annotations: route.Annotations,
		Status:      shared.RouteStatusSummary(route.Status.Parents, route.Namespace),
	}

	for _, rule := range route.Spec.Rules {
		streamRule := ir.StreamRule{
			Name:        shared.StringValue((*string)(rule.Name)),
			BackendRefs: BackendRefsFromRouteRule(rule.BackendRefs, route.Namespace),
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
		if shared.StringValue(ref.Group) != "" && shared.StringValue(ref.Group) != gatewayv1.GroupName {
			continue
		}
		if shared.StringValue(ref.Kind) != "" && shared.StringValue(ref.Kind) != "Gateway" {
			continue
		}
		key := shared.NamespaceOrDefault(ref.Namespace, routeNamespace) + "/" + string(ref.Name)
		gw, ok := gatewayByName[key]
		if !ok {
			continue
		}
		for _, listener := range gatewayapi.EffectiveListeners(gw) {
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
	return routeHostnamesIntersect(string(*listener.Hostname), routeHostname)
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

// routeHostnamesIntersect is a copy of the attachment hostname intersection logic
// duplicated here to avoid a circular import between the routes sub-package and
// the parent translator package.
func routeHostnamesIntersect(a, b string) bool {
	a = normalizeRouteHostname(a)
	b = normalizeRouteHostname(b)

	if a == "*" || b == "*" {
		return true
	}

	aWildcard, aSuffix := routeWildcardSuffix(a)
	bWildcard, bSuffix := routeWildcardSuffix(b)

	switch {
	case !aWildcard && !bWildcard:
		return a == b
	case aWildcard && !bWildcard:
		return routeHostnameMatchesPattern(a, b)
	case !aWildcard && bWildcard:
		return routeHostnameMatchesPattern(b, a)
	default:
		return aSuffix == bSuffix ||
			strings.HasSuffix(aSuffix, "."+bSuffix) ||
			strings.HasSuffix(bSuffix, "."+aSuffix)
	}
}

func routeHostnameMatchesPattern(pattern, host string) bool {
	pattern = normalizeRouteHostname(pattern)
	host = normalizeRouteHostname(host)
	if !strings.HasPrefix(pattern, "*.") {
		return pattern == host
	}

	suffix := strings.TrimPrefix(pattern, "*.")
	return host != suffix && strings.HasSuffix(host, "."+suffix)
}

func routeWildcardSuffix(host string) (bool, string) {
	return strings.HasPrefix(host, "*."), strings.TrimPrefix(host, "*.")
}

func normalizeRouteHostname(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
