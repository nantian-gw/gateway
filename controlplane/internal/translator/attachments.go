package translator

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
	"github.com/nantian-gw/gateway/controlplane/internal/mesh"
)

type attachmentRouteKind string

const (
	attachmentRouteKindHTTP attachmentRouteKind = "HTTPRoute"
	attachmentRouteKindGRPC attachmentRouteKind = "GRPCRoute"
	attachmentRouteKindTCP  attachmentRouteKind = "TCPRoute"
	attachmentRouteKindUDP  attachmentRouteKind = "UDPRoute"
	attachmentRouteKindTLS  attachmentRouteKind = "TLSRoute"
)

type attachmentPolicy struct {
	supportedKinds []gatewayv1.RouteGroupKind
	namespaceMode  gatewayv1.FromNamespaces
	selector       labels.Selector
}

func attachRoutes(snapshot *ir.Snapshot, gateways []gatewayv1.Gateway, namespaces []corev1.Namespace, listenerSets []gatewayv1.ListenerSet) {
	namespaceByName := make(map[string]corev1.Namespace, len(namespaces))
	for _, namespace := range namespaces {
		namespaceByName[namespace.Name] = namespace
	}

	gatewayByKey := make(map[string]gatewayv1.Gateway, len(gateways))
	for _, gateway := range gateways {
		gatewayByKey[gateway.Namespace+"/"+gateway.Name] = gateway
	}
	listenerSetByKey := make(map[string]gatewayv1.ListenerSet, len(listenerSets))
	listenerSetGateway := make(map[string]string)
	for _, ls := range listenerSets {
		key := ls.Namespace + "/" + ls.Name
		listenerSetByKey[key] = ls
		parentNS := namespaceOrDefault(ls.Spec.ParentRef.Namespace, ls.Namespace)
		listenerSetGateway[key] = parentNS + "/" + string(ls.Spec.ParentRef.Name)
	}
	serviceListeners := make(map[string][]ir.Listener)
	for _, listener := range snapshot.Listeners {
		if listener.Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
			continue
		}

		namespace := listener.Metadata[mesh.FrontendNamespaceMetadataKey]
		name := listener.Metadata[mesh.FrontendNameMetadataKey]
		if namespace == "" || name == "" {
			continue
		}
		serviceListeners[namespace+"/"+name] = append(
			serviceListeners[namespace+"/"+name],
			listener,
		)
	}

	attachments := make(map[string]map[string]struct{})

	for _, route := range snapshot.HTTPRoutes {
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentRouteKindHTTP,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range snapshot.GRPCRoutes {
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentRouteKindGRPC,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range snapshot.StreamRoutes {
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentKindForStreamRoute(route.Kind),
			streamRouteHostnames(route),
			route.ParentRefs,
			serviceListeners,
		)
	}

	for idx := range snapshot.Listeners {
		keys := attachments[snapshot.Listeners[idx].Name]
		if len(keys) == 0 {
			snapshot.Listeners[idx].AttachedRoutes = nil
			continue
		}

		attached := make([]string, 0, len(keys))
		for routeKey := range keys {
			attached = append(attached, routeKey)
		}
		sort.Strings(attached)
		snapshot.Listeners[idx].AttachedRoutes = attached
	}
}

func recordRouteAttachments(
	attachments map[string]map[string]struct{},
	gatewayByKey map[string]gatewayv1.Gateway,
	listenerSetByKey map[string]gatewayv1.ListenerSet,
	listenerSetGateway map[string]string,
	namespaceByName map[string]corev1.Namespace,
	routeNamespace string,
	routeName string,
	kind attachmentRouteKind,
	hostnames []string,
	parentRefs []ir.ParentRef,
	serviceListeners map[string][]ir.Listener,
) {
	routeKey := routeNamespace + "/" + routeName
	routeNamespaceObject := namespaceByName[routeNamespace]
	if routeNamespaceObject.Name == "" {
		routeNamespaceObject.Name = routeNamespace
	}

	for _, parentRef := range parentRefs {
		if isServiceParentRef(parentRef) {
			serviceNamespace := parentRef.Namespace
			if serviceNamespace == "" {
				serviceNamespace = routeNamespace
			}

			for _, listener := range serviceListeners[serviceNamespace+"/"+parentRef.Name] {
				if !serviceListenerMatchesParent(listener, parentRef) {
					continue
				}

				if attachments[listener.Name] == nil {
					attachments[listener.Name] = make(map[string]struct{})
				}
				attachments[listener.Name][routeKey] = struct{}{}
			}
			continue
		}

		if isListenerSetParentRef(parentRef) {
			gwKey, listeners := resolveListenerSetParent(
				parentRef, routeNamespace, gatewayByKey, listenerSetByKey, listenerSetGateway, namespaceByName,
			)
			if gwKey == "" {
				continue
			}
			for _, listener := range listeners {
				policy := buildAttachmentPolicy(listener)
				if !attachmentListenerAllowsRoute(policy, kind, gatewayByKey[gwKey].Namespace, routeNamespace, routeNamespaceObject) {
					continue
				}
				if !attachmentListenerMatchesHostnames(listener, hostnames) {
					continue
				}
				listenerKey := gwKey + "/" + string(listener.Name)
				if attachments[listenerKey] == nil {
					attachments[listenerKey] = make(map[string]struct{})
				}
				attachments[listenerKey][routeKey] = struct{}{}
			}
			continue
		}

		gatewayNamespace := parentRef.Namespace
		if gatewayNamespace == "" {
			gatewayNamespace = routeNamespace
		}

		gateway, ok := gatewayByKey[gatewayNamespace+"/"+parentRef.Name]
		if !ok {
			continue
		}

		for _, listener := range candidateAttachmentListeners(gateway, parentRef) {
			policy := buildAttachmentPolicy(listener)
			if !attachmentListenerAllowsRoute(policy, kind, gateway.Namespace, routeNamespace, routeNamespaceObject) {
				continue
			}
			if !attachmentListenerMatchesHostnames(listener, hostnames) {
				continue
			}

			listenerKey := gateway.Namespace + "/" + gateway.Name + "/" + string(listener.Name)
			if attachments[listenerKey] == nil {
				attachments[listenerKey] = make(map[string]struct{})
			}
			attachments[listenerKey][routeKey] = struct{}{}
		}
	}
}

func isServiceParentRef(parentRef ir.ParentRef) bool {
	return parentRef.Group == "" && parentRef.Kind == mesh.FrontendKindService
}

func isListenerSetParentRef(parentRef ir.ParentRef) bool {
	if parentRef.Group != "" && parentRef.Group != gatewayv1.GroupName {
		return false
	}
	return parentRef.Kind == "ListenerSet"
}

func resolveListenerSetParent(
	parentRef ir.ParentRef,
	routeNamespace string,
	gatewayByKey map[string]gatewayv1.Gateway,
	listenerSetByKey map[string]gatewayv1.ListenerSet,
	listenerSetGateway map[string]string,
	namespaces map[string]corev1.Namespace,
) (string, []gatewayv1.Listener) {
	lsNamespace := parentRef.Namespace
	if lsNamespace == "" {
		lsNamespace = routeNamespace
	}
	ls, ok := listenerSetByKey[lsNamespace+"/"+parentRef.Name]
	if !ok {
		return "", nil
	}
	gwKey, ok := listenerSetGateway[lsNamespace+"/"+parentRef.Name]
	if !ok {
		return "", nil
	}
	gateway, ok := gatewayByKey[gwKey]
	if !ok {
		return "", nil
	}
	if !gatewayAllowsListenerSet(gateway, ls, namespaces) {
		return "", nil
	}

	allListeners := mergeListenerSetListeners(gateway, gatewayapi.EffectiveListeners(gateway), []gatewayv1.ListenerSet{ls}, nil)
	out := make([]gatewayv1.Listener, 0)
	for _, l := range allListeners {
		if parentRef.SectionName != "" && string(l.Name) != parentRef.SectionName {
			continue
		}
		if parentRef.Port != 0 && uint32(l.Port) != parentRef.Port {
			continue
		}
		out = append(out, l)
	}
	return gwKey, out
}

func serviceListenerMatchesParent(listener ir.Listener, parentRef ir.ParentRef) bool {
	if parentRef.Port == 0 {
		return true
	}

	port, err := strconv.ParseUint(listener.Metadata[mesh.FrontendPortMetadataKey], 10, 32)
	if err != nil {
		return false
	}
	return uint32(port) == parentRef.Port
}

func candidateAttachmentListeners(gateway gatewayv1.Gateway, parentRef ir.ParentRef) []gatewayv1.Listener {
	listeners := gatewayapi.EffectiveListeners(gateway)
	out := make([]gatewayv1.Listener, 0, len(listeners))
	for _, listener := range listeners {
		if parentRef.SectionName != "" && string(listener.Name) != parentRef.SectionName {
			continue
		}
		if parentRef.Port != 0 && uint32(listener.Port) != parentRef.Port {
			continue
		}
		out = append(out, listener)
	}
	return out
}

func buildAttachmentPolicy(listener gatewayv1.Listener) attachmentPolicy {
	policy := attachmentPolicy{
		supportedKinds: attachmentSupportedKinds(listener.Protocol),
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
		return policy
	}

	policy.supportedKinds = policy.supportedKinds[:0]
	for _, routeGroupKind := range listener.AllowedRoutes.Kinds {
		group := stringValue(routeGroupKind.Group)
		if group != "" && group != gatewayv1.GroupName {
			continue
		}

		kind := attachmentRouteKind(routeGroupKind.Kind)
		if !attachmentKindAllowedByProtocol(listener.Protocol, kind) {
			continue
		}

		policy.supportedKinds = append(policy.supportedKinds, gatewayv1.RouteGroupKind{
			Group: attachmentGroupPtr(gatewayv1.GroupName),
			Kind:  gatewayv1.Kind(kind),
		})
	}

	return policy
}

func attachmentSupportedKinds(protocol gatewayv1.ProtocolType) []gatewayv1.RouteGroupKind {
	kinds := attachmentDefaultKinds(protocol)
	out := make([]gatewayv1.RouteGroupKind, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, gatewayv1.RouteGroupKind{
			Group: attachmentGroupPtr(gatewayv1.GroupName),
			Kind:  gatewayv1.Kind(kind),
		})
	}
	return out
}

func attachmentDefaultKinds(protocol gatewayv1.ProtocolType) []attachmentRouteKind {
	switch strings.ToUpper(string(protocol)) {
	case "HTTP":
		return []attachmentRouteKind{attachmentRouteKindGRPC, attachmentRouteKindHTTP}
	case "HTTP3":
		return []attachmentRouteKind{attachmentRouteKindHTTP}
	case "HTTPS":
		return []attachmentRouteKind{attachmentRouteKindGRPC, attachmentRouteKindHTTP}
	case "TLS":
		return []attachmentRouteKind{attachmentRouteKindTLS}
	case "TCP":
		return []attachmentRouteKind{attachmentRouteKindTCP}
	case "UDP":
		return []attachmentRouteKind{attachmentRouteKindUDP}
	default:
		return nil
	}
}

func attachmentKindAllowedByProtocol(protocol gatewayv1.ProtocolType, kind attachmentRouteKind) bool {
	for _, allowed := range attachmentDefaultKinds(protocol) {
		if allowed == kind {
			return true
		}
	}
	return false
}

func attachmentListenerAllowsRoute(
	policy attachmentPolicy,
	kind attachmentRouteKind,
	gatewayNamespace string,
	routeNamespace string,
	namespace corev1.Namespace,
) bool {
	if !attachmentListenerAllowsRouteKind(policy, kind) {
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

func attachmentListenerAllowsRouteKind(policy attachmentPolicy, kind attachmentRouteKind) bool {
	for _, item := range policy.supportedKinds {
		if attachmentRouteKind(item.Kind) == kind {
			return true
		}
	}
	return false
}

func attachmentListenerMatchesHostnames(listener gatewayv1.Listener, hostnames []string) bool {
	if listener.Hostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, routeHostname := range hostnames {
		if attachmentHostnamesIntersect(string(*listener.Hostname), routeHostname) {
			return true
		}
	}
	return false
}

func attachmentHostnamesIntersect(a, b string) bool {
	a = normalizeAttachmentHostname(a)
	b = normalizeAttachmentHostname(b)

	aWildcard, aSuffix := attachmentWildcardSuffix(a)
	bWildcard, bSuffix := attachmentWildcardSuffix(b)

	switch {
	case !aWildcard && !bWildcard:
		return a == b
	case aWildcard && !bWildcard:
		return attachmentHostnameMatchesPattern(a, b)
	case !aWildcard && bWildcard:
		return attachmentHostnameMatchesPattern(b, a)
	default:
		return aSuffix == bSuffix ||
			strings.HasSuffix(aSuffix, "."+bSuffix) ||
			strings.HasSuffix(bSuffix, "."+aSuffix)
	}
}

func attachmentHostnameMatchesPattern(pattern, host string) bool {
	pattern = normalizeAttachmentHostname(pattern)
	host = normalizeAttachmentHostname(host)
	if !strings.HasPrefix(pattern, "*.") {
		return pattern == host
	}

	suffix := strings.TrimPrefix(pattern, "*.")
	return host != suffix && strings.HasSuffix(host, "."+suffix)
}

func attachmentWildcardSuffix(host string) (bool, string) {
	return strings.HasPrefix(host, "*."), strings.TrimPrefix(host, "*.")
}

func normalizeAttachmentHostname(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func attachmentGroupPtr(group string) *gatewayv1.Group {
	value := gatewayv1.Group(group)
	return &value
}

func attachmentKindForStreamRoute(kind string) attachmentRouteKind {
	switch strings.ToUpper(kind) {
	case "ROUTE_KIND_TCP", "TCP":
		return attachmentRouteKindTCP
	case "ROUTE_KIND_UDP", "UDP":
		return attachmentRouteKindUDP
	case "ROUTE_KIND_TLS", "TLS":
		return attachmentRouteKindTLS
	default:
		return attachmentRouteKind(kind)
	}
}

func streamRouteHostnames(route ir.StreamRoute) []string {
	if attachmentKindForStreamRoute(route.Kind) != attachmentRouteKindTLS {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, rule := range route.Rules {
		for _, match := range rule.Matches {
			if match.SNIHostname == "" {
				continue
			}
			if _, ok := seen[match.SNIHostname]; ok {
				continue
			}
			seen[match.SNIHostname] = struct{}{}
			out = append(out, match.SNIHostname)
		}
	}
	return out
}
