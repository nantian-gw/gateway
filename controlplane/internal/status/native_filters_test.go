package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEvaluateResolvedRefsRejectsHTTPRequestRedirectAndURLRewriteWhenAllRulesInvalid(t *testing.T) {
	result := evaluateResolvedRefs(&clusterState{}, routeInput{
		kind:                     routeKindHTTP,
		namespace:                "default",
		name:                     "orders",
		generation:               3,
		resolvedRefsErrorMessage: "HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters",
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result.resolvedCondition)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
	if result.resolvedCondition.Message != "HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters" {
		t.Fatalf("unexpected message: %s", result.resolvedCondition.Message)
	}
	if result.resolvedCondition.ObservedGeneration != 3 {
		t.Fatalf("unexpected observed generation: %d", result.resolvedCondition.ObservedGeneration)
	}
	if len(result.extraConditions) != 0 {
		t.Fatalf("expected no extra conditions, got %#v", result.extraConditions)
	}
}

func TestEvaluateResolvedRefsSetsPartiallyInvalidWhenSomeHTTPRulesAreDropped(t *testing.T) {
	result := evaluateResolvedRefs(&clusterState{}, routeInput{
		kind:                         routeKindHTTP,
		namespace:                    "default",
		name:                         "orders",
		generation:                   5,
		partiallyInvalidErrorMessage: "Dropped Rule 2 because HTTPRoute rule 2 must not combine RequestRedirect and URLRewrite filters",
	})

	if result.resolvedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("expected resolved refs true, got %#v", result.resolvedCondition)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonResolvedRefs) {
		t.Fatalf("unexpected resolved refs reason: %s", result.resolvedCondition.Reason)
	}
	if len(result.extraConditions) != 1 {
		t.Fatalf("expected 1 extra condition, got %#v", result.extraConditions)
	}

	partial := result.extraConditions[0]
	if partial.Type != string(gatewayv1.RouteConditionPartiallyInvalid) {
		t.Fatalf("unexpected extra condition type: %s", partial.Type)
	}
	if partial.Status != metav1.ConditionTrue {
		t.Fatalf("unexpected extra condition status: %s", partial.Status)
	}
	if partial.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected extra condition reason: %s", partial.Reason)
	}
	if partial.Message != "Dropped Rule 2 because HTTPRoute rule 2 must not combine RequestRedirect and URLRewrite filters" {
		t.Fatalf("unexpected extra condition message: %s", partial.Message)
	}
	if partial.ObservedGeneration != 5 {
		t.Fatalf("unexpected extra condition generation: %d", partial.ObservedGeneration)
	}
}
