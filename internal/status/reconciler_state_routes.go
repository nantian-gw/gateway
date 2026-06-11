package status

import (
	"context"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func loadHTTPRoutesForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
	options ...Options,
) ([]gatewayv1.HTTPRoute, error) {
	opts := normalizeOptions(options)
	serviceParentRoutes, scoped, err := listHTTPRoutesWithServiceParents(ctx, reader)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return serviceParentRoutes, nil
	}

	index := make(map[string]gatewayv1.HTTPRoute, len(serviceParentRoutes))
	for _, route := range serviceParentRoutes {
		index[namespacedName(route.Namespace, route.Name)] = route
	}

	for _, gateway := range managedGateways {
		routes, scoped, err := listHTTPRoutesForGatewayScoped(ctx, reader, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			return nil, err
		}
		if !scoped {
			return routes, nil
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	if opts.EnableExperimentalGateway {
		listenerSetRoutes, err := listHTTPRoutesWithListenerSetParents(ctx, reader)
		if err != nil {
			return nil, err
		}
		listenerSets, err := loadListenerSetsForState(ctx, reader, managedGateways)
		if err != nil {
			return nil, err
		}
		lsByKey := make(map[string]gatewayv1.ListenerSet, len(listenerSets))
		for _, ls := range listenerSets {
			lsByKey[namespacedName(ls.Namespace, ls.Name)] = ls
		}
		for _, route := range listenerSetRoutes {
			if routeHasListenerSetParentForManagedGateway(route.Spec.ParentRefs, route.Namespace, lsByKey, managedGateways) {
				index[namespacedName(route.Namespace, route.Name)] = route
			}
		}
	}

	if scopes := defaultGatewayScopes(managedGateways); len(scopes) > 0 {
		routes, err := listHTTPRoutesForDefaultScopes(ctx, reader, scopes)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	out := make([]gatewayv1.HTTPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func loadGRPCRoutesForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
) ([]gatewayv1.GRPCRoute, error) {
	serviceParentRoutes, scoped, err := listGRPCRoutesWithServiceParents(ctx, reader)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return serviceParentRoutes, nil
	}

	index := make(map[string]gatewayv1.GRPCRoute, len(serviceParentRoutes))
	for _, route := range serviceParentRoutes {
		index[namespacedName(route.Namespace, route.Name)] = route
	}

	for _, gateway := range managedGateways {
		routes, scoped, err := listGRPCRoutesForGatewayScoped(ctx, reader, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			return nil, err
		}
		if !scoped {
			return routes, nil
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}
	if scopes := defaultGatewayScopes(managedGateways); len(scopes) > 0 {
		routes, err := listGRPCRoutesForDefaultScopes(ctx, reader, scopes)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	out := make([]gatewayv1.GRPCRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func loadTCPRoutesForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
) ([]gatewayv1alpha2.TCPRoute, error) {
	serviceParentRoutes, scoped, err := listTCPRoutesWithServiceParents(ctx, reader)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return serviceParentRoutes, nil
	}

	index := make(map[string]gatewayv1alpha2.TCPRoute, len(serviceParentRoutes))
	for _, route := range serviceParentRoutes {
		index[namespacedName(route.Namespace, route.Name)] = route
	}

	for _, gateway := range managedGateways {
		routes, scoped, err := listTCPRoutesForGatewayScoped(ctx, reader, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			return nil, err
		}
		if !scoped {
			return routes, nil
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}
	if scopes := defaultGatewayScopes(managedGateways); len(scopes) > 0 {
		routes, err := listTCPRoutesForDefaultScopes(ctx, reader, scopes)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	out := make([]gatewayv1alpha2.TCPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func loadUDPRoutesForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
) ([]gatewayv1alpha2.UDPRoute, error) {
	serviceParentRoutes, scoped, err := listUDPRoutesWithServiceParents(ctx, reader)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return serviceParentRoutes, nil
	}

	index := make(map[string]gatewayv1alpha2.UDPRoute, len(serviceParentRoutes))
	for _, route := range serviceParentRoutes {
		index[namespacedName(route.Namespace, route.Name)] = route
	}

	for _, gateway := range managedGateways {
		routes, scoped, err := listUDPRoutesForGatewayScoped(ctx, reader, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			return nil, err
		}
		if !scoped {
			return routes, nil
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}
	if scopes := defaultGatewayScopes(managedGateways); len(scopes) > 0 {
		routes, err := listUDPRoutesForDefaultScopes(ctx, reader, scopes)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	out := make([]gatewayv1alpha2.UDPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func loadTLSRoutesForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
) ([]gatewayv1alpha2.TLSRoute, error) {
	serviceParentRoutes, scoped, err := listTLSRoutesWithServiceParents(ctx, reader)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return serviceParentRoutes, nil
	}

	index := make(map[string]gatewayv1alpha2.TLSRoute, len(serviceParentRoutes))
	for _, route := range serviceParentRoutes {
		index[namespacedName(route.Namespace, route.Name)] = route
	}

	for _, gateway := range managedGateways {
		routes, scoped, err := listTLSRoutesForGatewayScoped(ctx, reader, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			return nil, err
		}
		if !scoped {
			return routes, nil
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}
	if scopes := defaultGatewayScopes(managedGateways); len(scopes) > 0 {
		routes, err := listTLSRoutesForDefaultScopes(ctx, reader, scopes)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			index[namespacedName(route.Namespace, route.Name)] = route
		}
	}

	out := make([]gatewayv1alpha2.TLSRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func defaultGatewayScopes(gateways []gatewayv1.Gateway) map[gatewayv1.GatewayDefaultScope]struct{} {
	out := make(map[gatewayv1.GatewayDefaultScope]struct{})
	for _, gateway := range gateways {
		if gateway.Spec.DefaultScope == "" || gateway.Spec.DefaultScope == gatewayv1.GatewayDefaultScopeNone {
			continue
		}
		out[gateway.Spec.DefaultScope] = struct{}{}
	}
	return out
}

func listHTTPRoutesForDefaultScopes(ctx context.Context, reader client.Reader, scopes map[gatewayv1.GatewayDefaultScope]struct{}) ([]gatewayv1.HTTPRoute, error) {
	var routes gatewayv1.HTTPRouteList
	if err := reader.List(ctx, &routes); err != nil {
		return nil, err
	}
	return filterHTTPRoutesByDefaultScopes(routes.Items, scopes), nil
}

func listGRPCRoutesForDefaultScopes(ctx context.Context, reader client.Reader, scopes map[gatewayv1.GatewayDefaultScope]struct{}) ([]gatewayv1.GRPCRoute, error) {
	var routes gatewayv1.GRPCRouteList
	if err := reader.List(ctx, &routes); err != nil {
		return nil, err
	}
	return filterGRPCRoutesByDefaultScopes(routes.Items, scopes), nil
}

func listTCPRoutesForDefaultScopes(ctx context.Context, reader client.Reader, scopes map[gatewayv1.GatewayDefaultScope]struct{}) ([]gatewayv1alpha2.TCPRoute, error) {
	var routes gatewayv1alpha2.TCPRouteList
	if err := reader.List(ctx, &routes); err != nil {
		return nil, err
	}
	return filterTCPRoutesByDefaultScopes(routes.Items, scopes), nil
}

func listUDPRoutesForDefaultScopes(ctx context.Context, reader client.Reader, scopes map[gatewayv1.GatewayDefaultScope]struct{}) ([]gatewayv1alpha2.UDPRoute, error) {
	var routes gatewayv1alpha2.UDPRouteList
	if err := reader.List(ctx, &routes); err != nil {
		return nil, err
	}
	return filterUDPRoutesByDefaultScopes(routes.Items, scopes), nil
}

func listTLSRoutesForDefaultScopes(ctx context.Context, reader client.Reader, scopes map[gatewayv1.GatewayDefaultScope]struct{}) ([]gatewayv1alpha2.TLSRoute, error) {
	var routes gatewayv1alpha2.TLSRouteList
	if err := reader.List(ctx, &routes); err != nil {
		return nil, err
	}
	return filterTLSRoutesByDefaultScopes(routes.Items, scopes), nil
}

func filterHTTPRoutesByDefaultScope(items []gatewayv1.HTTPRoute, scope gatewayv1.GatewayDefaultScope) []gatewayv1.HTTPRoute {
	return filterHTTPRoutesByDefaultScopes(items, map[gatewayv1.GatewayDefaultScope]struct{}{scope: {}})
}

func filterGRPCRoutesByDefaultScope(items []gatewayv1.GRPCRoute, scope gatewayv1.GatewayDefaultScope) []gatewayv1.GRPCRoute {
	return filterGRPCRoutesByDefaultScopes(items, map[gatewayv1.GatewayDefaultScope]struct{}{scope: {}})
}

func filterTCPRoutesByDefaultScope(items []gatewayv1alpha2.TCPRoute, scope gatewayv1.GatewayDefaultScope) []gatewayv1alpha2.TCPRoute {
	return filterTCPRoutesByDefaultScopes(items, map[gatewayv1.GatewayDefaultScope]struct{}{scope: {}})
}

func filterUDPRoutesByDefaultScope(items []gatewayv1alpha2.UDPRoute, scope gatewayv1.GatewayDefaultScope) []gatewayv1alpha2.UDPRoute {
	return filterUDPRoutesByDefaultScopes(items, map[gatewayv1.GatewayDefaultScope]struct{}{scope: {}})
}

func filterTLSRoutesByDefaultScope(items []gatewayv1alpha2.TLSRoute, scope gatewayv1.GatewayDefaultScope) []gatewayv1alpha2.TLSRoute {
	return filterTLSRoutesByDefaultScopes(items, map[gatewayv1.GatewayDefaultScope]struct{}{scope: {}})
}

func filterHTTPRoutesByDefaultScopes(items []gatewayv1.HTTPRoute, scopes map[gatewayv1.GatewayDefaultScope]struct{}) []gatewayv1.HTTPRoute {
	out := make([]gatewayv1.HTTPRoute, 0)
	for _, route := range items {
		if _, ok := scopes[route.Spec.UseDefaultGateways]; ok {
			out = append(out, route)
		}
	}
	return out
}

func filterGRPCRoutesByDefaultScopes(items []gatewayv1.GRPCRoute, scopes map[gatewayv1.GatewayDefaultScope]struct{}) []gatewayv1.GRPCRoute {
	out := make([]gatewayv1.GRPCRoute, 0)
	for _, route := range items {
		if _, ok := scopes[route.Spec.UseDefaultGateways]; ok {
			out = append(out, route)
		}
	}
	return out
}

func filterTCPRoutesByDefaultScopes(items []gatewayv1alpha2.TCPRoute, scopes map[gatewayv1.GatewayDefaultScope]struct{}) []gatewayv1alpha2.TCPRoute {
	out := make([]gatewayv1alpha2.TCPRoute, 0)
	for _, route := range items {
		if _, ok := scopes[route.Spec.UseDefaultGateways]; ok {
			out = append(out, route)
		}
	}
	return out
}

func filterUDPRoutesByDefaultScopes(items []gatewayv1alpha2.UDPRoute, scopes map[gatewayv1.GatewayDefaultScope]struct{}) []gatewayv1alpha2.UDPRoute {
	out := make([]gatewayv1alpha2.UDPRoute, 0)
	for _, route := range items {
		if _, ok := scopes[route.Spec.UseDefaultGateways]; ok {
			out = append(out, route)
		}
	}
	return out
}

func filterTLSRoutesByDefaultScopes(items []gatewayv1alpha2.TLSRoute, scopes map[gatewayv1.GatewayDefaultScope]struct{}) []gatewayv1alpha2.TLSRoute {
	out := make([]gatewayv1alpha2.TLSRoute, 0)
	for _, route := range items {
		if _, ok := scopes[route.Spec.UseDefaultGateways]; ok {
			out = append(out, route)
		}
	}
	return out
}

func mergeHTTPRoutesByKey(left, right []gatewayv1.HTTPRoute) []gatewayv1.HTTPRoute {
	index := make(map[string]gatewayv1.HTTPRoute, len(left)+len(right))
	for _, route := range left {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	for _, route := range right {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	out := make([]gatewayv1.HTTPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func mergeGRPCRoutesByKey(left, right []gatewayv1.GRPCRoute) []gatewayv1.GRPCRoute {
	index := make(map[string]gatewayv1.GRPCRoute, len(left)+len(right))
	for _, route := range left {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	for _, route := range right {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	out := make([]gatewayv1.GRPCRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func mergeTCPRoutesByKey(left, right []gatewayv1alpha2.TCPRoute) []gatewayv1alpha2.TCPRoute {
	index := make(map[string]gatewayv1alpha2.TCPRoute, len(left)+len(right))
	for _, route := range left {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	for _, route := range right {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	out := make([]gatewayv1alpha2.TCPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func mergeUDPRoutesByKey(left, right []gatewayv1alpha2.UDPRoute) []gatewayv1alpha2.UDPRoute {
	index := make(map[string]gatewayv1alpha2.UDPRoute, len(left)+len(right))
	for _, route := range left {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	for _, route := range right {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	out := make([]gatewayv1alpha2.UDPRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func mergeTLSRoutesByKey(left, right []gatewayv1alpha2.TLSRoute) []gatewayv1alpha2.TLSRoute {
	index := make(map[string]gatewayv1alpha2.TLSRoute, len(left)+len(right))
	for _, route := range left {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	for _, route := range right {
		index[namespacedName(route.Namespace, route.Name)] = route
	}
	out := make([]gatewayv1alpha2.TLSRoute, 0, len(index))
	for _, route := range index {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func listHTTPRoutesWithListenerSetParents(ctx context.Context, reader client.Reader) ([]gatewayv1.HTTPRoute, error) {
	var routes gatewayv1.HTTPRouteList
	if err := reader.List(ctx, &routes, client.MatchingFields{
		statusHTTPRouteListenerSetParentIndex: statusListenerSetParentIndexMarker,
	}); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func routeHasListenerSetParentForManagedGateway(parentRefs []gatewayv1.ParentReference, routeNamespace string, lsByKey map[string]gatewayv1.ListenerSet, managedGateways []gatewayv1.Gateway) bool {
	gwSet := make(map[string]bool, len(managedGateways))
	for _, g := range managedGateways {
		gwSet[namespacedName(g.Namespace, g.Name)] = true
	}
	for _, ref := range parentRefs {
		if ref.Kind != nil && string(*ref.Kind) == "ListenerSet" && ref.Group != nil && string(*ref.Group) == gatewayv1.GroupName {
			ns := routeNamespace
			if ref.Namespace != nil && *ref.Namespace != "" {
				ns = string(*ref.Namespace)
			}
			lsKey := namespacedName(ns, string(ref.Name))
			ls, ok := lsByKey[lsKey]
			if !ok {
				continue
			}
			gwNs := ""
			if ls.Spec.ParentRef.Namespace != nil {
				gwNs = string(*ls.Spec.ParentRef.Namespace)
			}
			gwKey := namespacedName(gwNs, string(ls.Spec.ParentRef.Name))
			if gwSet[gwKey] {
				return true
			}
		}
	}
	return false
}
