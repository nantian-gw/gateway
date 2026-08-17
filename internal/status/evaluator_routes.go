package status

import (
	"log/slog"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
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

// routeHostnameOverlapTotal counts hostname overlaps detected between routes
// attached to the same listener. Per the Gateway API spec, intersecting hostnames
// on the same listener are not a conflict: all intersecting routes remain Accepted
// and attached, and the data plane selects the most specific hostname for a given
// request. The counter exists so overlaps are observable rather than silently
// dropped, without altering route status or listener attachments.
var routeHostnameOverlapTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "nantian_gw_controlplane_route_hostname_overlap_total",
		Help: "Total number of hostname overlaps detected between routes attached to the same listener; overlapping routes remain accepted.",
	},
	[]string{"listener"},
)

func init() {
	ctrlmetrics.Registry.MustRegister(routeHostnameOverlapTotal)
}

// routeHostnameInfo carries the hostname set and creation timestamp of a route,
// used to enumerate hostname overlaps deterministically.
type routeHostnameInfo struct {
	hostnames []gatewayv1.Hostname
	createdAt metav1.Time
}

// observeRouteHostnameOverlaps detects hostname overlaps between routes attached to
// the same listener. Per the Gateway API spec, intersecting hostnames on the same
// listener are not a conflict: all intersecting routes remain Accepted and attached,
// and the data plane selects the most specific hostname for a given request. This
// function therefore only reports overlaps for observability (a counter metric and a
// structured log entry); it never changes route status or listener attachments.
func observeRouteHostnameOverlaps(state *clusterState, out *routeState) {
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
					observeRouteHostnameOverlap(listener, candidates[i], candidates[j])
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

// observeRouteHostnameOverlap records one hostname overlap between two routes
// attached to the same listener. Both routes remain Accepted and attached; the data
// plane selects the most specific hostname for a given request.
func observeRouteHostnameOverlap(listener listenerKey, left, right client.ObjectKey) {
	routeHostnameOverlapTotal.WithLabelValues(
		listener.gatewayNamespace + "/" + listener.gatewayName + "/" + string(listener.listenerName),
	).Inc()
	slog.Info("hostname overlap detected between routes on the same listener; both routes remain accepted",
		"gateway", listener.gatewayNamespace+"/"+listener.gatewayName,
		"listener", string(listener.listenerName),
		"route_a", left.String(),
		"route_b", right.String(),
	)
}
