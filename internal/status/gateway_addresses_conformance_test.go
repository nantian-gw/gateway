package status

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileGatewayStaticAddressIgnoresStaleDerivedServiceAdvertisements(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	oldGateway := staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
		Type:  addressTypePtr(gatewayv1.IPAddressType),
		Value: "203.0.113.13",
	}})
	oldGateway.Generation = 2
	oldService := gatewayInfrastructureServiceForGateway(*oldGateway)
	oldService.Spec.Type = corev1.ServiceTypeLoadBalancer
	oldService.Spec.ExternalIPs = []string{"203.0.113.13"}

	currentGateway := staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
		Type:  addressTypePtr(gatewayv1.IPAddressType),
		Value: "127.0.0.1",
	}})
	currentGateway.Generation = 3

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			currentGateway,
			oldService,
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		3,
	)
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected current static address while derived service generation is stale, got %#v", gateway.Status.Addresses)
	}
}
