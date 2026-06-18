package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extensionfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func evaluateRoute(state *clusterState, route routeInput) []routeParentEvaluation {
	parentRefs := routeEffectiveParentRefs(state, route)
	if len(parentRefs) == 0 {
		return nil
	}

	resolution := evaluateResolvedRefs(state, route)
	out := make([]routeParentEvaluation, 0, len(parentRefs))
	for _, parentRef := range parentRefs {
		eval, ok := evaluateParentRef(state, route, parentRef, resolution)
		if !ok {
			continue
		}
		out = append(out, eval)
	}
	return out
}

func routeEffectiveParentRefs(state *clusterState, route routeInput) []gatewayv1.ParentReference {
	return gatewayapi.DefaultGatewayParentRefs(
		route.parentRefs,
		route.namespace,
		route.defaultGatewayScope,
		state.managedGateways,
	)
}

func evaluateParentRef(
	state *clusterState,
	route routeInput,
	parentRef gatewayv1.ParentReference,
	resolution routeResolutionEvaluation,
) (routeParentEvaluation, bool) {
	normalizedParentRef := normalizeParentRef(route.namespace, parentRef)
	if isServiceParentRef(parentRef) {
		return evaluateServiceParentRef(state, route, parentRef, normalizedParentRef, resolution), true
	}

	if isListenerSetParentRef(parentRef) {
		return evaluateListenerSetParentRef(state, route, parentRef, normalizedParentRef, resolution)
	}

	gatewayNamespace := namespaceOrDefault(parentRef.Namespace, route.namespace)
	gatewayKey := namespacedName(gatewayNamespace, string(parentRef.Name))
	gateway, ok := state.managedGatewayByKey[gatewayKey]
	if !ok {
		return routeParentEvaluation{}, false
	}

	accepted := conditionSpec{
		Type:               string(gatewayv1.RouteConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gatewayv1.RouteReasonNoMatchingParent),
		Message:            "No matching parent was found for this route",
		ObservedGeneration: route.generation,
	}
	matchedListeners := make([]listenerKey, 0)

	candidates := candidateListeners(state, gateway, parentRef)
	if len(candidates) == 0 {
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingParent)
	} else {
		allowedListeners := make([]gatewayv1.Listener, 0, len(candidates))
		for _, listener := range candidates {
			policy := buildListenerPolicy(listener)
			if !listenerAllowsRoute(policy, route.kind, gateway.Namespace, route.namespace, state.namespaceByName[route.namespace]) {
				continue
			}
			allowedListeners = append(allowedListeners, listener)
		}

		switch {
		case len(allowedListeners) == 0:
			accepted.Reason = string(gatewayv1.RouteReasonNotAllowedByListeners)
			accepted.Message = "Parent listeners do not allow this route"
		default:
			for _, listener := range allowedListeners {
				if !listenerMatchesHostnames(listener, route.hostnames) {
					continue
				}
				matchedListeners = append(matchedListeners, listenerKey{
					gatewayNamespace: gateway.Namespace,
					gatewayName:      gateway.Name,
					listenerName:     listener.Name,
				})
			}

			if len(matchedListeners) == 0 {
				accepted.Reason = string(gatewayv1.RouteReasonNoMatchingListenerHostname)
				accepted.Message = "Route hostnames do not intersect with parent listener hostnames"
			} else {
				accepted.Status = metav1.ConditionTrue
				accepted.Reason = string(gatewayv1.RouteReasonAccepted)
				accepted.Message = "Route is accepted by nantian-gw"
			}
		}
	}
	accepted = applyRouteAcceptedError(route, accepted)

	return routeParentEvaluation{
		parentRef:         normalizedParentRef,
		controllerName:    gatewayv1.GatewayController(state.controllerName),
		acceptedCondition: accepted,
		resolvedCondition: resolution.resolvedCondition,
		extraConditions:   append([]conditionSpec(nil), resolution.extraConditions...),
		matchedListeners:  matchedListeners,
	}, true
}

func evaluateServiceParentRef(
	state *clusterState,
	route routeInput,
	parentRef gatewayv1.ParentReference,
	normalizedParentRef gatewayv1.ParentReference,
	resolution routeResolutionEvaluation,
) routeParentEvaluation {
	serviceNamespace := namespaceOrDefault(parentRef.Namespace, route.namespace)
	service, ok := state.serviceByKey[namespacedName(serviceNamespace, string(parentRef.Name))]

	accepted := conditionSpec{
		Type:               string(gatewayv1.RouteConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gatewayv1.RouteReasonNoMatchingParent),
		Message:            "No matching parent was found for this route",
		ObservedGeneration: route.generation,
	}
	if ok && serviceParentPortMatches(service, parentRef) {
		accepted.Status = metav1.ConditionTrue
		accepted.Reason = string(gatewayv1.RouteReasonAccepted)
		accepted.Message = "Route is accepted by nantian-gw"
	}
	accepted = applyRouteAcceptedError(route, accepted)

	return routeParentEvaluation{
		parentRef:         normalizedParentRef,
		controllerName:    gatewayv1.GatewayController(state.controllerName),
		acceptedCondition: accepted,
		resolvedCondition: resolution.resolvedCondition,
		extraConditions:   append([]conditionSpec(nil), resolution.extraConditions...),
	}
}

func applyRouteAcceptedError(route routeInput, accepted conditionSpec) conditionSpec {
	if route.acceptedErrorMessage == "" || accepted.Status != metav1.ConditionTrue {
		return accepted
	}
	accepted.Status = metav1.ConditionFalse
	accepted.Reason = string(gatewayv1.RouteReasonUnsupportedValue)
	accepted.Message = route.acceptedErrorMessage
	return accepted
}

func evaluateResolvedRefs(state *clusterState, route routeInput) routeResolutionEvaluation {
	result := routeResolutionEvaluation{
		resolvedCondition: conditionSpec{
			Type:               string(gatewayv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.RouteReasonResolvedRefs),
			Message:            "Route references are resolved",
			ObservedGeneration: route.generation,
		},
	}

	if route.partiallyInvalidErrorMessage != "" {
		result.extraConditions = append(result.extraConditions, conditionSpec{
			Type:               string(gatewayv1.RouteConditionPartiallyInvalid),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.RouteReasonUnsupportedValue),
			Message:            route.partiallyInvalidErrorMessage,
			ObservedGeneration: route.generation,
		})
	}

	if route.resolvedRefsErrorMessage != "" {
		result.resolvedCondition.Status = metav1.ConditionFalse
		result.resolvedCondition.Reason = string(gatewayv1.RouteReasonUnsupportedValue)
		result.resolvedCondition.Message = route.resolvedRefsErrorMessage
		return result
	}
	if target, ok := routeExtensionTarget(route.kind); ok {
		resolver := extensionfilter.NewResolver(state.configMaps)
		for _, ref := range route.extensionRefs {
			resolved := resolver.Resolve(ref, target)
			if resolved.Resolved {
				continue
			}
			result.resolvedCondition.Status = metav1.ConditionFalse
			result.resolvedCondition.Reason = resolved.Reason
			result.resolvedCondition.Message = resolved.Message
			return result
		}
	}

	for _, backend := range route.backends {
		targetGroup := backend.Group
		if targetGroup != "" && targetGroup != mcsv1alpha1.GroupName {
			result.resolvedCondition.Status = metav1.ConditionFalse
			result.resolvedCondition.Reason = string(gatewayv1.RouteReasonInvalidKind)
			result.resolvedCondition.Message = "BackendRef group is not supported"
			return result
		}

		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			result.resolvedCondition.Status = metav1.ConditionFalse
			result.resolvedCondition.Reason = string(gatewayv1.RouteReasonInvalidKind)
			result.resolvedCondition.Message = "BackendRef kind is not supported"
			return result
		}

		targetNamespace := backend.Namespace
		if targetNamespace == "" {
			targetNamespace = route.namespace
		}
		allowCrossNamespaceRefs := mesh.RouteUsesOnlyServiceParents(routeEffectiveParentRefs(state, route), route.namespace)
		if targetNamespace != route.namespace && !allowCrossNamespaceRefs && !referenceGranted(
			state.referenceGrants,
			targetNamespace,
			gatewayv1beta1.ReferenceGrantFrom{
				Group:     gatewayv1beta1.Group(routeGroupForKind(route.kind)),
				Kind:      gatewayv1beta1.Kind(route.kind),
				Namespace: gatewayv1beta1.Namespace(route.namespace),
			},
			gatewayv1beta1.ReferenceGrantTo{
				Group: gatewayv1beta1.Group(targetGroup),
				Kind:  gatewayv1beta1.Kind(targetKind),
				Name:  objectNamePtr(backend.Name),
			},
		) {
			result.resolvedCondition.Status = metav1.ConditionFalse
			result.resolvedCondition.Reason = string(gatewayv1.RouteReasonRefNotPermitted)
			result.resolvedCondition.Message = "Cross-namespace BackendRef is not permitted"
			return result
		}

		switch targetKind {
		case "Service":
			service, ok := state.serviceByKey[namespacedName(targetNamespace, backend.Name)]
			if !ok || !servicePortExists(service, backend.Port) {
				result.resolvedCondition.Status = metav1.ConditionFalse
				result.resolvedCondition.Reason = string(gatewayv1.RouteReasonBackendNotFound)
				result.resolvedCondition.Message = "BackendRef does not point to an existing Service"
				return result
			}
		case "ServiceImport":
			serviceImport, ok := state.serviceImportByKey[namespacedName(targetNamespace, backend.Name)]
			if !ok || !serviceImportPortExists(serviceImport, backend.Port) {
				result.resolvedCondition.Status = metav1.ConditionFalse
				result.resolvedCondition.Reason = string(gatewayv1.RouteReasonBackendNotFound)
				result.resolvedCondition.Message = "BackendRef does not point to an existing ServiceImport"
				return result
			}
		}
	}

	return result
}

func recordAttachments(attachments map[listenerKey]routeAttachmentSet, routeKey client.ObjectKey, evals []routeParentEvaluation) {
	for _, eval := range evals {
		if eval.acceptedCondition.Status != metav1.ConditionTrue {
			continue
		}
		for _, listener := range eval.matchedListeners {
			if attachments[listener] == nil {
				attachments[listener] = make(routeAttachmentSet)
			}
			attachments[listener][routeKey] = struct{}{}
		}
	}
}

func isListenerSetParentRef(parentRef gatewayv1.ParentReference) bool {
	return parentRef.Group != nil && string(*parentRef.Group) == gatewayv1.GroupName &&
		parentRef.Kind != nil && string(*parentRef.Kind) == "ListenerSet"
}

func evaluateListenerSetParentRef(
	state *clusterState,
	route routeInput,
	parentRef gatewayv1.ParentReference,
	normalizedParentRef gatewayv1.ParentReference,
	resolution routeResolutionEvaluation,
) (routeParentEvaluation, bool) {
	lsNamespace := namespaceOrDefault(parentRef.Namespace, route.namespace)
	lsKey := namespacedName(lsNamespace, string(parentRef.Name))
	ls, ok := state.listenerSetByKey[lsKey]
	if !ok {
		return routeParentEvaluation{}, false
	}

	gwKey := listenerSetParentGatewayKey(ls)
	gateway, ok := state.managedGatewayByKey[gwKey]
	if !ok {
		return routeParentEvaluation{}, false
	}

	accepted := conditionSpec{
		Type:               string(gatewayv1.RouteConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gatewayv1.RouteReasonNoMatchingParent),
		Message:            "No matching parent was found for this route",
		ObservedGeneration: route.generation,
	}
	matchedListeners := make([]listenerKey, 0)

	candidates := listenerSetCandidateListeners(ls, parentRef)
	if len(candidates) == 0 {
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingParent)
	} else {
		allowedListeners := make([]gatewayv1.Listener, 0, len(candidates))
		for _, listener := range candidates {
			policy := buildListenerPolicy(listener)
			if !listenerAllowsRoute(policy, route.kind, ls.Namespace, route.namespace, state.namespaceByName[route.namespace]) {
				continue
			}
			allowedListeners = append(allowedListeners, listener)
		}

		switch {
		case len(allowedListeners) == 0:
			accepted.Reason = string(gatewayv1.RouteReasonNotAllowedByListeners)
			accepted.Message = "Parent listeners do not allow this route"
		default:
			for _, listener := range allowedListeners {
				if !listenerMatchesHostnames(listener, route.hostnames) {
					continue
				}
				matchedListeners = append(matchedListeners, listenerKey{
					gatewayNamespace: gateway.Namespace,
					gatewayName:      gateway.Name,
					listenerName:     listener.Name,
				})
			}

			if len(matchedListeners) == 0 {
				accepted.Reason = string(gatewayv1.RouteReasonNoMatchingListenerHostname)
				accepted.Message = "Route hostnames do not intersect with parent listener hostnames"
			} else {
				accepted.Status = metav1.ConditionTrue
				accepted.Reason = string(gatewayv1.RouteReasonAccepted)
				accepted.Message = "Route is accepted by nantian-gw"
			}
		}
	}
	accepted = applyRouteAcceptedError(route, accepted)

	return routeParentEvaluation{
		parentRef:         normalizedParentRef,
		controllerName:    gatewayv1.GatewayController(state.controllerName),
		acceptedCondition: accepted,
		resolvedCondition: resolution.resolvedCondition,
		extraConditions:   resolution.extraConditions,
		matchedListeners:  matchedListeners,
	}, true
}

func listenerSetCandidateListeners(ls gatewayv1.ListenerSet, parentRef gatewayv1.ParentReference) []gatewayv1.Listener {
	if parentRef.SectionName != nil {
		for _, entry := range ls.Spec.Listeners {
			if entry.Name == *parentRef.SectionName {
				return []gatewayv1.Listener{listenerEntryToInternalListener(entry, ls)}
			}
		}
		return nil
	}
	out := make([]gatewayv1.Listener, 0, len(ls.Spec.Listeners))
	for _, entry := range ls.Spec.Listeners {
		out = append(out, listenerEntryToInternalListener(entry, ls))
	}
	return out
}
