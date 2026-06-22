package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gwapi"
)

func evaluateRoute(state *clusterState, route routeInput) []routeParentEvaluation {
	return newRouteEvaluationContext(state).evaluateRoute(route)
}

func routeEffectiveParentRefs(state *clusterState, route routeInput) []gatewayv1.ParentReference {
	return gwapi.DefaultGatewayParentRefs(
		route.parentRefs,
		route.namespace,
		route.defaultGatewayScope,
		state.managedGateways,
	)
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
	ctx := newRouteEvaluationContext(state)
	return ctx.evaluateResolvedRefs(ctx.prepareRoute(route))
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
