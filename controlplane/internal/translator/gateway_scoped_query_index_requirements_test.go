package translator

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestListGatewayClassesForControllerRequiresFieldIndex(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
		).
		Build()

	_, err := listGatewayClassesForController(context.Background(), cl, string(controllerName))
	if err == nil {
		t.Fatal("expected missing field index error, got nil")
	}
	if !strings.Contains(err.Error(), gatewayClassControllerNameIndex) {
		t.Fatalf("expected error to mention %q, got %v", gatewayClassControllerNameIndex, err)
	}
}

func TestListGatewaysForGatewayClassRequiresFieldIndex(t *testing.T) {
	scheme := buildSupportScheme(t)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
				},
			},
		).
		Build()

	_, err := listGatewaysForGatewayClass(context.Background(), cl, "nantian-gw")
	if err == nil {
		t.Fatal("expected missing field index error, got nil")
	}
	if !strings.Contains(err.Error(), gatewayGatewayClassNameIndex) {
		t.Fatalf("expected error to mention %q, got %v", gatewayGatewayClassNameIndex, err)
	}
}
