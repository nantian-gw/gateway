package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestGatewayInfrastructureParametersMessageFiltersUnrelatedAcceptedMessages(t *testing.T) {
	t.Parallel()

	conditions := []metav1.Condition{{
		Type:    string(gatewayv1.GatewayConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(gatewayv1.GatewayReasonInvalidParameters),
		Message: "some other validation failed",
	}}

	if got := gatewayInfrastructureParametersMessage(conditions); got != "" {
		t.Fatalf("expected unrelated invalid-parameters message to be ignored, got %q", got)
	}
}

func TestGatewayInfrastructureParametersMessageReturnsInfrastructureRefFailures(t *testing.T) {
	t.Parallel()

	message := "Gateway.spec.infrastructure.parametersRef points to an unsupported ConfigMap"
	conditions := []metav1.Condition{{
		Type:    string(gatewayv1.GatewayConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(gatewayv1.GatewayReasonInvalidParameters),
		Message: message,
	}}

	if got := gatewayInfrastructureParametersMessage(conditions); got != message {
		t.Fatalf("expected infrastructure parameters message, got %q", got)
	}
}
