package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/mesh"
)

type preparedRouteInput struct {
	routeInput
	effectiveParentRefs []gatewayv1.ParentReference
	serviceParentOnly   bool
}

type routeEvaluationContext struct {
	state                  *clusterState
	gatewayListenerPolicy  map[listenerKey]listenerPolicy
	listenerSetEntryPolicy map[string]listenerPolicy
}

func newRouteEvaluationContext(state *clusterState) *routeEvaluationContext {
	return &routeEvaluationContext{
		state:                  state,
		gatewayListenerPolicy:  make(map[listenerKey]listenerPolicy),
		listenerSetEntryPolicy: make(map[string]listenerPolicy),
	}
}

func (c *routeEvaluationContext) prepareRoute(route routeInput) preparedRouteInput {
	parentRefs := routeEffectiveParentRefs(c.state, route)
	return preparedRouteInput{
		routeInput:          route,
		effectiveParentRefs: parentRefs,
		serviceParentOnly:   mesh.RouteUsesOnlyServiceParents(parentRefs, route.namespace),
	}
}

func (c *routeEvaluationContext) gatewayPolicy(gateway gatewayv1.Gateway, listener gatewayv1.Listener) listenerPolicy {
	key := listenerKey{
		gatewayNamespace: gateway.Namespace,
		gatewayName:      gateway.Name,
		listenerName:     listener.Name,
	}
	if policy, ok := c.gatewayListenerPolicy[key]; ok {
		return policy
	}
	policy := buildListenerPolicy(listener)
	c.gatewayListenerPolicy[key] = policy
	return policy
}

func (c *routeEvaluationContext) listenerSetPolicy(ls gatewayv1.ListenerSet, entry gatewayv1.ListenerEntry) listenerPolicy {
	key := namespacedName(ls.Namespace, ls.Name) + "/" + string(entry.Name)
	if policy, ok := c.listenerSetEntryPolicy[key]; ok {
		return policy
	}
	policy := buildListenerPolicy(listenerEntryToInternalListener(entry, ls))
	c.listenerSetEntryPolicy[key] = policy
	return policy
}

func (c *routeEvaluationContext) evaluateRoute(route routeInput) []routeParentEvaluation {
	prepared := c.prepareRoute(route)
	if len(prepared.effectiveParentRefs) == 0 {
		return nil
	}
	return c.evaluatePreparedRoute(prepared, c.evaluateResolvedRefs(prepared))
}

func (c *routeEvaluationContext) evaluateRouteAttachments(route routeInput) []routeParentEvaluation {
	prepared := c.prepareRoute(route)
	if len(prepared.effectiveParentRefs) == 0 {
		return nil
	}
	return c.evaluatePreparedRoute(prepared, routeResolutionEvaluation{
		resolvedCondition: conditionSpec{
			ObservedGeneration: route.generation,
		},
	})
}

func (c *routeEvaluationContext) evaluatePreparedRoute(route preparedRouteInput, resolution routeResolutionEvaluation) []routeParentEvaluation {
	out := make([]routeParentEvaluation, 0, len(route.effectiveParentRefs))
	for _, parentRef := range route.effectiveParentRefs {
		eval, ok := c.evaluateParentRef(route, parentRef, resolution)
		if ok {
			out = append(out, eval)
		}
	}
	return out
}

func (c *routeEvaluationContext) evaluateParentRef(
	route preparedRouteInput,
	parentRef gatewayv1.ParentReference,
	resolution routeResolutionEvaluation,
) (routeParentEvaluation, bool) {
	normalizedParentRef := normalizeParentRef(route.namespace, parentRef)
	if isServiceParentRef(parentRef) {
		return evaluateServiceParentRef(c.state, route.routeInput, parentRef, normalizedParentRef, resolution), true
	}
	if isListenerSetParentRef(parentRef) {
		return c.evaluateListenerSetParentRef(route, parentRef, normalizedParentRef, resolution)
	}

	gatewayNamespace := namespaceOrDefault(parentRef.Namespace, route.namespace)
	gatewayKey := namespacedName(gatewayNamespace, string(parentRef.Name))
	gateway, ok := c.state.managedGatewayByKey[gatewayKey]
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

	namespace := c.state.namespaceByName[route.namespace]
	matchedParent := false
	allowedByListeners := false
	matchedListeners := make([]listenerKey, 0, 1)

	for _, listener := range gateway.Spec.Listeners {
		if parentRef.SectionName != nil && listener.Name != *parentRef.SectionName {
			continue
		}
		if parentRef.Port != nil && listener.Port != *parentRef.Port {
			continue
		}

		policy := c.gatewayPolicy(gateway, listener)
		matchedParent = true

		if !listenerAllowsRoute(policy, route.kind, gateway.Namespace, route.namespace, namespace) {
			continue
		}

		allowedByListeners = true
		if !listenerMatchesHostnames(listener, route.hostnames) {
			continue
		}

		matchedListeners = append(matchedListeners, listenerKey{
			gatewayNamespace: gateway.Namespace,
			gatewayName:      gateway.Name,
			listenerName:     listener.Name,
		})
	}

	switch {
	case !matchedParent:
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingParent)
	case !allowedByListeners:
		accepted.Reason = string(gatewayv1.RouteReasonNotAllowedByListeners)
		accepted.Message = "Parent listeners do not allow this route"
	case len(matchedListeners) == 0:
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingListenerHostname)
		accepted.Message = "Route hostnames do not intersect with parent listener hostnames"
	default:
		accepted.Status = metav1.ConditionTrue
		accepted.Reason = string(gatewayv1.RouteReasonAccepted)
		accepted.Message = "Route is accepted by nantian-gw"
	}

	accepted = applyRouteAcceptedError(route.routeInput, accepted)

	return routeParentEvaluation{
		parentRef:         normalizedParentRef,
		controllerName:    gatewayv1.GatewayController(c.state.controllerName),
		acceptedCondition: accepted,
		resolvedCondition: resolution.resolvedCondition,
		extraConditions:   append([]conditionSpec(nil), resolution.extraConditions...),
		matchedListeners:  matchedListeners,
	}, true
}

func (c *routeEvaluationContext) evaluateResolvedRefs(route preparedRouteInput) routeResolutionEvaluation {
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
		resolver := extfilter.NewResolver(c.state.configMaps)
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

		if targetNamespace != route.namespace && !route.serviceParentOnly && !referenceGranted(
			c.state.referenceGrants,
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
			service, ok := c.state.serviceByKey[namespacedName(targetNamespace, backend.Name)]
			if !ok || !servicePortExists(service, backend.Port) {
				result.resolvedCondition.Status = metav1.ConditionFalse
				result.resolvedCondition.Reason = string(gatewayv1.RouteReasonBackendNotFound)
				result.resolvedCondition.Message = "BackendRef does not point to an existing Service"
				return result
			}
		case "ServiceImport":
			serviceImport, ok := c.state.serviceImportByKey[namespacedName(targetNamespace, backend.Name)]
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

func (c *routeEvaluationContext) evaluateListenerSetParentRef(
	route preparedRouteInput,
	parentRef gatewayv1.ParentReference,
	normalizedParentRef gatewayv1.ParentReference,
	resolution routeResolutionEvaluation,
) (routeParentEvaluation, bool) {
	lsNamespace := namespaceOrDefault(parentRef.Namespace, route.namespace)
	lsKey := namespacedName(lsNamespace, string(parentRef.Name))
	ls, ok := c.state.listenerSetByKey[lsKey]
	if !ok {
		return routeParentEvaluation{}, false
	}

	gwKey := listenerSetParentGatewayKey(ls)
	gateway, ok := c.state.managedGatewayByKey[gwKey]
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

	namespace := c.state.namespaceByName[route.namespace]
	matchedParent := false
	allowedByListeners := false
	matchedListeners := make([]listenerKey, 0, 1)

	for _, entry := range ls.Spec.Listeners {
		if parentRef.SectionName != nil && entry.Name != *parentRef.SectionName {
			continue
		}

		entryListener := listenerEntryToInternalListener(entry, ls)
		policy := c.listenerSetPolicy(ls, entry)
		matchedParent = true

		if !listenerAllowsRoute(policy, route.kind, ls.Namespace, route.namespace, namespace) {
			continue
		}

		allowedByListeners = true
		if !listenerMatchesHostnames(entryListener, route.hostnames) {
			continue
		}

		matchedListeners = append(matchedListeners, listenerKey{
			gatewayNamespace: gateway.Namespace,
			gatewayName:      gateway.Name,
			listenerName:     entryListener.Name,
		})
	}

	switch {
	case !matchedParent:
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingParent)
	case !allowedByListeners:
		accepted.Reason = string(gatewayv1.RouteReasonNotAllowedByListeners)
		accepted.Message = "Parent listeners do not allow this route"
	case len(matchedListeners) == 0:
		accepted.Reason = string(gatewayv1.RouteReasonNoMatchingListenerHostname)
		accepted.Message = "Route hostnames do not intersect with parent listener hostnames"
	default:
		accepted.Status = metav1.ConditionTrue
		accepted.Reason = string(gatewayv1.RouteReasonAccepted)
		accepted.Message = "Route is accepted by nantian-gw"
	}

	accepted = applyRouteAcceptedError(route.routeInput, accepted)

	return routeParentEvaluation{
		parentRef:         normalizedParentRef,
		controllerName:    gatewayv1.GatewayController(c.state.controllerName),
		acceptedCondition: accepted,
		resolvedCondition: resolution.resolvedCondition,
		extraConditions:   append([]conditionSpec(nil), resolution.extraConditions...),
		matchedListeners:  matchedListeners,
	}, true
}
