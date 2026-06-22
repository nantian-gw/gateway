package status

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/extfilter"
)

func httpRouteExtensionRefs(route gatewayv1.HTTPRoute) []extfilter.Ref {
	out := make([]extfilter.Ref, 0)
	for _, rule := range route.Spec.Rules {
		out = append(out, httpFiltersExtensionRefs(route.Namespace, rule.Filters)...)
		for _, backendRef := range rule.BackendRefs {
			out = append(out, httpFiltersExtensionRefs(route.Namespace, backendRef.Filters)...)
		}
	}
	return out
}

func grpcRouteExtensionRefs(route gatewayv1.GRPCRoute) []extfilter.Ref {
	out := make([]extfilter.Ref, 0)
	for _, rule := range route.Spec.Rules {
		out = append(out, grpcFiltersExtensionRefs(route.Namespace, rule.Filters)...)
		for _, backendRef := range rule.BackendRefs {
			out = append(out, grpcFiltersExtensionRefs(route.Namespace, backendRef.Filters)...)
		}
	}
	return out
}

func httpFiltersExtensionRefs(namespace string, filters []gatewayv1.HTTPRouteFilter) []extfilter.Ref {
	out := make([]extfilter.Ref, 0, len(filters))
	for _, filter := range filters {
		if filter.Type != gatewayv1.HTTPRouteFilterExtensionRef {
			continue
		}
		out = append(out, extfilter.RefFromLocalRef(namespace, filter.ExtensionRef))
	}
	return out
}

func grpcFiltersExtensionRefs(namespace string, filters []gatewayv1.GRPCRouteFilter) []extfilter.Ref {
	out := make([]extfilter.Ref, 0, len(filters))
	for _, filter := range filters {
		if filter.Type != gatewayv1.GRPCRouteFilterExtensionRef {
			continue
		}
		out = append(out, extfilter.RefFromLocalRef(namespace, filter.ExtensionRef))
	}
	return out
}

func routeExtensionTarget(kind routeKind) (extfilter.Target, bool) {
	switch kind {
	case routeKindHTTP:
		return extfilter.TargetHTTP, true
	case routeKindGRPC:
		return extfilter.TargetGRPC, true
	default:
		return "", false
	}
}
