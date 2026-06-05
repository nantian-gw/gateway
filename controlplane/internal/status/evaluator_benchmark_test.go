package status

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	benchmarkRouteStateSink         routeState
	benchmarkGatewayEvaluationSink  map[client.ObjectKey]gatewayEvaluation
	benchmarkRouteParentStatusSink  []gatewayv1.RouteParentStatus
	benchmarkListenerStatusSink     []gatewayv1.ListenerStatus
	benchmarkBackendTLSPoliciesSink map[client.ObjectKey]backendTLSPolicyEvaluation
	benchmarkBackendLBPoliciesSink  map[client.ObjectKey]backendLBPolicyEvaluation
	benchmarkGatewayAttachmentsSink map[listenerKey]map[string]struct{}
)

func BenchmarkEvaluateRoutesRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			state := benchmarkLoadStatusState(b, newStatusBenchmarkReconciler(b, routeCount, true))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkRouteStateSink = evaluateRoutes(state)
			}
		})
	}
}

func BenchmarkEvaluateGatewaysGatewayFleet(b *testing.B) {
	for _, gatewayCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("gateways_%d", gatewayCount), func(b *testing.B) {
			b.ReportAllocs()
			state := benchmarkLoadStatusState(b, newGatewayConvergenceBenchmarkReconciler(b, gatewayCount))
			attachments := benchmarkGatewayFleetAttachments(state, 4)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkGatewayEvaluationSink = evaluateGateways(state, attachments)
			}
		})
	}
}

func BenchmarkMergeRouteParents(b *testing.B) {
	for _, parentCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("parents_%d", parentCount), func(b *testing.B) {
			b.ReportAllocs()
			existing, evals := benchmarkRouteParentMergeInputs(parentCount)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkRouteParentStatusSink = mergeRouteParents(existing, evals)
			}
		})
	}
}

func BenchmarkMergeListenerStatuses(b *testing.B) {
	for _, listenerCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("listeners_%d", listenerCount), func(b *testing.B) {
			b.ReportAllocs()
			existing, evals := benchmarkListenerStatusMergeInputs(listenerCount)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkListenerStatusSink = mergeListenerStatuses(existing, evals)
			}
		})
	}
}

func BenchmarkEvaluateBackendPolicyFanout(b *testing.B) {
	for _, policyCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("policies_%d", policyCount), func(b *testing.B) {
			b.ReportAllocs()
			state := benchmarkLoadStatusState(b, newBackendPolicyStatusBenchmarkReconciler(b, policyCount))
			routeState := evaluateRoutes(state)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkBackendTLSPoliciesSink = evaluateBackendTLSPolicies(state, routeState)
				benchmarkBackendLBPoliciesSink = evaluateBackendLBPolicies(state, routeState)
			}
		})
	}
}

func benchmarkLoadStatusState(b *testing.B, reconciler *Reconciler) *clusterState {
	b.Helper()

	state, err := reconciler.loadState(context.Background())
	if err != nil {
		b.Fatalf("loadState returned error: %v", err)
	}
	return state
}

func benchmarkGatewayFleetAttachments(state *clusterState, attachedPerListener int) map[listenerKey]map[string]struct{} {
	out := make(map[listenerKey]map[string]struct{}, len(state.managedGateways))
	for _, gateway := range state.managedGateways {
		for _, listener := range gateway.Spec.Listeners {
			key := listenerKey{
				gatewayNamespace: gateway.Namespace,
				gatewayName:      gateway.Name,
				listenerName:     listener.Name,
			}
			attached := make(map[string]struct{}, attachedPerListener)
			for i := 0; i < attachedPerListener; i++ {
				attached[fmt.Sprintf("%s/%s-route-%d", gateway.Namespace, gateway.Name, i)] = struct{}{}
			}
			out[key] = attached
		}
	}
	benchmarkGatewayAttachmentsSink = out
	return out
}

func benchmarkRouteParentMergeInputs(count int) ([]gatewayv1.RouteParentStatus, []routeParentEvaluation) {
	existing := make([]gatewayv1.RouteParentStatus, 0, count)
	evals := make([]routeParentEvaluation, 0, count)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	for i := 0; i < count; i++ {
		parentRef := gatewayv1.ParentReference{
			Name:        gatewayv1.ObjectName(fmt.Sprintf("gw-%03d", i)),
			SectionName: ptr(gatewayv1.SectionName("http")),
		}
		existing = append(existing, gatewayv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: controllerName,
			Conditions: []metav1.Condition{
				{
					Type:               string(gatewayv1.RouteConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonNoMatchingParent),
					Message:            "previous parent state",
					ObservedGeneration: 1,
				},
				{
					Type:               string(gatewayv1.RouteConditionPartiallyInvalid),
					Status:             metav1.ConditionTrue,
					Reason:             string(gatewayv1.RouteReasonUnsupportedValue),
					Message:            "stale partially invalid state",
					ObservedGeneration: 1,
				},
			},
		})
		evals = append(evals, routeParentEvaluation{
			parentRef:      parentRef,
			controllerName: controllerName,
			acceptedCondition: conditionSpec{
				Type:               string(gatewayv1.RouteConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.RouteReasonAccepted),
				Message:            "Route is accepted by aether-gateway",
				ObservedGeneration: 2,
			},
			resolvedCondition: conditionSpec{
				Type:               string(gatewayv1.RouteConditionResolvedRefs),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.RouteReasonResolvedRefs),
				Message:            "Route references are resolved",
				ObservedGeneration: 2,
			},
		})
	}

	return existing, evals
}

func benchmarkListenerStatusMergeInputs(count int) ([]gatewayv1.ListenerStatus, []listenerEvaluation) {
	existing := make([]gatewayv1.ListenerStatus, 0, count)
	evals := make([]listenerEvaluation, 0, count)
	supportedKinds := []gatewayv1.RouteGroupKind{{
		Group: ptr(gatewayv1.Group(gatewayv1.GroupName)),
		Kind:  gatewayv1.Kind("HTTPRoute"),
	}}

	for i := 0; i < count; i++ {
		listenerName := gatewayv1.SectionName(fmt.Sprintf("http-%03d", i))
		existing = append(existing, gatewayv1.ListenerStatus{
			Name: listenerName,
			Conditions: []metav1.Condition{
				{
					Type:               string(gatewayv1.ListenerConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.ListenerReasonInvalid),
					Message:            "previous listener state",
					ObservedGeneration: 1,
				},
				{
					Type:               string(gatewayv1.ListenerConditionConflicted),
					Status:             metav1.ConditionTrue,
					Reason:             string(gatewayv1.ListenerReasonHostnameConflict),
					Message:            "stale conflict state",
					ObservedGeneration: 1,
				},
			},
		})
		evals = append(evals, listenerEvaluation{
			name:           listenerName,
			supportedKinds: supportedKinds,
			attachedRoutes: int32(i % 8),
			acceptedCondition: conditionSpec{
				Type:               string(gatewayv1.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonAccepted),
				Message:            "Listener is accepted by aether-gateway",
				ObservedGeneration: 2,
			},
			resolvedCondition: conditionSpec{
				Type:               string(gatewayv1.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
				Message:            "Listener references are resolved",
				ObservedGeneration: 2,
			},
			programmedCondition: conditionSpec{
				Type:               string(gatewayv1.ListenerConditionProgrammed),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonProgrammed),
				Message:            "Listener is programmed",
				ObservedGeneration: 2,
			},
		})
	}

	return existing, evals
}
