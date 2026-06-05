package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestReconcileTCPRouteMarksCrossNamespaceBackendRefAsNotPermitted(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1alpha2.TCPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "backend"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "tcp",
						Protocol: gatewayv1.TCPProtocolType,
						Port:     9000,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "backend"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1alpha2.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "infra", Generation: 1},
				Spec: gatewayv1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1alpha2.TCPRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "echo",
								Namespace: namespacePtr("backend"),
								Port:      portPtr(8080),
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var route gatewayv1alpha2.TCPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "route"}, &route); err != nil {
		t.Fatalf("Get TCPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.RouteReasonRefNotPermitted), 1)
}
