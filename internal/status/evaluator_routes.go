package status

import (
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
)

func evaluateRoute(state *clusterState, route routeInput) []routeParentEvaluation {
	return newRouteEvaluationContext(state).evaluateRoute(route)
}

func routeEffectiveParentRefs(state *clusterState, route routeInput) []gatewayv1.ParentReference {
	return gatewayapi.DefaultGatewayParentRefs(
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

// routeReasonHostnameConflict is the reason reported on the Accepted condition of a
// route that loses a hostname conflict on a listener. gateway-api does not define a
// route-level reason for this case (only ListenerReasonHostnameConflict), so a custom
// reason string is used, as allowed by the spec comment on RouteReasonAccepted.
const routeReasonHostnameConflict = "HostnameConflict"

// routeHostnameInfo carries the hostname set and creation timestamp of a route, used
// to resolve hostname conflicts deterministically (earliest creation wins).
type routeHostnameInfo struct {
	hostnames []gatewayv1.Hostname
	createdAt metav1.Time
}

// evaluateRouteConflicts detects hostname overlaps between routes attached to the
// same listener and marks the later-created route as conflicted (Accepted=False with
// Reason=HostnameConflict) via the existing extraConditions mechanism, mirroring the
// listener-level conflict pattern in evaluateListenerConflict.
func evaluateRouteConflicts(state *clusterState, out *routeState) {
	infoByKey := make(map[client.ObjectKey]routeHostnameInfo)
	for _, route := range state.httpRoutes {
		key := client.ObjectKeyFromObject(&route)
		if len(route.Spec.Hostnames) == 0 {
			continue
		}
		infoByKey[key] = routeHostnameInfo{hostnames: route.Spec.Hostnames, createdAt: route.CreationTimestamp}
	}
	for _, route := range state.grpcRoutes {
		key := client.ObjectKeyFromObject(&route)
		if len(route.Spec.Hostnames) == 0 {
			continue
		}
		infoByKey[key] = routeHostnameInfo{hostnames: route.Spec.Hostnames, createdAt: route.CreationTimestamp}
	}
	for _, route := range state.tlsRoutes {
		key := client.ObjectKeyFromObject(&route)
		if len(route.Spec.Hostnames) == 0 {
			continue
		}
		infoByKey[key] = routeHostnameInfo{hostnames: route.Spec.Hostnames, createdAt: route.CreationTimestamp}
	}

	for listener, attached := range out.attachments {
		candidates := make([]client.ObjectKey, 0, len(attached))
		for key := range attached {
			if _, ok := infoByKey[key]; ok {
				candidates = append(candidates, key)
			}
		}

		sort.Slice(candidates, func(i, j int) bool {
			left := infoByKey[candidates[i]].createdAt.Time
			right := infoByKey[candidates[j]].createdAt.Time
			if left.Equal(right) {
				return candidates[i].String() < candidates[j].String()
			}
			return left.Before(right)
		})

		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				if routeHostnameSetsOverlap(infoByKey[candidates[i]].hostnames, infoByKey[candidates[j]].hostnames) {
					markRouteHostnameConflict(out, candidates[j], listener, candidates[i])
				}
			}
		}
	}
}

// routeHostnameSetsOverlap reports whether two hostname sets overlap. Routes with an
// empty hostname list are excluded from conflict detection (a catch-all route coexists
// with specific-hostname routes; the specific one wins for its hostname).
func routeHostnameSetsOverlap(a, b []gatewayv1.Hostname) bool {
	for _, left := range a {
		for _, right := range b {
			if hostnamesIntersect(string(left), string(right)) {
				return true
			}
		}
	}
	return false
}

// markRouteHostnameConflict appends an Accepted=False condition with
// Reason=HostnameConflict to every parent evaluation of the losing route that
// matched the conflicting listener. mergeRouteParents writes extraConditions after
// acceptedCondition, so the appended condition overrides the earlier Accepted=True.
func markRouteHostnameConflict(out *routeState, loserKey client.ObjectKey, listener listenerKey, winnerKey client.ObjectKey) {
	message := fmt.Sprintf("Hostname conflict with route %s/%s on listener %s; the earlier route wins", winnerKey.Namespace, winnerKey.Name, listener.listenerName)

	mark := func(evals []routeParentEvaluation) {
		for i := range evals {
			if evals[i].acceptedCondition.Status != metav1.ConditionTrue {
				continue
			}
			for _, matched := range evals[i].matchedListeners {
				if matched == listener {
					evals[i].extraConditions = append(evals[i].extraConditions, conditionSpec{
						Type:               string(gatewayv1.RouteConditionAccepted),
						Status:             metav1.ConditionFalse,
						Reason:             routeReasonHostnameConflict,
						Message:            message,
						ObservedGeneration: evals[i].acceptedCondition.ObservedGeneration,
					})
					break
				}
			}
		}
	}
	mark(out.http[loserKey])
	mark(out.grpc[loserKey])
	mark(out.tls[loserKey])
	if attached := out.attachments[listener]; attached != nil {
		delete(attached, loserKey)
	}
}
