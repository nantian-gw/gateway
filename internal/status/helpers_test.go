package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestMergeRouteParentsDoesNotMutateExistingConditions(t *testing.T) {
	existing := []gatewayv1.RouteParentStatus{{
		ParentRef: gatewayv1.ParentReference{
			Name:        "gw",
			SectionName: ptr[gatewayv1.SectionName]("http"),
		},
		ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
		Conditions: []metav1.Condition{{
			Type:               string(gatewayv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			Reason:             string(gatewayv1.RouteReasonBackendNotFound),
			ObservedGeneration: 1,
		}},
	}}

	merged := mergeRouteParents(existing, []routeParentEvaluation{{
		parentRef:      existing[0].ParentRef,
		controllerName: existing[0].ControllerName,
		resolvedCondition: conditionSpec{
			Type:               string(gatewayv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.RouteReasonResolvedRefs),
			Message:            "Route references are resolved",
			ObservedGeneration: 1,
		},
	}})

	if existing[0].Conditions[0].Status != metav1.ConditionFalse {
		t.Fatalf("existing route parent condition mutated to %s", existing[0].Conditions[0].Status)
	}
	if merged[0].Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("merged route parent condition status = %s, want %s", merged[0].Conditions[0].Status, metav1.ConditionTrue)
	}
}

func TestMergeListenerStatusesDoesNotMutateExistingConditions(t *testing.T) {
	existing := []gatewayv1.ListenerStatus{{
		Name: "http",
		Conditions: []metav1.Condition{{
			Type:               string(gatewayv1.ListenerConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			Reason:             string(gatewayv1.ListenerReasonInvalidRouteKinds),
			ObservedGeneration: 1,
		}},
	}}

	merged := mergeListenerStatuses(existing, []listenerEvaluation{{
		name: "http",
		resolvedCondition: conditionSpec{
			Type:               string(gatewayv1.ListenerConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
			Message:            "Listener references are resolved",
			ObservedGeneration: 1,
		},
	}})

	if existing[0].Conditions[0].Status != metav1.ConditionFalse {
		t.Fatalf("existing listener condition mutated to %s", existing[0].Conditions[0].Status)
	}
	if merged[0].Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("merged listener condition status = %s, want %s", merged[0].Conditions[0].Status, metav1.ConditionTrue)
	}
}

func TestMergeRouteParentsRemovesStalePartiallyInvalidCondition(t *testing.T) {
	existing := []gatewayv1.RouteParentStatus{{
		ParentRef: gatewayv1.ParentReference{
			Name: "gw",
		},
		ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
		Conditions: []metav1.Condition{{
			Type:               string(gatewayv1.RouteConditionPartiallyInvalid),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.RouteReasonUnsupportedValue),
			ObservedGeneration: 2,
		}},
	}}

	merged := mergeRouteParents(existing, []routeParentEvaluation{{
		parentRef:      existing[0].ParentRef,
		controllerName: existing[0].ControllerName,
	}})

	assertConditionAbsent(t, merged[0].Conditions, string(gatewayv1.RouteConditionPartiallyInvalid))
}
