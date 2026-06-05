package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

func TestReconcileRefreshesResolvedRefsWhenBackendServiceAppears(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	pathType := gatewayv1.PathMatchPathPrefix
	portNumber := gatewayv1.PortNumber(8080)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
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
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathType,
								Value: ptr("/"),
							},
						}},
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
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
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(
		t,
		route.Status.Parents[0].Conditions,
		string(gatewayv1.RouteConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.RouteReasonBackendNotFound),
		1,
	)

	if err := k8sClient.Create(
		context.Background(),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 8080}},
			},
		},
	); err != nil {
		t.Fatalf("Create Service returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(
		t,
		route.Status.Parents[0].Conditions,
		string(gatewayv1.RouteConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.RouteReasonResolvedRefs),
		1,
	)
}

func TestReconcileSetsBackendTLSPolicyStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&gatewayv1alpha3.BackendTLSPolicy{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "orders",
									Port: portPtr(8443),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: ptr(gatewayv1.SectionName("https")),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
						SubjectAltNames: []gatewayv1.SubjectAltName{{
							Type:     gatewayv1.HostnameSubjectAltNameType,
							Hostname: "orders.backend.svc",
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}
	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor status, got %d", len(policy.Status.Ancestors))
	}
	ancestor := policy.Status.Ancestors[0]
	if ancestor.AncestorRef.Kind == nil || *ancestor.AncestorRef.Kind != gatewayv1.Kind("Gateway") {
		t.Fatalf("ancestor kind = %#v, want Gateway", ancestor.AncestorRef.Kind)
	}
	if ancestor.AncestorRef.Name != "gw" {
		t.Fatalf("ancestor name = %q, want gw", ancestor.AncestorRef.Name)
	}
	assertCondition(
		t,
		ancestor.Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.PolicyReasonAccepted),
		1,
	)
	assertCondition(
		t,
		ancestor.Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
}

func TestReconcileAcceptsBackendTLSPolicyWithCustomCAConfigMap(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&gatewayv1alpha3.BackendTLSPolicy{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-ca", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "orders",
									Port: portPtr(8443),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: ptr(gatewayv1.SectionName("https")),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: "orders.internal.example",
						CACertificateRefs: []gatewayv1.LocalObjectReference{{
							Name: "orders-ca",
						}},
						SubjectAltNames: []gatewayv1.SubjectAltName{{
							Type:     gatewayv1.HostnameSubjectAltNameType,
							Hostname: "orders.backend.svc",
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.PolicyReasonAccepted),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
}

func TestReconcileRejectsBackendTLSPolicyWithEmptyURISAN(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
						SubjectAltNames: []gatewayv1.SubjectAltName{{
							Type: gatewayv1.URISubjectAltNameType,
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonInvalid),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
}

func TestReconcileAcceptsBackendTLSPolicyWithURISubjectAltName(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
						SubjectAltNames: []gatewayv1.SubjectAltName{{
							Type: gatewayv1.URISubjectAltNameType,
							URI:  "spiffe://cluster.local/ns/default/sa/orders",
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.PolicyReasonAccepted),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
}

func TestReconcileAcceptsBackendTLSPolicyWithAtLeastOneValidCAConfigMap(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-ca-valid", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: "orders.internal.example",
						CACertificateRefs: []gatewayv1.LocalObjectReference{
							{Name: "orders-ca-missing"},
							{Name: "orders-ca-valid"},
						},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.PolicyReasonAccepted),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionFalse,
		backendTLSPolicyReasonInvalidCACertRef,
		1,
	)
}

func TestReconcileAcceptsBackendTLSPolicyWithAtLeastOneValidTargetRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "https", Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
								Group: "",
								Kind:  "Service",
								Name:  "orders",
							},
							SectionName: ptr(gatewayv1.SectionName("https")),
						},
						{
							LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
								Group: "",
								Kind:  "Service",
								Name:  "missing",
							},
						},
					},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.PolicyReasonAccepted),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonTargetNotFound),
		1,
	)
}

func TestReconcileRejectsBackendTLSPolicyWithInvalidTLSVersionOption(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
					},
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						"gateway.nantian.dev/backend-tls-min-version": "TLS1_2",
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonInvalid),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
}

func TestReconcileRejectsBackendTLSPolicyWithInvalidSubjectAltName(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
						SubjectAltNames: []gatewayv1.SubjectAltName{{
							Type:     gatewayv1.HostnameSubjectAltNameType,
							Hostname: "orders.default.svc",
							URI:      "spiffe://cluster.local/ns/default/sa/orders",
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonInvalid),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		1,
	)
	for _, condition := range policy.Status.Ancestors[0].Conditions {
		if condition.Type == string(gatewayv1.PolicyConditionAccepted) {
			if condition.Message == "" {
				t.Fatal("expected accepted condition to include an invalid subjectAltName message")
			}
			return
		}
	}
	t.Fatal("accepted condition not found")
}

func TestReconcileRejectsBackendTLSPolicyWithInvalidCustomCAConfigMap(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-ca", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": "not-a-certificate",
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: "orders.internal.example",
						CACertificateRefs: []gatewayv1.LocalObjectReference{{
							Name: "orders-ca",
						}},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		backendTLSPolicyReasonNoValidCACert,
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionFalse,
		backendTLSPolicyReasonInvalidCACertRef,
		1,
	)
}
