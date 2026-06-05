package infrastructure

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestLoadManagedGatewaysRequiresFieldIndexes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
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
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
				},
			},
		).
		Build()

	reconciler := New(cl, string(controllerName), discardLogger())
	_, err := reconciler.loadManagedGateways(context.Background())
	if err == nil {
		t.Fatal("expected missing field index error, got nil")
	}
	if !strings.Contains(err.Error(), gatewayClassControllerNameIndex) {
		t.Fatalf("expected error to mention %q, got %v", gatewayClassControllerNameIndex, err)
	}
}

func TestListHTTPRoutesWithServiceParentsRequiresFieldIndex(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mesh",
					Namespace: "default",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
		).
		Build()

	_, err := listHTTPRoutesWithServiceParents(context.Background(), cl)
	if err == nil {
		t.Fatal("expected missing field index error, got nil")
	}
	if !strings.Contains(err.Error(), httpRouteServiceParentIndex) {
		t.Fatalf("expected error to mention %q, got %v", httpRouteServiceParentIndex, err)
	}
}
