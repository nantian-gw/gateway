package status

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func TestBackendRefStatusIndexKeysDeduplicatesSortsAndDefaultsNamespace(t *testing.T) {
	route := routeInput{
		namespace: "apps",
		backends: []backendInput{
			{Kind: "Service", Name: "orders"},
			{Kind: "Service", Namespace: "shared", Name: "orders"},
			{Kind: "Service", Name: "orders"},
			{Group: mcsv1alpha1.GroupName, Kind: "ServiceImport", Name: "catalog"},
			{Group: "example.com", Kind: "Other", Name: "ignored"},
			{Kind: "Service"},
		},
	}

	got := backendRefStatusIndexKeys(route)
	want := []string{
		"Service/apps/orders",
		"Service/shared/orders",
		"ServiceImport/apps/catalog",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backendRefStatusIndexKeys() = %#v, want %#v", got, want)
	}
}

func TestRouteUsesServiceParent(t *testing.T) {
	serviceKind := gatewayv1.Kind("Service")
	gatewayKind := gatewayv1.Kind("Gateway")

	route := routeInput{
		namespace: "apps",
		parentRefs: []gatewayv1.ParentReference{
			{Kind: &gatewayKind, Name: "edge"},
			{Kind: &serviceKind, Name: "orders"},
			{Kind: &serviceKind, Namespace: namespacePtr("shared"), Name: "inventory"},
		},
	}

	if !routeUsesServiceParent(route, client.ObjectKey{Namespace: "apps", Name: "orders"}) {
		t.Fatal("expected route to use default-namespace service parent")
	}
	if !routeUsesServiceParent(route, client.ObjectKey{Namespace: "shared", Name: "inventory"}) {
		t.Fatal("expected route to use explicit-namespace service parent")
	}
	if routeUsesServiceParent(route, client.ObjectKey{Namespace: "apps", Name: "missing"}) {
		t.Fatal("unexpected service parent match")
	}
}

func TestRouteUsesBackendRef(t *testing.T) {
	route := routeInput{
		namespace: "apps",
		backends: []backendInput{
			{Kind: "Service", Name: "orders"},
			{Group: mcsv1alpha1.GroupName, Kind: "ServiceImport", Namespace: "imports", Name: "catalog"},
			{Group: "example.com", Kind: "Other", Namespace: "apps", Name: "ignored"},
		},
	}

	if !routeUsesBackendRef(route, "Service", client.ObjectKey{Namespace: "apps", Name: "orders"}) {
		t.Fatal("expected route to use default-namespace service backend")
	}
	if !routeUsesBackendRef(route, "ServiceImport", client.ObjectKey{Namespace: "imports", Name: "catalog"}) {
		t.Fatal("expected route to use explicit-namespace ServiceImport backend")
	}
	if routeUsesBackendRef(route, "Service", client.ObjectKey{Namespace: "imports", Name: "catalog"}) {
		t.Fatal("unexpected service match for ServiceImport backend")
	}
}

func TestListRoutesForBackendRefFallsBackToClientSideFiltering(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("install gateway v1 scheme: %v", err)
	}
	if err := gatewayv1alpha2.Install(scheme); err != nil {
		t.Fatalf("install gateway v1alpha2 scheme: %v", err)
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "match-grpc"},
			Spec: gatewayv1.GRPCRouteSpec{Rules: []gatewayv1.GRPCRouteRule{{
				BackendRefs: []gatewayv1.GRPCBackendRef{{BackendRef: serviceBackendRef("orders")}},
			}}},
		},
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "miss-grpc"},
			Spec: gatewayv1.GRPCRouteSpec{Rules: []gatewayv1.GRPCRouteRule{{
				BackendRefs: []gatewayv1.GRPCBackendRef{{BackendRef: serviceBackendRef("users")}},
			}}},
		},
		&gatewayv1alpha2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "match-tls"},
			Spec: gatewayv1alpha2.TLSRouteSpec{Rules: []gatewayv1alpha2.TLSRouteRule{{
				BackendRefs: []gatewayv1.BackendRef{serviceImportBackendRef("imports", "catalog")},
			}}},
		},
	).Build()

	grpcRoutes, scoped, err := listGRPCRoutesForServiceBackend(ctx, cl, client.ObjectKey{Namespace: "apps", Name: "orders"})
	if err != nil {
		t.Fatalf("listGRPCRoutesForServiceBackend() returned error: %v", err)
	}
	if scoped {
		t.Fatal("listGRPCRoutesForServiceBackend() scoped = true, want false when fake client has no index")
	}
	if len(grpcRoutes) != 1 || grpcRoutes[0].Name != "match-grpc" {
		t.Fatalf("listGRPCRoutesForServiceBackend() = %#v, want match-grpc", grpcRoutes)
	}

	tlsRoutes, scoped, err := listTLSRoutesForServiceImportBackend(ctx, cl, client.ObjectKey{Namespace: "imports", Name: "catalog"})
	if err != nil {
		t.Fatalf("listTLSRoutesForServiceImportBackend() returned error: %v", err)
	}
	if scoped {
		t.Fatal("listTLSRoutesForServiceImportBackend() scoped = true, want false when fake client has no index")
	}
	if len(tlsRoutes) != 1 || tlsRoutes[0].Name != "match-tls" {
		t.Fatalf("listTLSRoutesForServiceImportBackend() = %#v, want match-tls", tlsRoutes)
	}
}

func TestListRoutesForServiceParentFallsBackToClientSideFiltering(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("install gateway v1 scheme: %v", err)
	}

	serviceKind := gatewayv1.Kind("Service")
	gatewayKind := gatewayv1.Kind("Gateway")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "service-parent"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Kind: &serviceKind, Name: "orders"}},
				},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "gateway-parent"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Kind: &gatewayKind, Name: "edge"}},
				},
			},
		},
	).Build()

	routes, scoped, err := listHTTPRoutesForServiceParent(ctx, cl, client.ObjectKey{Namespace: "apps", Name: "orders"})
	if err != nil {
		t.Fatalf("listHTTPRoutesForServiceParent() returned error: %v", err)
	}
	if scoped {
		t.Fatal("listHTTPRoutesForServiceParent() scoped = true, want false when fake client has no index")
	}
	if len(routes) != 1 || routes[0].Name != "service-parent" {
		t.Fatalf("listHTTPRoutesForServiceParent() = %#v, want service-parent", routes)
	}
}
