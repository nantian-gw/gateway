package translator

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
)

func TestAffectedBackendRefRoutesFindsRoutesAcrossKinds(t *testing.T) {
	current := &ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Namespace: "apps",
			Name:      "http",
			Rules: []ir.HTTPRule{{
				BackendRefs: []ir.BackendRef{serviceIRBackendRef("apps", "orders", 80)},
			}},
		}},
		GRPCRoutes: []ir.GRPCRoute{{
			Namespace: "apps",
			Name:      "grpc",
			Rules: []ir.GRPCRule{{
				BackendRefs: []ir.BackendRef{serviceImportIRBackendRef("imports", "catalog", 9090)},
			}},
		}},
		StreamRoutes: []ir.StreamRoute{
			{
				Kind:      "TCP",
				Namespace: "apps",
				Name:      "tcp",
				Rules: []ir.StreamRule{{
					BackendRefs: []ir.BackendRef{serviceIRBackendRef("shared", "tcp-backend", 7000)},
				}},
			},
			{
				Kind:      "UDP",
				Namespace: "apps",
				Name:      "udp",
				Rules: []ir.StreamRule{{
					BackendRefs: []ir.BackendRef{serviceIRBackendRef("apps", "orders", 53)},
				}},
			},
			{
				Kind:      "TLS",
				Namespace: "apps",
				Name:      "tls",
				Rules: []ir.StreamRule{{
					BackendRefs: []ir.BackendRef{serviceImportIRBackendRef("imports", "catalog", 9443)},
				}},
			},
			{
				Kind:      "TCP",
				Namespace: "apps",
				Name:      "unaffected",
				Rules: []ir.StreamRule{{
					BackendRefs: []ir.BackendRef{serviceIRBackendRef("apps", "users", 80)},
				}},
			},
		},
	}

	routeKeys, httpRoutes, grpcRoutes, streamRoutes := affectedBackendRefRoutes(
		current,
		[]client.ObjectKey{{Namespace: "apps", Name: "orders"}},
		[]client.ObjectKey{{Namespace: "imports", Name: "catalog"}},
		[]string{"shared"},
	)

	if !reflect.DeepEqual(routeKeys.http, []client.ObjectKey{{Namespace: "apps", Name: "http"}}) {
		t.Fatalf("routeKeys.http = %#v, want apps/http", routeKeys.http)
	}
	if !reflect.DeepEqual(routeKeys.grpc, []client.ObjectKey{{Namespace: "apps", Name: "grpc"}}) {
		t.Fatalf("routeKeys.grpc = %#v, want apps/grpc", routeKeys.grpc)
	}
	if !reflect.DeepEqual(routeKeys.tcp, []client.ObjectKey{{Namespace: "apps", Name: "tcp"}}) {
		t.Fatalf("routeKeys.tcp = %#v, want apps/tcp", routeKeys.tcp)
	}
	if !reflect.DeepEqual(routeKeys.udp, []client.ObjectKey{{Namespace: "apps", Name: "udp"}}) {
		t.Fatalf("routeKeys.udp = %#v, want apps/udp", routeKeys.udp)
	}
	if !reflect.DeepEqual(routeKeys.tls, []client.ObjectKey{{Namespace: "apps", Name: "tls"}}) {
		t.Fatalf("routeKeys.tls = %#v, want apps/tls", routeKeys.tls)
	}
	if len(httpRoutes) != 1 || httpRoutes[0].Name != "http" {
		t.Fatalf("httpRoutes = %#v, want http", httpRoutes)
	}
	if len(grpcRoutes) != 1 || grpcRoutes[0].Name != "grpc" {
		t.Fatalf("grpcRoutes = %#v, want grpc", grpcRoutes)
	}
	if len(streamRoutes) != 3 {
		t.Fatalf("streamRoutes length = %d, want 3: %#v", len(streamRoutes), streamRoutes)
	}
}

func TestAffectedBackendRefRoutesHandlesNilSnapshot(t *testing.T) {
	routeKeys, httpRoutes, grpcRoutes, streamRoutes := affectedBackendRefRoutes(
		nil,
		[]client.ObjectKey{{Namespace: "apps", Name: "orders"}},
		nil,
		nil,
	)

	if !reflect.DeepEqual(routeKeys, affectedBackendRefRouteKeys{}) {
		t.Fatalf("routeKeys = %#v, want empty", routeKeys)
	}
	if httpRoutes != nil || grpcRoutes != nil || streamRoutes != nil {
		t.Fatalf("routes = %#v %#v %#v, want nil slices", httpRoutes, grpcRoutes, streamRoutes)
	}
}

func TestRouteBackendRefsTouchAffectedBackends(t *testing.T) {
	serviceKeys := objectKeyMap([]client.ObjectKey{{Namespace: "apps", Name: "orders"}})
	serviceImportKeys := objectKeyMap([]client.ObjectKey{{Namespace: "imports", Name: "catalog"}})
	namespaceSet := map[string]struct{}{"shared": {}}

	tests := []struct {
		name string
		refs []ir.BackendRef
		want bool
	}{
		{
			name: "matches changed service key",
			refs: []ir.BackendRef{serviceIRBackendRef("apps", "orders", 80)},
			want: true,
		},
		{
			name: "matches changed service import key",
			refs: []ir.BackendRef{serviceImportIRBackendRef("imports", "catalog", 9090)},
			want: true,
		},
		{
			name: "matches known backend kind in affected namespace",
			refs: []ir.BackendRef{serviceIRBackendRef("shared", "payments", 8080)},
			want: true,
		},
		{
			name: "ignores unsupported backend kind",
			refs: []ir.BackendRef{{Group: "example.com", Kind: "Other", Namespace: "shared", Name: "external"}},
		},
		{
			name: "ignores missing backend identity",
			refs: []ir.BackendRef{{Namespace: "apps"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeBackendRefsTouchAffectedBackends(tt.refs, serviceKeys, serviceImportKeys, namespaceSet)
			if got != tt.want {
				t.Fatalf("routeBackendRefsTouchAffectedBackends() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefreshBackendRefUpdatesAndCleansValidationMetadata(t *testing.T) {
	annotator := backends.NewBackendRefTranslator(
		[]corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "apps"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}}}},
		nil,
		nil,
		extfilter.Resolver{},
		nil,
		nil,
	)

	validRef := refreshBackendRef(
		ir.BackendRef{
			Namespace: "apps",
			Name:      "orders",
			Port:      80,
			Metadata: map[string]string{
				backends.BackendRefMetaValid:  "false",
				backends.BackendRefMetaReason: string(gatewayv1.RouteReasonBackendNotFound),
				"keep":               "value",
			},
		},
		"apps",
		backends.RouteKindHTTP,
		false,
		annotator,
	)
	if !reflect.DeepEqual(validRef.Metadata, map[string]string{"keep": "value"}) {
		t.Fatalf("validRef.Metadata = %#v, want only unrelated metadata", validRef.Metadata)
	}

	cleanRef := refreshBackendRef(
		ir.BackendRef{
			Namespace: "apps",
			Name:      "orders",
			Port:      80,
			Metadata: map[string]string{
				backends.BackendRefMetaValid:  "false",
				backends.BackendRefMetaReason: string(gatewayv1.RouteReasonBackendNotFound),
			},
		},
		"apps",
		backends.RouteKindHTTP,
		false,
		annotator,
	)
	if cleanRef.Metadata != nil {
		t.Fatalf("cleanRef.Metadata = %#v, want nil after stale validation metadata is removed", cleanRef.Metadata)
	}

	invalidRef := refreshBackendRef(
		ir.BackendRef{
			Namespace: "apps",
			Name:      "missing",
			Port:      80,
			Metadata:  map[string]string{"keep": "value"},
		},
		"apps",
		backends.RouteKindHTTP,
		false,
		annotator,
	)
	wantInvalid := map[string]string{
		backends.BackendRefMetaValid:  "false",
		backends.BackendRefMetaReason: string(gatewayv1.RouteReasonBackendNotFound),
	}
	if !reflect.DeepEqual(invalidRef.Metadata, wantInvalid) {
		t.Fatalf("invalidRef.Metadata = %#v, want %#v", invalidRef.Metadata, wantInvalid)
	}
}

func TestRefreshHTTPRouteBackendRefsAllowsCrossNamespaceRefsForServiceParents(t *testing.T) {
	annotator := backends.NewBackendRefTranslator(
		[]corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "shared"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}}}},
		nil,
		nil,
		extfilter.Resolver{},
		nil,
		nil,
	)
	routes := []ir.HTTPRoute{{
		Namespace:  "apps",
		Name:       "http",
		ParentRefs: []ir.ParentRef{{Kind: "Service", Name: "frontend"}},
		Rules: []ir.HTTPRule{{
			BackendRefs: []ir.BackendRef{serviceIRBackendRef("shared", "orders", 80)},
		}},
	}}

	refreshHTTPRouteBackendRefs(routes, annotator)

	if metadata := routes[0].Rules[0].BackendRefs[0].Metadata; len(metadata) != 0 {
		t.Fatalf("cross-namespace service-parent backend metadata = %#v, want none", metadata)
	}
}

func TestRouteKindForStreamIR(t *testing.T) {
	tests := []struct {
		kind string
		want backends.RouteKind
	}{
		{kind: "TCP", want: backends.RouteKindTCP},
		{kind: "UDP", want: backends.RouteKindUDP},
		{kind: "TLS", want: backends.RouteKindTLS},
		{kind: "unknown", want: backends.RouteKindTCP},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := routeKindForStreamIR(tt.kind); got != tt.want {
				t.Fatalf("routeKindForStreamIR(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func serviceIRBackendRef(namespace string, name string, port uint32) ir.BackendRef {
	return ir.BackendRef{
		Kind:      "Service",
		Namespace: namespace,
		Name:      name,
		Port:      port,
	}
}

func serviceImportIRBackendRef(namespace string, name string, port uint32) ir.BackendRef {
	return ir.BackendRef{
		Group:     mcsv1alpha1.GroupName,
		Kind:      "ServiceImport",
		Namespace: namespace,
		Name:      name,
		Port:      port,
	}
}
