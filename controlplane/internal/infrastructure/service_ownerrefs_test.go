package infrastructure

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDesiredGatewayServiceSetsGatewayOwnerReference(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
			UID:       types.UID("gateway-uid-123"),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "aether-gateway",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}

	service := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{}, "")

	if len(service.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %#v, want 1 gateway owner", service.OwnerReferences)
	}
	owner := service.OwnerReferences[0]
	if owner.APIVersion != gatewayv1.GroupVersion.String() {
		t.Fatalf("owner APIVersion = %q, want %q", owner.APIVersion, gatewayv1.GroupVersion.String())
	}
	if owner.Kind != "Gateway" {
		t.Fatalf("owner Kind = %q, want Gateway", owner.Kind)
	}
	if owner.Name != "public" {
		t.Fatalf("owner Name = %q, want public", owner.Name)
	}
	if owner.UID != gateway.UID {
		t.Fatalf("owner UID = %q, want %q", owner.UID, gateway.UID)
	}
	if owner.Controller == nil || !*owner.Controller {
		t.Fatalf("owner Controller = %#v, want true", owner.Controller)
	}
}

func TestReconcileBackfillsGatewayServiceOwnerReference(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
					UID:       types.UID("gateway-uid-456"),
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayServiceName("public"),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:        managedByValue,
						serviceRoleLabel:      serviceRoleGateway,
						gatewayNameLabel:      "public",
						gatewayNamespaceLabel: "default",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{{
						Name:       "tcp-80",
						Port:       80,
						TargetPort: intstrFrom(80),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	service, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if len(service.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %#v, want 1 gateway owner", service.OwnerReferences)
	}
	owner := service.OwnerReferences[0]
	if owner.Kind != "Gateway" || owner.Name != "public" || owner.UID != types.UID("gateway-uid-456") {
		t.Fatalf("unexpected ownerReference %#v", owner)
	}
}

func intstrFrom(value int) intstr.IntOrString {
	return intstr.FromInt(value)
}
