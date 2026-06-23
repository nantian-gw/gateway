package infrastructure

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileDeletesStaleGatewayInfrastructureServices(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayServiceName("stale"),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleGateway,
						gatewayNameLabel: "stale",
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("stale")},
		&corev1.Service{},
	)
	if !serviceMissing(err) {
		t.Fatalf("expected stale service to be deleted, got err=%v", err)
	}
}
func TestReconcileDeletesStaleGatewayInfrastructureEndpointResources(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	serviceName := gatewayServiceName("stale")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:        managedByValue,
						serviceRoleLabel:      serviceRoleGateway,
						gatewayNameLabel:      "stale",
						gatewayNamespaceLabel: "default",
					},
				},
			},
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: "default",
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-gateway-ep-stale",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: serviceName,
						discoveryv1.LabelManagedBy:   managedByValue,
						serviceRoleLabel:             gatewayEndpointSliceRoleValue,
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: serviceName},
		&corev1.Service{},
	)
	if !serviceMissing(err) {
		t.Fatalf("expected stale service to be deleted, got err=%v", err)
	}

	endpoints := &corev1.Endpoints{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: serviceName},
		endpoints,
	); client.IgnoreNotFound(err) != nil {
		t.Fatalf("Get stale Endpoints returned error: %v", err)
	} else if err == nil {
		t.Fatalf("expected stale Endpoints to be deleted")
	}

	endpointSlice := &discoveryv1.EndpointSlice{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "nantian-gw-gateway-ep-stale"},
		endpointSlice,
	); client.IgnoreNotFound(err) != nil {
		t.Fatalf("Get stale EndpointSlice returned error: %v", err)
	} else if err == nil {
		t.Fatalf("expected stale EndpointSlice to be deleted")
	}
}
func TestReconcileDeletesSharedServiceWithoutManagedListeners(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultSharedServiceName,
					Namespace: defaultDataplaneNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeNodePort,
					Selector: map[string]string{"app": "nantian-gw-dataplane"},
					Ports: []corev1.ServicePort{{
						Name:       "tcp-80",
						Port:       80,
						TargetPort: intstr.FromInt(80),
						Protocol:   corev1.ProtocolTCP,
						NodePort:   30080,
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
		&corev1.Service{},
	)
	if !serviceMissing(err) {
		t.Fatalf("expected shared service to be deleted, got err=%v", err)
	}

	policy := &networkingv1.NetworkPolicy{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: defaultDataplaneNamespace,
			Name:      defaultDataplaneNetworkPolicyName,
		},
		policy,
	); err != nil {
		t.Fatalf("Get dataplane NetworkPolicy returned error: %v", err)
	}

	if len(policy.Spec.Ingress) != 1 {
		t.Fatalf("expected only admin ingress rule, got %#v", policy.Spec.Ingress)
	}
	assertAdminNetworkPolicyRule(t, policy.Spec.Ingress, defaultDataplaneNamespace)
}
