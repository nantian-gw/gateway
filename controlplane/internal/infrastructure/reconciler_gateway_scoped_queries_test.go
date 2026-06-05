package infrastructure

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestLoadManagedGatewaysUsesScopedGatewayIndexes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := withInfrastructureGatewayIndexes(
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerName,
					},
				},
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "other"},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: gatewayv1.GatewayController("example.com/other"),
					},
				},
				&gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
					Spec: gatewayv1.GatewaySpec{
						GatewayClassName: "nantian-gw",
					},
				},
				&gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: "default"},
					Spec: gatewayv1.GatewaySpec{
						GatewayClassName: "other",
					},
				},
			),
	).Build()

	reconciler := New(
		rawValidatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireMatchingField(
					gatewayClassControllerNameIndex,
					string(controllerName),
				),
				reflect.TypeOf(&gatewayv1.GatewayList{}): requireMatchingField(
					gatewayGatewayClassNameIndex,
					"nantian-gw",
				),
			},
		},
		string(controllerName),
		discardLogger(),
	)

	gateways, err := reconciler.loadManagedGateways(context.Background())
	if err != nil {
		t.Fatalf("loadManagedGateways returned error: %v", err)
	}
	if len(gateways) != 1 {
		t.Fatalf("expected 1 managed Gateway, got %d", len(gateways))
	}
	if gateways[0].Namespace != "default" || gateways[0].Name != "public" {
		t.Fatalf("unexpected managed Gateway: %s/%s", gateways[0].Namespace, gateways[0].Name)
	}
}
