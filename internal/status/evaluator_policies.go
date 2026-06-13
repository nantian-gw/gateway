package status

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/mesh"
)

func candidateListeners(state *clusterState, gateway gatewayv1.Gateway, parentRef gatewayv1.ParentReference) []gatewayv1.Listener {
	out := make([]gatewayv1.Listener, 0, len(gateway.Spec.Listeners))
	for _, listener := range gateway.Spec.Listeners {
		if parentRef.SectionName != nil && listener.Name != *parentRef.SectionName {
			continue
		}
		if parentRef.Port != nil && listener.Port != *parentRef.Port {
			continue
		}
		out = append(out, listener)
	}
	return out
}

func serviceParentPortMatches(service corev1.Service, parentRef gatewayv1.ParentReference) bool {
	if parentRef.Port == nil {
		return true
	}

	return servicePortExists(service, uint16(*parentRef.Port))
}

func buildListenerPolicy(listener gatewayv1.Listener) listenerPolicy {
	defaultKinds := defaultListenerKinds(listener.Protocol)
	allowedKinds := make(map[routeKind]struct{}, len(defaultKinds))
	for _, kind := range defaultKinds {
		allowedKinds[kind] = struct{}{}
	}

	policy := listenerPolicy{
		allowedKinds:   allowedKinds,
		supportedKinds: []gatewayv1.RouteGroupKind{},
		namespaceMode:  gatewayv1.NamespacesFromSame,
	}

	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil && listener.AllowedRoutes.Namespaces.From != nil {
		policy.namespaceMode = *listener.AllowedRoutes.Namespaces.From
	}
	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil && listener.AllowedRoutes.Namespaces.Selector != nil {
		if selector, err := metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector); err == nil {
			policy.selector = selector
		}
	}

	if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		policy.supportedKinds = supportedKindsForRoutes(defaultKinds)
		return policy
	}

	seen := make(map[routeKind]struct{})
	for _, routeGroupKind := range listener.AllowedRoutes.Kinds {
		group := stringOrEmpty(routeGroupKind.Group)
		kind := routeKind(routeGroupKind.Kind)
		if group != "" && group != gatewayGroup {
			policy.invalidKindRefs = true
			continue
		}
		if _, ok := allowedKinds[kind]; !ok {
			policy.invalidKindRefs = true
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		policy.supportedKinds = append(policy.supportedKinds, gatewayv1.RouteGroupKind{
			Group: groupPtr(gatewayGroup),
			Kind:  gatewayv1.Kind(kind),
		})
	}

	sort.Slice(policy.supportedKinds, func(i, j int) bool {
		return policy.supportedKinds[i].Kind < policy.supportedKinds[j].Kind
	})
	return policy
}

func supportedKindsForRoutes(kinds []routeKind) []gatewayv1.RouteGroupKind {
	out := make([]gatewayv1.RouteGroupKind, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, gatewayv1.RouteGroupKind{
			Group: groupPtr(gatewayGroup),
			Kind:  gatewayv1.Kind(kind),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}

func defaultListenerKinds(protocol gatewayv1.ProtocolType) []routeKind {
	switch strings.ToUpper(string(protocol)) {
	case "HTTP":
		return []routeKind{routeKindGRPC, routeKindHTTP}
	case "HTTP3":
		return []routeKind{routeKindHTTP}
	case "HTTPS":
		return []routeKind{routeKindGRPC, routeKindHTTP}
	case "TLS":
		return []routeKind{routeKindTLS}
	case "TCP":
		return []routeKind{routeKindTCP}
	case "UDP":
		return []routeKind{routeKindUDP}
	default:
		return nil
	}
}

func listenerAllowsRoute(
	policy listenerPolicy,
	kind routeKind,
	gatewayNamespace string,
	routeNamespace string,
	namespace corev1.Namespace,
) bool {
	if !listenerAllowsRouteKind(policy, kind) {
		return false
	}

	switch policy.namespaceMode {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSelector:
		if policy.selector == nil {
			return false
		}
		return policy.selector.Matches(labels.Set(namespace.Labels))
	case gatewayv1.NamespacesFromSame:
		return gatewayNamespace == routeNamespace
	default:
		return gatewayNamespace == routeNamespace
	}
}

func listenerAllowsRouteKind(policy listenerPolicy, kind routeKind) bool {
	for _, item := range policy.supportedKinds {
		if routeKind(item.Kind) == kind {
			return true
		}
	}
	return len(policy.supportedKinds) == 0 && false
}

func listenerMatchesHostnames(listener gatewayv1.Listener, hostnames []gatewayv1.Hostname) bool {
	if listener.Hostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, routeHostname := range hostnames {
		if hostnamesIntersect(string(*listener.Hostname), string(routeHostname)) {
			return true
		}
	}

	return false
}

func hostnamesIntersect(a, b string) bool {
	a = normalizeHostname(a)
	b = normalizeHostname(b)

	aWildcard, aSuffix := wildcardSuffix(a)
	bWildcard, bSuffix := wildcardSuffix(b)

	switch {
	case !aWildcard && !bWildcard:
		return a == b
	case aWildcard && !bWildcard:
		return hostnameMatchesPattern(a, b)
	case !aWildcard && bWildcard:
		return hostnameMatchesPattern(b, a)
	default:
		return wildcardIntersects(aSuffix, bSuffix)
	}
}

func normalizeParentRef(routeNamespace string, ref gatewayv1.ParentReference) gatewayv1.ParentReference {
	normalized := gatewayv1.ParentReference{
		Name:        ref.Name,
		SectionName: ref.SectionName,
		Port:        ref.Port,
	}
	if ref.Group != nil {
		group := gatewayv1.Group(*ref.Group)
		normalized.Group = &group
	} else {
		normalized.Group = groupPtr(gatewayGroup)
	}
	if ref.Kind != nil {
		kind := gatewayv1.Kind(*ref.Kind)
		normalized.Kind = &kind
	} else {
		normalized.Kind = kindPtr("Gateway")
	}
	if ref.Namespace != nil {
		namespace := gatewayv1.Namespace(*ref.Namespace)
		normalized.Namespace = &namespace
	} else if routeNamespace != "" {
		namespace := gatewayv1.Namespace(routeNamespace)
		normalized.Namespace = nil
		_ = namespace
	}
	return normalized
}

func isServiceParentRef(parentRef gatewayv1.ParentReference) bool {
	return stringOrEmpty(parentRef.Group) == "" &&
		stringOrEmpty(parentRef.Kind) == mesh.FrontendKindService
}

func referenceGranted(
	grants []gatewayv1beta1.ReferenceGrant,
	targetNamespace string,
	from gatewayv1beta1.ReferenceGrantFrom,
	to gatewayv1beta1.ReferenceGrantTo,
) bool {
	for _, grant := range grants {
		if grant.Namespace != targetNamespace {
			continue
		}
		if !referenceGrantFromAllowed(grant.Spec.From, from) {
			continue
		}
		if referenceGrantToAllowed(grant.Spec.To, to) {
			return true
		}
	}
	return false
}

func referenceGrantFromAllowed(items []gatewayv1beta1.ReferenceGrantFrom, expected gatewayv1beta1.ReferenceGrantFrom) bool {
	for _, item := range items {
		if item.Group == expected.Group && item.Kind == expected.Kind && item.Namespace == expected.Namespace {
			return true
		}
	}
	return false
}

func referenceGrantToAllowed(items []gatewayv1beta1.ReferenceGrantTo, expected gatewayv1beta1.ReferenceGrantTo) bool {
	for _, item := range items {
		if item.Group != expected.Group || item.Kind != expected.Kind {
			continue
		}
		if item.Name == nil || expected.Name == nil || *item.Name == *expected.Name {
			return true
		}
	}
	return false
}

func httpRouteBackends(route gatewayv1.HTTPRoute) []backendInput {
	out := make([]backendInput, 0)
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			out = append(out, backendInput{
				Group:     stringOrEmpty(backendRef.BackendRef.Group),
				Kind:      stringOrEmpty(backendRef.BackendRef.Kind),
				Namespace: namespaceOrDefault(backendRef.BackendRef.Namespace, route.Namespace),
				Name:      string(backendRef.BackendRef.Name),
				Port:      portOrZero(backendRef.BackendRef.Port),
			})
		}
	}
	return out
}

func grpcRouteBackends(route gatewayv1.GRPCRoute) []backendInput {
	out := make([]backendInput, 0)
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			out = append(out, backendInput{
				Group:     stringOrEmpty(backendRef.BackendRef.Group),
				Kind:      stringOrEmpty(backendRef.BackendRef.Kind),
				Namespace: namespaceOrDefault(backendRef.BackendRef.Namespace, route.Namespace),
				Name:      string(backendRef.BackendRef.Name),
				Port:      portOrZero(backendRef.BackendRef.Port),
			})
		}
	}
	return out
}

func tcpRouteBackends(route gatewayv1alpha2.TCPRoute) []backendInput {
	out := make([]backendInput, 0)
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			out = append(out, backendInput{
				Group:     stringOrEmpty(backendRef.Group),
				Kind:      stringOrEmpty(backendRef.Kind),
				Namespace: namespaceOrDefault(backendRef.Namespace, route.Namespace),
				Name:      string(backendRef.Name),
				Port:      portOrZero(backendRef.Port),
			})
		}
	}
	return out
}

func udpRouteBackends(route gatewayv1alpha2.UDPRoute) []backendInput {
	out := make([]backendInput, 0)
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			out = append(out, backendInput{
				Group:     stringOrEmpty(backendRef.Group),
				Kind:      stringOrEmpty(backendRef.Kind),
				Namespace: namespaceOrDefault(backendRef.Namespace, route.Namespace),
				Name:      string(backendRef.Name),
				Port:      portOrZero(backendRef.Port),
			})
		}
	}
	return out
}

func tlsRouteBackends(route gatewayv1alpha2.TLSRoute) []backendInput {
	out := make([]backendInput, 0)
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			out = append(out, backendInput{
				Group:     stringOrEmpty(backendRef.Group),
				Kind:      stringOrEmpty(backendRef.Kind),
				Namespace: namespaceOrDefault(backendRef.Namespace, route.Namespace),
				Name:      string(backendRef.Name),
				Port:      portOrZero(backendRef.Port),
			})
		}
	}
	return out
}

func portOrZero(port *gatewayv1.PortNumber) uint16 {
	if port == nil {
		return 0
	}
	return uint16(*port)
}

func backendKindForStatus(group, kind string) (string, bool) {
	if group == "" {
		if kind == "" || kind == "Service" {
			return "Service", true
		}
		return "", false
	}
	if group == mcsv1alpha1.GroupName && kind == "ServiceImport" {
		return "ServiceImport", true
	}
	return "", false
}

func serviceImportPortExists(serviceImport mcsv1alpha1.ServiceImport, port uint16) bool {
	if port == 0 {
		return true
	}
	for _, item := range serviceImport.Spec.Ports {
		if uint16(item.Port) == port {
			return true
		}
	}
	return false
}

func routeGroupForKind(kind routeKind) string {
	return gatewayGroup
}

func groupPtr(group string) *gatewayv1.Group {
	value := gatewayv1.Group(group)
	return &value
}

func kindPtr(kind string) *gatewayv1.Kind {
	value := gatewayv1.Kind(kind)
	return &value
}

func objectNamePtr(name string) *gatewayv1beta1.ObjectName {
	value := gatewayv1beta1.ObjectName(name)
	return &value
}

func wildcardSuffix(host string) (bool, string) {
	suffix := strings.TrimPrefix(host, "*.")
	return strings.HasPrefix(host, "*."), suffix
}

func hostnameMatchesPattern(pattern, host string) bool {
	pattern = normalizeHostname(pattern)
	host = normalizeHostname(host)
	if !strings.HasPrefix(pattern, "*.") {
		return pattern == host
	}
	suffix := strings.TrimPrefix(pattern, "*.")
	return host != suffix && strings.HasSuffix(host, "."+suffix)
}

func wildcardIntersects(aSuffix, bSuffix string) bool {
	return aSuffix == bSuffix ||
		strings.HasSuffix(aSuffix, "."+bSuffix) ||
		strings.HasSuffix(bSuffix, "."+aSuffix)
}

func normalizeHostname(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
