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

	"github.com/aether-gateway/aether-gateway/controlplane/internal/managedresources"
)

func TestReconcileRefreshesInfrastructureConvergenceFromReaderWhenGatewayGenerationIsCurrent(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	freshGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 2,
			UID:        staticAddressGatewayUID,
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

	currentService := gatewayInfrastructureServiceForGateway(freshGateway)
	currentFrontendSlice := gatewayInfrastructureEndpointSlice(
		"default",
		"gw",
		managedresources.EndpointSliceRoleGatewayFrontend,
	)
	currentFrontendSlice.Annotations[gatewayConvergenceOwnerGenerationAnnotation] = "2"

	tests := []struct {
		name         string
		staleObjects []client.Object
		freshObjects []client.Object
	}{
		{
			name: "service metadata convergence",
			staleObjects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerName,
					},
				},
				&gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "gw",
						Namespace:  "default",
						Generation: 2,
						UID:        staticAddressGatewayUID,
					},
					Spec: freshGateway.Spec,
				},
				gatewayInfrastructureService("default", "gw"),
			},
			freshObjects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 2},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerName,
					},
				},
				&freshGateway,
				currentService.DeepCopy(),
				currentFrontendSlice.DeepCopy(),
			},
		},
		{
			name: "frontend endpoint slice convergence",
			staleObjects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerName,
					},
				},
				&gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "gw",
						Namespace:  "default",
						Generation: 2,
						UID:        staticAddressGatewayUID,
					},
					Spec: freshGateway.Spec,
				},
				currentService.DeepCopy(),
				gatewayInfrastructureEndpointSlice(
					"default",
					"gw",
					managedresources.EndpointSliceRoleSharedFrontend,
				),
			},
			freshObjects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 2},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerName,
					},
				},
				&freshGateway,
				currentService.DeepCopy(),
				currentFrontendSlice.DeepCopy(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staleClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(
					&gatewayv1.GatewayClass{},
					&gatewayv1.Gateway{},
				).
				WithObjects(tt.staleObjects...).
				Build()

			baseReader := fake.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
				WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
				WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
				WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
				WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
				WithObjects(tt.freshObjects...).
				Build()
			reader := &countingGetReader{Reader: baseReader}

			reconciler := NewWithAddressesAndReader(
				staleClient,
				reader,
				string(controllerName),
				[]string{"127.0.0.1"},
				discardLogger(),
			)
			if err := reconciler.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile returned error: %v", err)
			}

			var gateway gatewayv1.Gateway
			if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
				t.Fatalf("Get Gateway returned error: %v", err)
			}
			assertCondition(
				t,
				gateway.Status.Conditions,
				string(gatewayv1.GatewayConditionProgrammed),
				metav1.ConditionTrue,
				string(gatewayv1.GatewayReasonProgrammed),
				2,
			)
			if reader.gatewayGets != 0 {
				t.Fatalf("gateway reader Get count = %d, want 0 when only infrastructure state is refreshed", reader.gatewayGets)
			}
		})
	}
}

func TestReconcileBatchesInfrastructureRefreshPerNamespace(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	gatewayA := gatewayWithNameAndGenerationForConvergenceTest("gw-a", 2)
	gatewayB := gatewayWithNameAndGenerationForConvergenceTest("gw-b", 2)
	staleServiceA := gatewayInfrastructureService("default", "gw-a")
	staleServiceB := gatewayInfrastructureService("default", "gw-b")

	freshServiceA, freshSliceA := benchmarkGatewayInfrastructureObjects(*gatewayA)
	freshServiceB, freshSliceB := benchmarkGatewayInfrastructureObjects(*gatewayB)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 2},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayA.DeepCopy(),
			gatewayB.DeepCopy(),
			staleServiceA.DeepCopy(),
			staleServiceB.DeepCopy(),
		).
		Build()

	reader := &countingGetReader{Reader: fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 2},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayA.DeepCopy(),
			gatewayB.DeepCopy(),
			freshServiceA.DeepCopy(),
			freshServiceB.DeepCopy(),
			freshSliceA.DeepCopy(),
			freshSliceB.DeepCopy(),
		).
		Build()}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	for _, name := range []string{"gw-a", "gw-b"} {
		var gateway gatewayv1.Gateway
		if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, &gateway); err != nil {
			t.Fatalf("Get Gateway %s returned error: %v", name, err)
		}
		assertCondition(
			t,
			gateway.Status.Conditions,
			string(gatewayv1.GatewayConditionProgrammed),
			metav1.ConditionTrue,
			string(gatewayv1.GatewayReasonProgrammed),
			2,
		)
	}
	if reader.serviceGets != 0 {
		t.Fatalf("service reader Get count = %d, want 0 after batched infrastructure refresh", reader.serviceGets)
	}
	if reader.serviceLists != 1 {
		t.Fatalf("service list count = %d, want 1 namespace-scoped refresh", reader.serviceLists)
	}
	if reader.endpointSliceLists != 1 {
		t.Fatalf("endpoint slice list count = %d, want 1 namespace-scoped refresh", reader.endpointSliceLists)
	}
}
