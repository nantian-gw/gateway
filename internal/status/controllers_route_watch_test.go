package status

import (
	"context"
	"reflect"
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func TestHTTPRouteStatusRequestsForServiceEnqueuesBackendAndServiceParentRoutes(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteServiceParentIndex, statusHTTPRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteBackendRefIndex, statusHTTPRouteBackendRefIndexKeys).
		WithObjects(
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "backend-route", Namespace: "consumer"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("default"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "service-parent-route", Namespace: "consumer"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group("")),
							Kind:      ptr(gatewayv1.Kind("Service")),
							Name:      "echo",
							Namespace: namespacePtr("default"),
						}},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "other-route", Namespace: "consumer"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "other",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	got := httpRouteStatusRequestsForService(
		context.Background(),
		k8sClient,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"}},
	)

	want := []reconcile.Request{
		{NamespacedName: types.NamespacedName{Namespace: "consumer", Name: "backend-route"}},
		{NamespacedName: types.NamespacedName{Namespace: "consumer", Name: "service-parent-route"}},
	}
	if !reflect.DeepEqual(sortedRequests(got), sortedRequests(want)) {
		t.Fatalf("httpRouteStatusRequestsForService() = %#v, want %#v", sortedRequests(got), sortedRequests(want))
	}
}

func TestHTTPRouteStatusRequestsForServiceImportEnqueuesBackendRoutes(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteBackendRefIndex, statusHTTPRouteBackendRefIndexKeys).
		WithObjects(
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "import-route", Namespace: "consumer"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Group:     ptr(gatewayv1.Group(mcsv1alpha1.GroupName)),
									Kind:      ptr(gatewayv1.Kind("ServiceImport")),
									Name:      "echo",
									Namespace: namespacePtr("default"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "service-route", Namespace: "consumer"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("default"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	got := httpRouteStatusRequestsForServiceImport(
		context.Background(),
		k8sClient,
		&mcsv1alpha1.ServiceImport{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"}},
	)

	want := []reconcile.Request{
		{NamespacedName: types.NamespacedName{Namespace: "consumer", Name: "import-route"}},
	}
	if !reflect.DeepEqual(sortedRequests(got), sortedRequests(want)) {
		t.Fatalf("httpRouteStatusRequestsForServiceImport() = %#v, want %#v", sortedRequests(got), sortedRequests(want))
	}
}

func TestGatewayListenerSetStatusRequestsDefaultsParentNamespace(t *testing.T) {
	t.Parallel()

	got := gatewayListenerSetStatusRequests(
		context.Background(),
		&gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			},
		},
	)

	want := []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gatewayListenerSetStatusRequests() = %#v, want %#v", got, want)
	}
}

func sortedRequests(items []reconcile.Request) []reconcile.Request {
	out := append([]reconcile.Request(nil), items...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}
