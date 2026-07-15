package status

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
)

type fakeFieldIndexer struct {
	errs    map[string]error
	objects []client.Object
}

func (f *fakeFieldIndexer) IndexField(
	_ context.Context,
	object client.Object,
	field string,
	_ client.IndexerFunc,
) error {
	if f != nil {
		f.objects = append(f.objects, object)
	}
	if f == nil || f.errs == nil {
		return nil
	}
	return f.errs[field]
}

func TestSetupIndexesStandardModeSkipsExperimentalRouteIndexes(t *testing.T) {
	indexer := &fakeFieldIndexer{}

	if err := SetupIndexes(context.Background(), indexer, Options{EnableExperimentalGateway: false}); err != nil {
		t.Fatalf("SetupIndexes returned error: %v", err)
	}

	for _, object := range indexer.objects {
		switch object.(type) {
		case *gatewayv1alpha2.TCPRoute, *gatewayv1alpha2.UDPRoute, *gatewayv1alpha2.TLSRoute:
			t.Fatalf("standard mode registered experimental route index for %T", object)
		}
	}
}

func TestSetupIndexesExperimentalModeIncludesExperimentalRouteIndexes(t *testing.T) {
	indexer := &fakeFieldIndexer{}

	if err := SetupIndexes(context.Background(), indexer, Options{EnableExperimentalGateway: true}); err != nil {
		t.Fatalf("SetupIndexes returned error: %v", err)
	}

	seen := map[reflect.Type]bool{}
	for _, object := range indexer.objects {
		seen[reflect.TypeOf(object)] = true
	}

	for _, want := range []client.Object{
		&gatewayv1alpha2.TCPRoute{},
		&gatewayv1alpha2.UDPRoute{},
		&gatewayv1alpha2.TLSRoute{},
	} {
		if !seen[reflect.TypeOf(want)] {
			t.Fatalf("experimental mode did not register index for %T", want)
		}
	}
}

func TestSetupIndexesIgnoresMissingStatusBackendLBPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendLBPolicyTargetRefIndex: &meta.NoKindMatchError{
				GroupKind: schema.GroupKind{
					Group: backendlb.GroupVersion.Group,
					Kind:  "BackendLBPolicy",
				},
				SearchedVersions: []string{backendlb.GroupVersion.Version},
			},
		},
	}

	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatalf("SetupIndexes returned error for missing optional BackendLBPolicy CRD: %v", err)
	}
}

func TestSetupIndexesIgnoresMissingStatusBackendTLSPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendTLSPolicyTargetRefIndex: &meta.NoKindMatchError{
				GroupKind: gatewayapi.BackendTLSPolicyV1GVK.GroupKind(),
				SearchedVersions: []string{
					gatewayapi.BackendTLSPolicyV1GVK.Version,
				},
			},
		},
	}

	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatalf("SetupIndexes returned error for missing optional BackendTLSPolicy CRD: %v", err)
	}
}

func TestSetupIndexesReturnsUnexpectedStatusBackendLBPolicyIndexError(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendLBPolicyTargetRefIndex: errors.New("boom"),
		},
	}

	err := SetupIndexes(context.Background(), indexer)
	if err == nil {
		t.Fatal("SetupIndexes returned nil error, want wrapped BackendLBPolicy index failure")
	}
	if !strings.Contains(err.Error(), "index BackendLBPolicy target refs: boom") {
		t.Fatalf("SetupIndexes returned %q, want BackendLBPolicy index failure", err)
	}
}

func TestStatusBackendPolicyTargetRefIndexKeysDeduplicateAndSortValues(t *testing.T) {
	raw, err := gatewayapi.EncodeBackendTLSPolicyV1(&gatewayv1alpha3.BackendTLSPolicy{
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: mcsv1alpha1.GroupName,
						Kind:  "ServiceImport",
						Name:  "imported",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("EncodeBackendTLSPolicyV1 returned error: %v", err)
	}

	gotTLS := statusBackendTLSPolicyTargetRefIndexKeys(raw)
	wantTLS := []string{
		backendPolicyTargetRefIndexValue("", "Service", "echo"),
		backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "imported"),
	}
	if !reflect.DeepEqual(gotTLS, wantTLS) {
		t.Fatalf("statusBackendTLSPolicyTargetRefIndexKeys() = %#v, want %#v", gotTLS, wantTLS)
	}

	gotLB := statusBackendLBPolicyTargetRefIndexKeys(&backendlb.BackendLBPolicy{
		Spec: backendlb.BackendLBPolicySpec{
			TargetRefs: []backendlb.LocalPolicyTargetReference{
				{Group: mcsv1alpha1.GroupName, Kind: "ServiceImport", Name: "imported"},
				{Kind: "Service", Name: "echo"},
				{Kind: "Service", Name: "echo"},
			},
		},
	})
	wantLB := []string{
		backendPolicyTargetRefIndexValue("", "Service", "echo"),
		backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "imported"),
	}
	if !reflect.DeepEqual(gotLB, wantLB) {
		t.Fatalf("statusBackendLBPolicyTargetRefIndexKeys() = %#v, want %#v", gotLB, wantLB)
	}
}

func TestCollectRouteBackendPolicyRefsIncludesAllRouteKinds(t *testing.T) {
	state := &clusterState{
		httpRoutes: []gatewayv1.HTTPRoute{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "http"},
			Spec: gatewayv1.HTTPRouteSpec{Rules: []gatewayv1.HTTPRouteRule{{
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: serviceBackendRef("orders"),
				}},
			}}},
		}},
		grpcRoutes: []gatewayv1.GRPCRoute{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "grpc"},
			Spec: gatewayv1.GRPCRouteSpec{Rules: []gatewayv1.GRPCRouteRule{{
				BackendRefs: []gatewayv1.GRPCBackendRef{{
					BackendRef: serviceImportBackendRef("imports", "inventory"),
				}},
			}}},
		}},
		tcpRoutes: []gatewayv1alpha2.TCPRoute{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "tcp", Name: "tcp"},
			Spec: gatewayv1alpha2.TCPRouteSpec{Rules: []gatewayv1alpha2.TCPRouteRule{{
				BackendRefs: []gatewayv1alpha2.BackendRef{serviceBackendRef("tcp-backend")},
			}}},
		}},
		udpRoutes: []gatewayv1alpha2.UDPRoute{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "udp", Name: "udp"},
			Spec: gatewayv1alpha2.UDPRouteSpec{Rules: []gatewayv1alpha2.UDPRouteRule{{
				BackendRefs: []gatewayv1alpha2.BackendRef{serviceBackendRef("udp-backend")},
			}}},
		}},
		tlsRoutes: []gatewayv1alpha2.TLSRoute{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "tls", Name: "tls"},
			Spec: gatewayv1alpha2.TLSRouteSpec{Rules: []gatewayv1alpha2.TLSRouteRule{{
				BackendRefs: []gatewayv1.BackendRef{serviceImportBackendRef("tls", "tls-import")},
			}}},
		}},
	}

	services, serviceImports := collectRouteBackendPolicyRefs(state)

	wantServices := map[string]client.ObjectKey{
		"apps/orders":     {Namespace: "apps", Name: "orders"},
		"tcp/tcp-backend": {Namespace: "tcp", Name: "tcp-backend"},
		"udp/udp-backend": {Namespace: "udp", Name: "udp-backend"},
	}
	if !reflect.DeepEqual(services, wantServices) {
		t.Fatalf("services = %#v, want %#v", services, wantServices)
	}

	wantServiceImports := map[string]client.ObjectKey{
		"imports/inventory": {Namespace: "imports", Name: "inventory"},
		"tls/tls-import":    {Namespace: "tls", Name: "tls-import"},
	}
	if !reflect.DeepEqual(serviceImports, wantServiceImports) {
		t.Fatalf("serviceImports = %#v, want %#v", serviceImports, wantServiceImports)
	}
}

func TestBackendPolicyTargetRefIndexValuesByNamespace(t *testing.T) {
	values := backendPolicyTargetRefIndexValuesByNamespace(
		map[string]client.ObjectKey{
			"apps/orders": {Namespace: "apps", Name: "orders"},
			"apps/users":  {Namespace: "apps", Name: "users"},
		},
		map[string]client.ObjectKey{
			"imports/inventory": {Namespace: "imports", Name: "inventory"},
			"apps/catalog":      {Namespace: "apps", Name: "catalog"},
		},
	)

	want := map[string][]string{
		"apps": {
			backendPolicyTargetRefIndexValue("", "Service", "orders"),
			backendPolicyTargetRefIndexValue("", "Service", "users"),
			backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "catalog"),
		},
		"imports": {
			backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "inventory"),
		},
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("backendPolicyTargetRefIndexValuesByNamespace() = %#v, want %#v", values, want)
	}
}

func TestBackendPolicyTouchesKeys(t *testing.T) {
	serviceKeys := map[string]client.ObjectKey{
		"apps/orders": {Namespace: "apps", Name: "orders"},
	}
	serviceImportKeys := map[string]client.ObjectKey{
		"apps/catalog": {Namespace: "apps", Name: "catalog"},
	}

	tlsRefs := []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
		{LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{Kind: "Service", Name: "missing"}},
		{LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{Kind: "Service", Name: "orders"}},
	}
	if !backendTLSPolicyTouchesKeys("apps", tlsRefs, serviceKeys, serviceImportKeys) {
		t.Fatal("expected BackendTLSPolicy to touch orders service")
	}

	lbRefs := []backendlb.LocalPolicyTargetReference{
		{Group: mcsv1alpha1.GroupName, Kind: "ServiceImport", Name: "catalog"},
	}
	if !backendLBPolicyTouchesKeys("apps", lbRefs, serviceKeys, serviceImportKeys) {
		t.Fatal("expected BackendLBPolicy to touch catalog ServiceImport")
	}

	if backendLBPolicyTouchesKeys(
		"apps",
		[]backendlb.LocalPolicyTargetReference{{Kind: "Service", Name: "missing"}},
		serviceKeys,
		serviceImportKeys,
	) {
		t.Fatal("unexpected BackendLBPolicy match for missing service")
	}
}

func serviceBackendRef(name string) gatewayv1.BackendRef {
	return gatewayv1.BackendRef{
		BackendObjectReference: gatewayv1.BackendObjectReference{
			Name: gatewayv1.ObjectName(name),
		},
	}
}

func serviceImportBackendRef(namespace string, name string) gatewayv1.BackendRef {
	return gatewayv1.BackendRef{
		BackendObjectReference: gatewayv1.BackendObjectReference{
			Group:     ptr(gatewayv1.Group(mcsv1alpha1.GroupName)),
			Kind:      ptr(gatewayv1.Kind("ServiceImport")),
			Namespace: namespacePtr(namespace),
			Name:      gatewayv1.ObjectName(name),
		},
	}
}
