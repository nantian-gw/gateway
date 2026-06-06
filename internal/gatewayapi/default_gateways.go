package gatewayapi

import (
	"strconv"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	GatewayConditionDefaultGateway = "DefaultGateway"
	GatewayReasonDefaultGateway    = "DefaultGateway"
)

func UsesDefaultGateways(scope gatewayv1.GatewayDefaultScope) bool {
	return scope != "" && scope != gatewayv1.GatewayDefaultScopeNone
}

func GatewayMatchesDefaultScope(gateway gatewayv1.Gateway, scope gatewayv1.GatewayDefaultScope) bool {
	return UsesDefaultGateways(scope) && gateway.Spec.DefaultScope == scope
}

func GatewayActsAsDefault(gateway gatewayv1.Gateway) bool {
	return UsesDefaultGateways(gateway.Spec.DefaultScope)
}

func DefaultGatewayParentRefs(
	parentRefs []gatewayv1.ParentReference,
	routeNamespace string,
	scope gatewayv1.GatewayDefaultScope,
	gateways []gatewayv1.Gateway,
) []gatewayv1.ParentReference {
	out := append([]gatewayv1.ParentReference(nil), parentRefs...)
	if !UsesDefaultGateways(scope) {
		return out
	}

	seen := make(map[string]struct{}, len(out)+len(gateways))
	for _, ref := range out {
		seen[parentRefKey(ref, routeNamespace)] = struct{}{}
	}

	for _, gateway := range gateways {
		if !GatewayMatchesDefaultScope(gateway, scope) {
			continue
		}
		ref := defaultGatewayParentRef(gateway, routeNamespace)
		key := parentRefKey(ref, routeNamespace)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}

	return out
}

func defaultGatewayParentRef(gateway gatewayv1.Gateway, routeNamespace string) gatewayv1.ParentReference {
	group := gatewayv1.Group(gatewayv1.GroupName)
	kind := gatewayv1.Kind("Gateway")
	ref := gatewayv1.ParentReference{
		Group: &group,
		Kind:  &kind,
		Name:  gatewayv1.ObjectName(gateway.Name),
	}
	if gateway.Namespace != "" && gateway.Namespace != routeNamespace {
		namespace := gatewayv1.Namespace(gateway.Namespace)
		ref.Namespace = &namespace
	}
	return ref
}

func parentRefKey(ref gatewayv1.ParentReference, defaultNamespace string) string {
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if group == "" && kind == "Gateway" {
		group = gatewayv1.GroupName
	}
	namespace := defaultNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		namespace = string(*ref.Namespace)
	}
	sectionName := ""
	if ref.SectionName != nil {
		sectionName = string(*ref.SectionName)
	}
	port := ""
	if ref.Port != nil {
		port = strconv.Itoa(int(*ref.Port))
	}
	return group + "/" + kind + "/" + namespace + "/" + string(ref.Name) + "/" + sectionName + "/" + port
}
