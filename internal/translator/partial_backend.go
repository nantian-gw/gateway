package translator

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func affectedBackendRefRoutes(
	current *ir.Snapshot,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	backendNamespaces []string,
) (affectedBackendRefRouteKeys, []ir.HTTPRoute, []ir.GRPCRoute, []ir.StreamRoute) {
	if current == nil {
		return affectedBackendRefRouteKeys{}, nil, nil, nil
	}

	serviceKeySet := objectKeyMap(serviceKeys)
	serviceImportKeySet := objectKeyMap(serviceImportKeys)
	namespaceSet := make(map[string]struct{}, len(backendNamespaces))
	for _, namespace := range backendNamespaces {
		if namespace == "" {
			continue
		}
		namespaceSet[namespace] = struct{}{}
	}

	var (
		routeKeys    affectedBackendRefRouteKeys
		httpRoutes   []ir.HTTPRoute
		grpcRoutes   []ir.GRPCRoute
		streamRoutes []ir.StreamRoute
	)

	for _, route := range current.HTTPRoutes {
		if !routeBackendRefsTouchAffectedBackends(
			httpRouteBackendRefs(route),
			serviceKeySet,
			serviceImportKeySet,
			namespaceSet,
		) {
			continue
		}
		routeKeys.http = append(routeKeys.http, client.ObjectKey{Namespace: route.Namespace, Name: route.Name})
		httpRoutes = append(httpRoutes, route)
	}
	for _, route := range current.GRPCRoutes {
		if !routeBackendRefsTouchAffectedBackends(
			grpcRouteBackendRefs(route),
			serviceKeySet,
			serviceImportKeySet,
			namespaceSet,
		) {
			continue
		}
		routeKeys.grpc = append(routeKeys.grpc, client.ObjectKey{Namespace: route.Namespace, Name: route.Name})
		grpcRoutes = append(grpcRoutes, route)
	}
	for _, route := range current.StreamRoutes {
		if !routeBackendRefsTouchAffectedBackends(
			streamRouteBackendRefs(route),
			serviceKeySet,
			serviceImportKeySet,
			namespaceSet,
		) {
			continue
		}
		switch route.Kind {
		case "TCP":
			routeKeys.tcp = append(routeKeys.tcp, client.ObjectKey{Namespace: route.Namespace, Name: route.Name})
		case "UDP":
			routeKeys.udp = append(routeKeys.udp, client.ObjectKey{Namespace: route.Namespace, Name: route.Name})
		case "TLS":
			routeKeys.tls = append(routeKeys.tls, client.ObjectKey{Namespace: route.Namespace, Name: route.Name})
		}
		streamRoutes = append(streamRoutes, route)
	}

	return routeKeys, httpRoutes, grpcRoutes, streamRoutes
}

func httpRouteBackendRefs(route ir.HTTPRoute) []ir.BackendRef {
	total := 0
	for _, rule := range route.Rules {
		total += len(rule.BackendRefs)
	}
	out := make([]ir.BackendRef, 0, total)
	for _, rule := range route.Rules {
		out = append(out, rule.BackendRefs...)
	}
	return out
}

func grpcRouteBackendRefs(route ir.GRPCRoute) []ir.BackendRef {
	total := 0
	for _, rule := range route.Rules {
		total += len(rule.BackendRefs)
	}
	out := make([]ir.BackendRef, 0, total)
	for _, rule := range route.Rules {
		out = append(out, rule.BackendRefs...)
	}
	return out
}

func streamRouteBackendRefs(route ir.StreamRoute) []ir.BackendRef {
	total := 0
	for _, rule := range route.Rules {
		total += len(rule.BackendRefs)
	}
	out := make([]ir.BackendRef, 0, total)
	for _, rule := range route.Rules {
		out = append(out, rule.BackendRefs...)
	}
	return out
}

func routeBackendRefsTouchAffectedBackends(
	backendRefs []ir.BackendRef,
	serviceKeySet map[string]client.ObjectKey,
	serviceImportKeySet map[string]client.ObjectKey,
	namespaceSet map[string]struct{},
) bool {
	for _, ref := range backendRefs {
		if ref.Name == "" || ref.Namespace == "" {
			continue
		}
		if _, ok := namespaceSet[ref.Namespace]; ok {
			if _, backendKindKnown := backends.BackendKindForRef(ref.Group, ref.Kind); backendKindKnown {
				return true
			}
		}

		key := shared.BackendObjectKey(ref.Namespace, ref.Name)
		switch kind, ok := backends.BackendKindForRef(ref.Group, ref.Kind); {
		case !ok:
			continue
		case kind == "Service":
			if _, exists := serviceKeySet[key]; exists {
				return true
			}
		case kind == "ServiceImport":
			if _, exists := serviceImportKeySet[key]; exists {
				return true
			}
		}
	}
	return false
}

func refreshHTTPRouteBackendRefs(routes []ir.HTTPRoute, annotator backends.BackendRefTranslator) {
	for i := range routes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(routes[i].ParentRefs)
		for j := range routes[i].Rules {
			routes[i].Rules[j].BackendRefs = refreshBackendRefs( //nolint:gosec
				routes[i].Rules[j].BackendRefs, //nolint:gosec
				routes[i].Namespace,            //nolint:gosec
				backends.RouteKindHTTP,
				allowCrossNamespaceRefs,
				annotator,
			)
		}
	}
}

func refreshGRPCRouteBackendRefs(routes []ir.GRPCRoute, annotator backends.BackendRefTranslator) {
	for i := range routes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(routes[i].ParentRefs)
		for j := range routes[i].Rules {
			routes[i].Rules[j].BackendRefs = refreshBackendRefs(
				routes[i].Rules[j].BackendRefs,
				routes[i].Namespace,
				backends.RouteKindGRPC,
				allowCrossNamespaceRefs,
				annotator,
			)
		}
	}
}

func refreshStreamRouteBackendRefs(routes []ir.StreamRoute, annotator backends.BackendRefTranslator) {
	for i := range routes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(routes[i].ParentRefs)
		kind := routeKindForStreamIR(routes[i].Kind)
		for j := range routes[i].Rules {
			routes[i].Rules[j].BackendRefs = refreshBackendRefs(
				routes[i].Rules[j].BackendRefs,
				routes[i].Namespace,
				kind,
				allowCrossNamespaceRefs,
				annotator,
			)
		}
	}
}

func refreshBackendRefs(
	refs []ir.BackendRef,
	routeNamespace string,
	routeKind backends.RouteKind,
	allowCrossNamespaceRefs bool,
	annotator backends.BackendRefTranslator,
) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, refreshBackendRef(ref, routeNamespace, routeKind, allowCrossNamespaceRefs, annotator))
	}
	return out
}

func refreshBackendRef(
	ref ir.BackendRef,
	routeNamespace string,
	routeKind backends.RouteKind,
	allowCrossNamespaceRefs bool,
	annotator backends.BackendRefTranslator,
) ir.BackendRef {
	metadata := annotator.BackendRefMetadata(routeNamespace, routeKind, allowCrossNamespaceRefs, ref)
	if len(metadata) != 0 {
		ref.Metadata = metadata
		return ref
	}
	if len(ref.Metadata) == 0 {
		return ref
	}

	cleaned := copyStringMap(ref.Metadata)
	delete(cleaned, backends.BackendRefMetaValid)
	delete(cleaned, backends.BackendRefMetaReason)
	if len(cleaned) == 0 {
		ref.Metadata = nil
		return ref
	}
	ref.Metadata = cleaned
	return ref
}

func routeKindForStreamIR(kind string) backends.RouteKind {
	switch kind {
	case "TCP":
		return backends.RouteKindTCP
	case "UDP":
		return backends.RouteKindUDP
	case "TLS":
		return backends.RouteKindTLS
	default:
		return backends.RouteKindTCP
	}
}
