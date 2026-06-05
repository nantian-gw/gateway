package status

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func TestLoadStateScopesRoutesToManagedGatewaysAndServiceParents(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, statusGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteServiceParentIndex, statusHTTPRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteServiceParentIndex, statusGRPCRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteServiceParentIndex, statusTCPRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteServiceParentIndex, statusUDPRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteServiceParentIndex, statusTLSRouteServiceParentIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "consumer"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "foreign",
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "managed-gateway-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign-gateway-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "foreign"}},
					},
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
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		k8sClient,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = validatingListReader{
		Reader: k8sClient,
		listValidators: map[reflect.Type]func(client.ListOptions) error{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireGatewayClassControllerList(string(controllerName)),
			reflect.TypeOf(&gatewayv1.GatewayList{}):      requireGatewayClassNameList("nantian-gw"),
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}): func(opts client.ListOptions) error {
				if opts.FieldSelector == nil || opts.FieldSelector.Empty() {
					return fmt.Errorf("HTTPRoute list must stay scoped")
				}
				if opts.FieldSelector.Matches(fields.Set{
					statusHTTPRouteServiceParentIndex: statusServiceParentIndexMarker,
				}) {
					return nil
				}
				if opts.FieldSelector.Matches(fields.Set{
					statusHTTPRouteGatewayParentIndex: gatewayParentStatusIndexValue("default", "gw"),
				}) {
					return nil
				}
				return fmt.Errorf("unexpected HTTPRoute field selector %q", opts.FieldSelector.String())
			},
		},
	}

	state, err := reconciler.loadState(context.Background())
	if err != nil {
		t.Fatalf("loadState returned error: %v", err)
	}

	got := make([]string, 0, len(state.httpRoutes))
	for _, route := range state.httpRoutes {
		got = append(got, route.Namespace+"/"+route.Name)
	}
	want := []string{
		"consumer/service-parent-route",
		"default/managed-gateway-route",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("http route keys = %#v, want %#v", got, want)
	}
}

func TestLoadStateLoadsBackendPoliciesForReferencedBackends(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	caBundle := gatewayv1.WellKnownCACertificatesSystem

	echoTLSPolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "echo",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "echo.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	echoTLSRaw, err := gatewayapi.EncodeBackendTLSPolicyV1(echoTLSPolicy)
	if err != nil {
		t.Fatalf("encode BackendTLSPolicy: %v", err)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, statusGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteServiceParentIndex, statusHTTPRouteServiceParentIndexKeys).
		WithIndex(gatewayapi.NewBackendTLSPolicyV1Object(), statusBackendTLSPolicyTargetRefIndex, statusBackendTLSPolicyTargetRefIndexKeys).
		WithIndex(&backendlbv1alpha2.BackendLBPolicy{}, statusBackendLBPolicyTargetRefIndex, statusBackendLBPolicyTargetRefIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "service-parent-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group("")),
							Kind:      ptr(gatewayv1.Kind("Service")),
							Name:      "frontend",
							Namespace: namespacePtr("default"),
						}},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			echoTLSRaw,
			&backendlbv1alpha2.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "default"},
				Spec: backendlbv1alpha2.BackendLBPolicySpec{
					TargetRefs: []backendlbv1alpha2.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "echo",
					}},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		k8sClient,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)

	state, err := reconciler.loadState(context.Background())
	if err != nil {
		t.Fatalf("loadState returned error: %v", err)
	}
	if len(state.backendLBPolicies) != 1 || state.backendLBPolicies[0].Name != "echo-lb" {
		t.Fatalf("unexpected BackendLBPolicies: %#v", state.backendLBPolicies)
	}
	if len(state.backendTLSPolicies) != 1 || state.backendTLSPolicies[0].Name != "echo-tls" {
		t.Fatalf("unexpected BackendTLSPolicies: %#v", state.backendTLSPolicies)
	}
}

func TestLoadStateFallsBackToBackendPolicyListsWithoutRouteBackendRefs(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	caBundle := gatewayv1.WellKnownCACertificatesSystem

	policyRaw, err := gatewayapi.EncodeBackendTLSPolicyV1(&gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Kind: "Service",
					Name: "orders",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "orders.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	})
	if err != nil {
		t.Fatalf("encode BackendTLSPolicy: %v", err)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(gatewayapi.NewBackendTLSPolicyV1Object(), statusBackendTLSPolicyTargetRefIndex, statusBackendTLSPolicyTargetRefIndexKeys).
		WithIndex(&backendlbv1alpha2.BackendLBPolicy{}, statusBackendLBPolicyTargetRefIndex, statusBackendLBPolicyTargetRefIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			policyRaw,
			&backendlbv1alpha2.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-lb", Namespace: "default"},
				Spec: backendlbv1alpha2.BackendLBPolicySpec{
					TargetRefs: []backendlbv1alpha2.LocalPolicyTargetReference{{
						Kind: "Service",
						Name: "orders",
					}},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		k8sClient,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = validatingListReader{
		Reader: k8sClient,
		listValidators: map[reflect.Type]func(client.ListOptions) error{
			reflect.TypeOf(&backendlbv1alpha2.BackendLBPolicyList{}): func(opts client.ListOptions) error {
				if opts.Namespace != "" {
					return fmt.Errorf("BackendLBPolicy fallback list namespace = %q, want cluster-wide", opts.Namespace)
				}
				if opts.FieldSelector != nil && !opts.FieldSelector.Empty() {
					return fmt.Errorf("BackendLBPolicy fallback selector = %q, want none", opts.FieldSelector.String())
				}
				return nil
			},
			reflect.TypeOf(&unstructured.UnstructuredList{}): func(opts client.ListOptions) error {
				if opts.Namespace != "" {
					return fmt.Errorf("BackendTLSPolicy fallback list namespace = %q, want cluster-wide", opts.Namespace)
				}
				if opts.FieldSelector != nil && !opts.FieldSelector.Empty() {
					return fmt.Errorf("BackendTLSPolicy fallback selector = %q, want none", opts.FieldSelector.String())
				}
				return nil
			},
		},
	}

	state, err := reconciler.loadState(context.Background())
	if err != nil {
		t.Fatalf("loadState returned error: %v", err)
	}
	if len(state.backendLBPolicies) != 1 || state.backendLBPolicies[0].Name != "orders-lb" {
		t.Fatalf("unexpected BackendLBPolicies: %#v", state.backendLBPolicies)
	}
	if len(state.backendTLSPolicies) != 1 || state.backendTLSPolicies[0].Name != "orders-tls" {
		t.Fatalf("unexpected BackendTLSPolicies: %#v", state.backendTLSPolicies)
	}
}
