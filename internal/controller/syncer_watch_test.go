package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gwapi"
	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
)

func TestSnapshotReconcileRequestsSkipsUnreferencedSupportObjects(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name: "used-cert",
						}},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{{
					Filters: []gatewayv1.HTTPRouteFilter{{
						Type: gatewayv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gatewayv1.LocalObjectReference{
							Kind: "ConfigMap",
							Name: "used-filter",
						},
					}},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "unused-cert"}},
	); len(got) != 0 {
		t.Fatalf("expected unreferenced secret update to be ignored, got %v", got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "unused-filter"}},
	); len(got) != 0 {
		t.Fatalf("expected unreferenced configmap update to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueuesReferencedSupportObjects(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				TLS: &gatewayv1.GatewayTLSConfig{
					Frontend: &gatewayv1.FrontendTLSConfig{
						Default: gatewayv1.TLSConfig{
							Validation: &gatewayv1.FrontendTLSValidation{
								CACertificateRefs: []gatewayv1.ObjectReference{{
									Name: "used-ca",
								}},
							},
						},
					},
				},
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name: "used-cert",
						}},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{{
					Filters: []gatewayv1.HTTPRouteFilter{{
						Type: gatewayv1.HTTPRouteFilterExtensionRef,
						ExtensionRef: &gatewayv1.LocalObjectReference{
							Kind: "ConfigMap",
							Name: "used-filter",
						},
					}},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-cert"}},
	); len(got) != 1 || got[0] != snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
		Namespace: "default",
		Name:      "edge",
	}) {
		t.Fatalf("expected referenced secret update to queue gateway-listener rebuild, got %v", got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-ca"}},
	); len(got) != 1 || got[0] != snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
		Namespace: "default",
		Name:      "edge",
	}) {
		t.Fatalf("expected referenced frontend-validation configmap update to queue gateway-listener rebuild, got %v", got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-filter"}},
	); len(got) != 1 || got[0] != snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKey{
		Namespace: "default",
		Name:      "route",
	}) {
		t.Fatalf("expected referenced route configmap update to queue route-scoped rebuild, got %v", got)
	}
}

func TestSnapshotReconcileRequestsIgnoreGatewaySupportObjectsForUnmanagedGatewayOutsideCurrentSnapshot(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
				TLS: &gatewayv1.GatewayTLSConfig{
					Frontend: &gatewayv1.FrontendTLSConfig{
						Default: gatewayv1.TLSConfig{
							Validation: &gatewayv1.FrontendTLSValidation{
								CACertificateRefs: []gatewayv1.ObjectReference{{
									Name: "used-ca",
								}},
							},
						},
					},
				},
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name: "used-cert",
						}},
					},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-cert"}},
	); len(got) != 0 {
		t.Fatalf("expected secret referenced only by unmanaged gateway outside current snapshot to be ignored, got %v", got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-ca"}},
	); len(got) != 0 {
		t.Fatalf("expected frontend-validation configmap referenced only by unmanaged gateway outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsIgnoreFrontendValidationConfigMapForTLSPassthroughListener(t *testing.T) {
	passthrough := gatewayv1.TLSModePassthrough
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				TLS: &gatewayv1.GatewayTLSConfig{
					Frontend: &gatewayv1.FrontendTLSConfig{
						Default: gatewayv1.TLSConfig{
							Validation: &gatewayv1.FrontendTLSValidation{
								CACertificateRefs: []gatewayv1.ObjectReference{{
									Name: "used-ca",
								}},
							},
						},
					},
				},
				Listeners: []gatewayv1.Listener{{
					Name:     "tls-pass",
					Protocol: gatewayv1.TLSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &passthrough,
					},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-ca"}},
	); len(got) != 0 {
		t.Fatalf("expected frontend-validation configmap referenced only by TLS passthrough listener to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueGatewaySupportObjectsForTrackedGatewayLeavingManagedClass(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
				TLS: &gatewayv1.GatewayTLSConfig{
					Frontend: &gatewayv1.FrontendTLSConfig{
						Default: gatewayv1.TLSConfig{
							Validation: &gatewayv1.FrontendTLSValidation{
								CACertificateRefs: []gatewayv1.ObjectReference{{
									Name: "used-ca",
								}},
							},
						},
					},
				},
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name: "used-cert",
						}},
					},
				}},
			},
		},
	)
	if !syncer.store.Publish(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name: "default/edge/https",
		}},
	}) {
		t.Fatal("expected seed snapshot publish")
	}

	want := snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
		Namespace: "default",
		Name:      "edge",
	})

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-cert"}},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected secret for tracked gateway leaving managed class to queue gateway-listener rebuild %v, got %v", want, got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "used-ca"}},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected frontend-validation configmap for tracked gateway leaving managed class to queue gateway-listener rebuild %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueuesOnlyRelevantNamespaces(t *testing.T) {
	fromSelector := gatewayv1.NamespacesFromSelector
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("default"),
					}},
				},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	); len(got) != 1 || got[0] != snapshotAttachmentsReconcileRequest("apps") {
		t.Fatalf("expected namespace with attached routes to queue attachment-scoped rebuild, got %v", got)
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "unused"}},
	); len(got) != 0 {
		t.Fatalf("expected namespace without relevant routes to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsScopeNamespaceSelectorLookupsToChangedNamespace(t *testing.T) {
	fromSelector := gatewayv1.NamespacesFromSelector
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge-a", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge-b", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     8080,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge-a",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "team-http", Namespace: "team-a"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge-b",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)
	syncer.client = namespaceScopedRouteListValidatingClient{
		Client:    syncer.client,
		namespace: "apps",
	}

	got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	)
	want := []reconcile.Request{snapshotAttachmentsReconcileRequest("apps")}
	if !equalReconcileRequests(got, want) {
		t.Fatalf("snapshotReconcileRequests() = %#v, want %#v", got, want)
	}
}

func TestSnapshotReconcileRequestsIgnoreNamespaceForUnmanagedSelectorGatewayOutsideCurrentSnapshot(t *testing.T) {
	fromSelector := gatewayv1.NamespacesFromSelector
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("default"),
					}},
				},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	); len(got) != 0 {
		t.Fatalf("expected namespace referenced only by unmanaged selector gateway outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueNamespaceForTrackedSelectorGatewayLeavingManagedClass(t *testing.T) {
	fromSelector := gatewayv1.NamespacesFromSelector
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("default"),
					}},
				},
			},
		},
	)
	if !syncer.store.Publish(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name: "default/edge/http",
		}},
	}) {
		t.Fatal("expected seed snapshot publish")
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	); len(got) != 1 || got[0] != snapshotAttachmentsReconcileRequest("apps") {
		t.Fatalf("expected namespace for tracked selector gateway leaving managed class to queue attachment rebuild, got %v", got)
	}
}

func TestSnapshotReconcileRequestsIgnoreNamespaceWithRoutesOnlyForOtherGateways(t *testing.T) {
	fromSelector := gatewayv1.NamespacesFromSelector
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: &fromSelector,
						},
					},
				}},
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "other",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	); len(got) != 0 {
		t.Fatalf("expected namespace with routes only for other gateways to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueServiceImportDependencyRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	want := snapshotBackendDependenciesReconcileRequestForServiceImport(client.ObjectKey{
		Namespace: "default",
		Name:      "echo",
	})

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&mcsv1alpha1.ServiceImport{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"}},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected serviceimport update to queue backend dependency rebuild %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueServiceDependencyRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	want := snapshotServiceDependenciesReconcileRequestForService(client.ObjectKey{
		Namespace: "default",
		Name:      "echo",
	})

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"}},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected service update to queue service dependency rebuild %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueEndpointSliceBackendRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	want := snapshotBackendsReconcileRequestForService(client.ObjectKey{
		Namespace: "default",
		Name:      "echo",
	})

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "echo-1",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "echo",
				},
			},
		},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected EndpointSlice update to queue backend-only rebuild %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueServiceImportEndpointSliceBackendRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	want := snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
		Namespace: "default",
		Name:      "echo",
	})

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "echo-import-1",
				Namespace: "default",
				Labels: map[string]string{
					mcsv1alpha1.LabelServiceName: "echo",
				},
			},
		},
	); len(got) != 1 || got[0] != want {
		t.Fatalf("expected ServiceImport EndpointSlice update to queue backend-only rebuild %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueBackendLBPolicyNamespaceRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	policy := &backendlb.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "default"},
		Spec: backendlb.BackendLBPolicySpec{
			TargetRefs: []backendlb.LocalPolicyTargetReference{
				{
					Kind: "Service",
					Name: "echo",
				},
				{
					Group: gatewayv1.Group(mcsv1alpha1.GroupName),
					Kind:  "ServiceImport",
					Name:  "echo-import",
				},
			},
		},
	}
	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		policy,
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected BackendLBPolicy update to queue targeted backend rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueBackendTLSPolicyNamespaceRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)
	policy := mustEncodeBackendTLSPolicyV1ForWatchTest(t, &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: gatewayv1.Group(mcsv1alpha1.GroupName),
						Kind:  "ServiceImport",
						Name:  "echo-import",
					},
				},
			},
		},
	})
	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}

	if got := syncer.snapshotReconcileRequests(context.Background(), policy); !equalReconcileRequests(got, want) {
		t.Fatalf("expected BackendTLSPolicy update to queue targeted backend rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueBackendTLSPolicyConfigMapNamespaceRefresh(t *testing.T) {
	policy := mustEncodeBackendTLSPolicyV1ForWatchTest(t, &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: gatewayv1.Group(mcsv1alpha1.GroupName),
						Kind:  "ServiceImport",
						Name:  "echo-import",
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayv1.LocalObjectReference{{
					Name: "backend-ca",
				}},
			},
		},
	})
	syncer := newIndexedWatchTestSyncer(
		t,
		policy.DeepCopy(),
	)
	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backend-ca"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected BackendTLSPolicy ConfigMap update to queue targeted backend rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsUsesIndexedBackendTLSPolicyConfigMapLookup(t *testing.T) {
	policy := mustEncodeBackendTLSPolicyV1ForWatchTest(t, &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: gatewayv1.Group(mcsv1alpha1.GroupName),
						Kind:  "ServiceImport",
						Name:  "echo-import",
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayv1.LocalObjectReference{{
					Name: "backend-ca",
				}},
			},
		},
	})

	syncer := newIndexedWatchTestSyncer(t, policy.DeepCopy())
	syncer.client = &partialRebuildValidatingClient{
		Client: syncer.client,
		forbiddenLists: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1alpha3.BackendTLSPolicyList{}): "indexed ConfigMap lookup should not list typed BackendTLSPolicies",
		},
	}

	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backend-ca"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected indexed BackendTLSPolicy ConfigMap lookup to avoid typed namespace list and queue %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsFallsBackWhenBackendTLSPolicyConfigMapFieldSelectorUnsupported(t *testing.T) {
	policy := mustEncodeBackendTLSPolicyV1ForWatchTest(t, &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: gatewayv1.Group(mcsv1alpha1.GroupName),
						Kind:  "ServiceImport",
						Name:  "echo-import",
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayv1.LocalObjectReference{{
					Name: "backend-ca",
				}},
			},
		},
	})

	syncer := newIndexedWatchTestSyncer(t, policy.DeepCopy())
	syncer.client = backendTLSPolicyFieldSelectorRejectingClient{Client: syncer.client}

	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backend-ca"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected BackendTLSPolicy ConfigMap lookup to fall back from unsupported field selectors and queue %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsDisablesUnsupportedBackendTLSPolicyConfigMapIndex(t *testing.T) {
	policy := mustEncodeBackendTLSPolicyV1ForWatchTest(t, &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: gatewayv1.Group(mcsv1alpha1.GroupName),
						Kind:  "ServiceImport",
						Name:  "echo-import",
					},
				},
			},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayv1.LocalObjectReference{{
					Name: "backend-ca",
				}},
			},
		},
	})

	syncer := newIndexedWatchTestSyncer(t, policy.DeepCopy())
	rejecting := &countingBackendTLSPolicyFieldSelectorRejectingClient{Client: syncer.client}
	syncer.client = rejecting

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ingress-nginx", Name: "ingress-controller-leader-nginx"}},
	); len(got) != 0 {
		t.Fatalf("expected unrelated ConfigMap update to be ignored after fallback lookup, got %v", got)
	}
	if attempts := rejecting.FieldSelectorLists(); attempts != 1 {
		t.Fatalf("expected one BackendTLSPolicy field-selector attempt after first ConfigMap, got %d", attempts)
	}
	if syncer.backendTLSPolicyConfigMapIndex {
		t.Fatal("expected unsupported BackendTLSPolicy ConfigMap index to be disabled after first field-selector failure")
	}

	want := []reconcile.Request{
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
		snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo-import",
		}),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backend-ca"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected disabled BackendTLSPolicy ConfigMap index to fall back and queue %v, got %v", want, got)
	}
	if attempts := rejecting.FieldSelectorLists(); attempts != 1 {
		t.Fatalf("expected disabled BackendTLSPolicy ConfigMap index to avoid repeat field-selector attempts, got %d", attempts)
	}
}

func TestSnapshotReconcileRequestsQueueReferenceGrantScopedRefresh(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	sharedNamespace := gatewayv1.Namespace("shared")

	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name:      "shared-cert",
							Namespace: &sharedNamespace,
						}},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "shared-http",
								Namespace: &sharedNamespace,
							},
						},
					}},
				}},
			},
		},
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "grpc", Namespace: "default"},
			Spec: gatewayv1.GRPCRouteSpec{
				Rules: []gatewayv1.GRPCRouteRule{{
					BackendRefs: []gatewayv1.GRPCBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "shared-grpc",
								Namespace: &sharedNamespace,
							},
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "tcp", Namespace: "default"},
			Spec: gatewayv1alpha2.TCPRouteSpec{
				Rules: []gatewayv1alpha2.TCPRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-tcp",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.UDPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "udp", Namespace: "default"},
			Spec: gatewayv1alpha2.UDPRouteSpec{
				Rules: []gatewayv1alpha2.UDPRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-udp",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "default"},
			Spec: gatewayv1alpha2.TLSRouteSpec{
				Rules: []gatewayv1alpha2.TLSRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-tls",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
	)

	want := []reconcile.Request{
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "edge"}),
		snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "http"}),
		snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "grpc"}),
		snapshotTCPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "tcp"}),
		snapshotUDPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "udp"}),
		snapshotTLSRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "tls"}),
	}

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "shared"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected ReferenceGrant update to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsReferenceGrantLookupRunsRouteQueriesConcurrently(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	sharedNamespace := gatewayv1.Namespace("shared")

	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name:      "shared-cert",
							Namespace: &sharedNamespace,
						}},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "shared-http",
								Namespace: &sharedNamespace,
							},
						},
					}},
				}},
			},
		},
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "grpc", Namespace: "default"},
			Spec: gatewayv1.GRPCRouteSpec{
				Rules: []gatewayv1.GRPCRouteRule{{
					BackendRefs: []gatewayv1.GRPCBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "shared-grpc",
								Namespace: &sharedNamespace,
							},
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "tcp", Namespace: "default"},
			Spec: gatewayv1alpha2.TCPRouteSpec{
				Rules: []gatewayv1alpha2.TCPRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-tcp",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.UDPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "udp", Namespace: "default"},
			Spec: gatewayv1alpha2.UDPRouteSpec{
				Rules: []gatewayv1alpha2.UDPRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-udp",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
		&gatewayv1alpha2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "default"},
			Spec: gatewayv1alpha2.TLSRouteSpec{
				Rules: []gatewayv1alpha2.TLSRouteRule{{
					BackendRefs: []gatewayv1.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "shared-tls",
							Namespace: &sharedNamespace,
						},
					}},
				}},
			},
		},
	)
	syncer.client = &blockingReferenceGrantLookupClient{
		Client:       syncer.client,
		expectedList: 6,
		release:      make(chan struct{}),
	}

	want := []reconcile.Request{
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "edge"}),
		snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "http"}),
		snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "grpc"}),
		snapshotTCPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "tcp"}),
		snapshotUDPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "udp"}),
		snapshotTLSRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "tls"}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	got := syncer.snapshotReconcileRequests(
		ctx,
		&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "shared"}},
	)
	if !equalReconcileRequests(got, want) {
		t.Fatalf("expected concurrent ReferenceGrant lookup to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsSkipsUnreferencedReferenceGrant(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name: "local-cert",
						}},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "local-http",
							},
						},
					}},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Name: "unused", Namespace: "shared"}},
	); len(got) != 0 {
		t.Fatalf("expected unreferenced ReferenceGrant update to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsIgnoreReferenceGrantForUnmanagedGatewayOutsideCurrentSnapshot(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	sharedNamespace := gatewayv1.Namespace("shared")

	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
				Listeners: []gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name:      "shared-cert",
							Namespace: &sharedNamespace,
						}},
					},
				}},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "shared"}},
	); len(got) != 0 {
		t.Fatalf("expected ReferenceGrant update affecting only unmanaged gateway outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueGatewayScopedRefreshes(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "team-grpc", Namespace: "team-a"},
			Spec: gatewayv1.GRPCRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "other-http", Namespace: "other"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "other",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
		snapshotAttachmentsReconcileRequest("team-a"),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"}, Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
		}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected gateway update to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueScopedRefreshForListenerSet(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			},
		},
		&gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "listener-set", Namespace: "apps"},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "edge",
					Namespace: ptr(gatewayv1.Namespace("infra")),
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "listener-set-route", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
						Kind:      ptr(gatewayv1.Kind("ListenerSet")),
						Name:      "listener-set",
						Namespace: ptr(gatewayv1.Namespace("apps")),
					}},
				},
			},
		},
	)

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "listener-set", Namespace: "apps"},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "edge",
					Namespace: ptr(gatewayv1.Namespace("infra")),
				},
			},
		},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected ListenerSet update to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueGatewayClassScopedRefreshes(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
		&gatewayv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "team-grpc", Namespace: "team-a"},
			Spec: gatewayv1.GRPCRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "other-http", Namespace: "other"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "other",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
		snapshotAttachmentsReconcileRequest("team-a"),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected gatewayclass update to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsIgnoreUnusedGatewayClass(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"}},
	); len(got) != 0 {
		t.Fatalf("expected unreferenced gatewayclass update to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsIgnoreGatewayClassForUnmanagedGatewayOutsideCurrentSnapshot(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
	); len(got) != 0 {
		t.Fatalf("expected GatewayClass update affecting only unmanaged gateway outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueGatewayClassScopedRefreshForTrackedGatewayLeavingManagedClass(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)
	if !syncer.store.Publish(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name: "infra/edge/http",
		}},
	}) {
		t.Fatal("expected seed snapshot publish")
	}

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected tracked GatewayClass update to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsIgnoreUnmanagedGatewayOutsideCurrentSnapshot(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
	)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
			},
		},
	); len(got) != 0 {
		t.Fatalf("expected unmanaged gateway outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueScopedRefreshForGatewayLeavingManagedClass(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/other-controller",
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)
	if !syncer.store.Publish(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name: "infra/edge/http",
		}},
	}) {
		t.Fatal("expected seed snapshot publish")
	}

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "other",
			},
		},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected gateway leaving managed class to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsIgnoreGatewayWithMissingGatewayClassOutsideCurrentSnapshot(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)

	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "missing",
			},
		},
	); len(got) != 0 {
		t.Fatalf("expected gateway with missing class outside current snapshot to be ignored, got %v", got)
	}
}

func TestSnapshotReconcileRequestsQueueScopedRefreshForTrackedGatewayWithMissingGatewayClass(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(
		t,
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "apps-http", Namespace: "apps"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name:      "edge",
						Namespace: ptr[gatewayv1.Namespace]("infra"),
					}},
				},
			},
		},
	)
	if !syncer.store.Publish(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name: "infra/edge/http",
		}},
	}) {
		t.Fatal("expected seed snapshot publish")
	}

	want := []reconcile.Request{
		snapshotAttachmentsReconcileRequest("apps"),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "infra",
			Name:      "edge",
		}),
	}
	if got := syncer.snapshotReconcileRequests(
		context.Background(),
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "infra"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "missing",
			},
		},
	); !equalReconcileRequests(got, want) {
		t.Fatalf("expected tracked gateway with missing class to queue scoped rebuilds %v, got %v", want, got)
	}
}

func TestSnapshotReconcileRequestsQueueRouteScopedRefresh(t *testing.T) {
	syncer := newIndexedWatchTestSyncer(t)

	tests := []struct {
		name   string
		object client.Object
		want   reconcile.Request
	}{
		{
			name:   "HTTPRoute",
			object: &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "default"}},
			want: snapshotScopedObjectReconcileRequest("snapshot-routes-http", client.ObjectKey{
				Namespace: "default",
				Name:      "http",
			}),
		},
		{
			name:   "GRPCRoute",
			object: &gatewayv1.GRPCRoute{ObjectMeta: metav1.ObjectMeta{Name: "grpc", Namespace: "default"}},
			want: snapshotScopedObjectReconcileRequest("snapshot-routes-grpc", client.ObjectKey{
				Namespace: "default",
				Name:      "grpc",
			}),
		},
		{
			name:   "TCPRoute",
			object: &gatewayv1alpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Name: "tcp", Namespace: "default"}},
			want: snapshotScopedObjectReconcileRequest("snapshot-routes-tcp", client.ObjectKey{
				Namespace: "default",
				Name:      "tcp",
			}),
		},
		{
			name:   "UDPRoute",
			object: &gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{Name: "udp", Namespace: "default"}},
			want: snapshotScopedObjectReconcileRequest("snapshot-routes-udp", client.ObjectKey{
				Namespace: "default",
				Name:      "udp",
			}),
		},
		{
			name:   "TLSRoute",
			object: &gatewayv1alpha2.TLSRoute{ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "default"}},
			want: snapshotScopedObjectReconcileRequest("snapshot-routes-tls", client.ObjectKey{
				Namespace: "default",
				Name:      "tls",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := syncer.snapshotReconcileRequests(context.Background(), tt.object); len(got) != 1 || got[0] != tt.want {
				t.Fatalf("expected %s update to queue route-scoped rebuild %v, got %v", tt.name, tt.want, got)
			}
		})
	}
}

func newIndexedWatchTestSyncer(t *testing.T, objects ...client.Object) *Syncer {
	t.Helper()

	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha3.Install)
	mustAddToScheme(t, scheme, backendlb.Install)

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.Gateway{}, "nantian.dev/infrastructure.gateway.gatewayclass-name", func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithObjects(objects...).
		WithIndex(&gatewayv1.Gateway{}, gatewaySecretReferenceIndex, gatewaySecretReferenceIndexKeys).
		WithIndex(&gatewayv1.Gateway{}, gatewayConfigMapReferenceIndex, gatewayConfigMapReferenceIndexKeys).
		WithIndex(&gatewayv1.Gateway{}, gatewayReferenceGrantNamespaceIndex, gatewayReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1.Gateway{}, gatewayNamespaceSelectorIndex, gatewayNamespaceSelectorIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, httpRouteConfigMapReferenceIndex, httpRouteConfigMapReferenceIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, httpRouteReferenceGrantNamespaceIndex, httpRouteReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, httpRouteParentGatewayIndex, httpRouteParentGatewayIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, grpcRouteConfigMapReferenceIndex, grpcRouteConfigMapReferenceIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, grpcRouteReferenceGrantNamespaceIndex, grpcRouteReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, grpcRouteParentGatewayIndex, grpcRouteParentGatewayIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, tcpRouteReferenceGrantNamespaceIndex, tcpRouteReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, tcpRouteParentGatewayIndex, tcpRouteParentGatewayIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, udpRouteReferenceGrantNamespaceIndex, udpRouteReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, udpRouteParentGatewayIndex, udpRouteParentGatewayIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, tlsRouteReferenceGrantNamespaceIndex, tlsRouteReferenceGrantNamespaceIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, tlsRouteParentGatewayIndex, tlsRouteParentGatewayIndexKeys).
		WithIndex(
			gwapi.NewBackendTLSPolicyV1Object(),
			backendTLSPolicyConfigMapRefIndex,
			backendTLSPolicyConfigMapReferenceIndexKeys,
		)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	syncer := NewSyncer(
		clientBuilder.Build(),
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		ir.NewSnapshotStore(logger),
		testMetrics(),
		time.Minute,
		logger,
	)
	syncer.setBackendTLSPolicyConfigMapIndexAvailable(true)
	return syncer
}

func mustEncodeBackendTLSPolicyV1ForWatchTest(
	t *testing.T,
	policy *gatewayv1alpha3.BackendTLSPolicy,
) *unstructured.Unstructured {
	t.Helper()

	raw, err := gwapi.EncodeBackendTLSPolicyV1(policy)
	if err != nil {
		t.Fatalf("encode BackendTLSPolicy v1: %v", err)
	}
	return raw
}

func equalReconcileRequests(got, want []reconcile.Request) bool {
	if len(got) != len(want) {
		return false
	}
	got = append([]reconcile.Request(nil), got...)
	want = append([]reconcile.Request(nil), want...)
	sortReconcileRequests(got)
	sortReconcileRequests(want)
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sortReconcileRequests(items []reconcile.Request) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i].Namespace + "/" + items[i].Name
		right := items[j].Namespace + "/" + items[j].Name
		return left < right
	})
}

type backendTLSPolicyFieldSelectorRejectingClient struct {
	client.Client
}

type countingBackendTLSPolicyFieldSelectorRejectingClient struct {
	client.Client
	mu                 sync.Mutex
	fieldSelectorLists int
}

type namespaceScopedRouteListValidatingClient struct {
	client.Client
	namespace string
}

func (c namespaceScopedRouteListValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch list.(type) {
	case *gatewayv1.HTTPRouteList,
		*gatewayv1.GRPCRouteList,
		*gatewayv1alpha2.TCPRouteList,
		*gatewayv1alpha2.UDPRouteList,
		*gatewayv1alpha2.TLSRouteList:
		var listOptions client.ListOptions
		for _, opt := range opts {
			opt.ApplyToList(&listOptions)
		}
		if listOptions.Namespace != c.namespace {
			return fmt.Errorf("route list namespace = %q, want %q", listOptions.Namespace, c.namespace)
		}
	}

	return c.Client.List(ctx, list, opts...)
}

func (c backendTLSPolicyFieldSelectorRejectingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	typed, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return c.Client.List(ctx, list, opts...)
	}
	if typed.GroupVersionKind() != gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
		return c.Client.List(ctx, list, opts...)
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector != nil && !listOptions.FieldSelector.Empty() {
		return fmt.Errorf("field label not supported: %s", backendTLSPolicyConfigMapRefIndex)
	}

	return c.Client.List(ctx, list, opts...)
}

func (c *countingBackendTLSPolicyFieldSelectorRejectingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	typed, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return c.Client.List(ctx, list, opts...)
	}
	if typed.GroupVersionKind() != gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
		return c.Client.List(ctx, list, opts...)
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector != nil && !listOptions.FieldSelector.Empty() {
		c.mu.Lock()
		c.fieldSelectorLists++
		c.mu.Unlock()
		return fmt.Errorf("field label not supported: %s", backendTLSPolicyConfigMapRefIndex)
	}

	return c.Client.List(ctx, list, opts...)
}

func (c *countingBackendTLSPolicyFieldSelectorRejectingClient) FieldSelectorLists() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fieldSelectorLists
}

type blockingReferenceGrantLookupClient struct {
	client.Client
	expectedList int
	release      chan struct{}
	mu           sync.Mutex
	started      int
}

func (c *blockingReferenceGrantLookupClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if !blocksReferenceGrantLookupList(list) {
		return c.Client.List(ctx, list, opts...)
	}

	c.mu.Lock()
	c.started++
	if c.started == c.expectedList {
		close(c.release)
	}
	c.mu.Unlock()

	select {
	case <-c.release:
	case <-ctx.Done():
		return ctx.Err()
	}

	return c.Client.List(ctx, list, opts...)
}

func blocksReferenceGrantLookupList(list client.ObjectList) bool {
	switch list.(type) {
	case *gatewayv1.GatewayList,
		*gatewayv1.HTTPRouteList,
		*gatewayv1.GRPCRouteList,
		*gatewayv1alpha2.TCPRouteList,
		*gatewayv1alpha2.UDPRouteList,
		*gatewayv1alpha2.TLSRouteList:
		return true
	default:
		return false
	}
}
