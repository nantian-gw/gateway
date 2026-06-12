package status

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/infrastructure"
)

func TestReconcileGatewayObjectAvoidsFullDependencyLists(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
		WithStatusSubresource(&gatewayv1.Gateway{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
				},
			},
		).
		Build()

	reader := &countingGetReader{Reader: rawValidatingReader{
		Reader: gatewayParentFilteringReader{Reader: restrictedReader{
			Reader: freshReader,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "gateway object reconcile should direct-Get referenced GatewayClasses",
				reflect.TypeOf(&gatewayv1.GatewayList{}):             "gateway object reconcile should read the target Gateway directly",
				reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "gateway object reconcile should load ReferenceGrants from cache-backed listReader",
				reflect.TypeOf(&corev1.ServiceList{}):                "gateway object reconcile should read its Service directly",
				reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "gateway object reconcile should not need ServiceImports",
				reflect.TypeOf(&corev1.NamespaceList{}):              "gateway object reconcile should fetch route Namespaces directly",
				reflect.TypeOf(&corev1.ConfigMapList{}):              "gateway object reconcile should fetch referenced ConfigMaps directly",
				reflect.TypeOf(&corev1.SecretList{}):                 "gateway object reconcile should fetch referenced Secrets directly",
			},
		}},
		listValidators: map[reflect.Type]func([]client.ListOption) error{
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}): func(opts []client.ListOption) error {
				return requireHTTPRouteGatewayOrListenerSetParentMatchingField(opts, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1.GRPCRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusGRPCRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusTCPRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusUDPRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusTLSRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&discoveryv1.EndpointSliceList{}): func(opts []client.ListOption) error {
				if err := requireNamespaceOption(opts, "default"); err != nil {
					return err
				}

				var listOptions client.ListOptions
				for _, opt := range opts {
					opt.ApplyToList(&listOptions)
				}
				if listOptions.LabelSelector == nil || listOptions.LabelSelector.Empty() {
					return fmt.Errorf("endpoint slice list must include a service label selector")
				}
				if !listOptions.LabelSelector.Matches(labels.Set{
					discoveryv1.LabelServiceName: infrastructure.GatewayServiceName("gw"),
				}) {
					return fmt.Errorf(
						"endpoint slice selector = %q does not match service %q",
						listOptions.LabelSelector.String(),
						infrastructure.GatewayServiceName("gw"),
					)
				}
				return nil
			},
		},
	}}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = restrictedReader{
		Reader: staleClient,
		blockedListTypes: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}): "gateway object reconcile should not list GatewayClasses from listReader",
		},
	}
	if err := reconciler.ReconcileGatewayObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "gw"},
	); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}
	if reader.gatewayClassGets != 1 {
		t.Fatalf("GatewayClass reader Get count = %d, want 1", reader.gatewayClassGets)
	}
}

func TestReconcileGatewayObjectListsReferenceGrantsPerReferencedNamespace(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	seenGrantNamespaces := make(map[string]int)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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
								Namespace: namespacePtr("certs"),
							}},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("shared"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-listener-set-cert", Namespace: "listener-certs"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("ListenerSet"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr(gatewayv1beta1.ObjectName("listener-cert")),
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-route-backend", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
						Name:  ptr(gatewayv1beta1.ObjectName("echo")),
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-gateway-cert", Namespace: "certs"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr(gatewayv1beta1.ObjectName("shared-cert")),
					}},
				},
			},
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "certs"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "listener-certs"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
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
								Namespace: namespacePtr("certs"),
							}},
						},
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     8443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name:      "listener-cert",
								Namespace: namespacePtr("listener-certs"),
							}},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("shared"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "shared"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-cert", Namespace: "certs"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
		).
		Build()

	reader := &countingGetReader{Reader: rawValidatingReader{
		Reader: gatewayParentFilteringReader{Reader: restrictedReader{
			Reader: freshReader,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "gateway object reconcile should direct-Get referenced GatewayClasses",
				reflect.TypeOf(&gatewayv1.GatewayList{}):             "gateway object reconcile should read the target Gateway directly",
				reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "gateway object reconcile should load ReferenceGrants from cache-backed listReader",
				reflect.TypeOf(&corev1.ServiceList{}):                "gateway object reconcile should read referenced Services directly",
				reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "gateway object reconcile should not need ServiceImports",
				reflect.TypeOf(&corev1.NamespaceList{}):              "gateway object reconcile should fetch route Namespaces directly",
				reflect.TypeOf(&corev1.ConfigMapList{}):              "gateway object reconcile should fetch referenced ConfigMaps directly",
				reflect.TypeOf(&corev1.SecretList{}):                 "gateway object reconcile should fetch referenced Secrets directly",
			},
		}},
		listValidators: map[reflect.Type]func([]client.ListOption) error{
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}): func(opts []client.ListOption) error {
				return requireHTTPRouteGatewayOrListenerSetParentMatchingField(opts, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1.GRPCRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusGRPCRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusTCPRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusUDPRouteGatewayParentIndex, "default", "gw")
			},
			reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): func(opts []client.ListOption) error {
				return requireGatewayParentMatchingField(opts, statusTLSRouteGatewayParentIndex, "default", "gw")
			},
		},
	}}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = rawValidatingReader{
		Reader: restrictedReader{
			Reader: staleClient,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}): "gateway object reconcile should not list GatewayClasses from listReader",
			},
		},
		listValidators: map[reflect.Type]func([]client.ListOption) error{
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): func(opts []client.ListOption) error {
				var listOptions client.ListOptions
				for _, opt := range opts {
					opt.ApplyToList(&listOptions)
				}
				if listOptions.Namespace == "" {
					return fmt.Errorf("ReferenceGrant list must be namespaced")
				}
				switch listOptions.Namespace {
				case "shared", "certs", "listener-certs":
					seenGrantNamespaces[listOptions.Namespace]++
					return nil
				default:
					return fmt.Errorf("unexpected ReferenceGrant namespace %q", listOptions.Namespace)
				}
			},
		},
	}

	if err := reconciler.ReconcileGatewayObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "gw"},
	); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}

	if seenGrantNamespaces["shared"] == 0 {
		t.Fatalf("expected ReferenceGrant list for shared namespace")
	}
	if seenGrantNamespaces["certs"] == 0 {
		t.Fatalf("expected ReferenceGrant list for certs namespace")
	}
	if seenGrantNamespaces["listener-certs"] == 0 {
		t.Fatalf("expected ReferenceGrant list for listener-certs namespace")
	}
	if reader.gatewayClassGets != 1 {
		t.Fatalf("GatewayClass reader Get count = %d, want 1", reader.gatewayClassGets)
	}
}

func TestReferenceGrantTargetNamespacesForGatewayIncludesListenerSetTLSRefs(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	state := &clusterState{
		listenerSets: []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name:      "shared-cert",
							Namespace: namespacePtr("certs"),
						}},
					},
				}},
			},
		}},
	}

	got := referenceGrantTargetNamespacesForGateway(
		gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}},
		state,
	)
	want := []string{"certs"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("referenceGrantTargetNamespacesForGateway() = %#v, want %#v", got, want)
	}
}

func requireGatewayParentMatchingField(
	opts []client.ListOption,
	field string,
	namespace string,
	name string,
) error {
	value := gatewayParentStatusIndexValue(namespace, name)
	for _, opt := range opts {
		matching, ok := opt.(client.MatchingFields)
		if !ok {
			continue
		}
		if matching[field] == value {
			return nil
		}
	}
	return fmt.Errorf("route list must include matching field %s=%s", field, value)
}

func requireHTTPRouteGatewayOrListenerSetParentMatchingField(
	opts []client.ListOption,
	namespace string,
	name string,
) error {
	if err := requireGatewayParentMatchingField(opts, statusHTTPRouteGatewayParentIndex, namespace, name); err == nil {
		return nil
	}
	for _, opt := range opts {
		matching, ok := opt.(client.MatchingFields)
		if !ok {
			continue
		}
		if matching[statusHTTPRouteListenerSetParentIndex] == statusListenerSetParentIndexMarker {
			return nil
		}
	}
	return fmt.Errorf(
		"HTTPRoute list must include matching field %s=%s or %s=%s",
		statusHTTPRouteGatewayParentIndex,
		gatewayParentStatusIndexValue(namespace, name),
		statusHTTPRouteListenerSetParentIndex,
		statusListenerSetParentIndexMarker,
	)
}
