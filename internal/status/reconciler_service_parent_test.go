package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileSetsHTTPRouteStatusForServiceParent(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.HTTPRoute{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-mesh"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-mesh-consumer"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-v1", Namespace: "gateway-conformance-mesh"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mesh-echo-add-header",
					Namespace:  "gateway-conformance-mesh-consumer",
					Generation: 1,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group("")),
							Kind:      ptr(gatewayv1.Kind("Service")),
							Name:      "echo-v1",
							Namespace: namespacePtr("gateway-conformance-mesh"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo-v1",
									Namespace: namespacePtr("gateway-conformance-mesh"),
									Port:      portPtr(80),
								},
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

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: "gateway-conformance-mesh-consumer",
			Name:      "mesh-echo-add-header",
		},
		&route,
	); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	parent := route.Status.Parents[0]
	if parent.ParentRef.Kind == nil || *parent.ParentRef.Kind != gatewayv1.Kind("Service") {
		t.Fatalf("parentRef.kind = %#v, want Service", parent.ParentRef.Kind)
	}
	if parent.ParentRef.Group == nil || *parent.ParentRef.Group != gatewayv1.Group("") {
		t.Fatalf("parentRef.group = %#v, want empty core group", parent.ParentRef.Group)
	}
	if parent.ParentRef.Namespace == nil || *parent.ParentRef.Namespace != gatewayv1.Namespace("gateway-conformance-mesh") {
		t.Fatalf("parentRef.namespace = %#v, want gateway-conformance-mesh", parent.ParentRef.Namespace)
	}
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}
