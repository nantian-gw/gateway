package status

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/extensionfilter"
)

func httpRouteExtensionRefs(route gatewayv1.HTTPRoute) []extensionfilter.Ref {
	out := make([]extensionfilter.Ref, 0)
	for _, rule := range route.Spec.Rules {
		out = append(out, httpFiltersExtensionRefs(route.Namespace, rule.Filters)...)
		for _, backendRef := range rule.BackendRefs {
			out = append(out, httpFiltersExtensionRefs(route.Namespace, backendRef.Filters)...)
		}
	}
	return out
}

func grpcRouteExtensionRefs(route gatewayv1.GRPCRoute) []extensionfilter.Ref {
	out := make([]extensionfilter.Ref, 0)
	for _, rule := range route.Spec.Rules {
		out = append(out, grpcFiltersExtensionRefs(route.Namespace, rule.Filters)...)
		for _, backendRef := range rule.BackendRefs {
			out = append(out, grpcFiltersExtensionRefs(route.Namespace, backendRef.Filters)...)
		}
	}
	return out
}

func httpFiltersExtensionRefs(namespace string, filters []gatewayv1.HTTPRouteFilter) []extensionfilter.Ref {
	out := make([]extensionfilter.Ref, 0, len(filters))
	for _, filter := range filters {
		if filter.Type != gatewayv1.HTTPRouteFilterExtensionRef {
			continue
		}
		out = append(out, extensionfilter.RefFromLocalRef(namespace, filter.ExtensionRef))
	}
	return out
}

func grpcFiltersExtensionRefs(namespace string, filters []gatewayv1.GRPCRouteFilter) []extensionfilter.Ref {
	out := make([]extensionfilter.Ref, 0, len(filters))
	for _, filter := range filters {
		if filter.Type != gatewayv1.GRPCRouteFilterExtensionRef {
			continue
		}
		out = append(out, extensionfilter.RefFromLocalRef(namespace, filter.ExtensionRef))
	}
	return out
}

func routeExtensionTarget(kind routeKind) (extensionfilter.Target, bool) {
	switch kind {
	case routeKindHTTP:
		return extensionfilter.TargetHTTP, true
	case routeKindGRPC:
		return extensionfilter.TargetGRPC, true
	default:
		return "", false
	}
}
