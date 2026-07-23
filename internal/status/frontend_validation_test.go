package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestReconcileRejectsInvalidFrontendValidationConfigMap(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "client-ca",
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
						},
					}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "client-ca", Namespace: "default"},
				Data:       map[string]string{},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		listenerReasonInvalidCACertificateRef,
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionFalse,
		listenerReasonNoValidCACertificate,
		1,
	)
}

func TestReconcileRejectsUnsupportedFrontendValidationCARefKind(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "client-ca",
										Kind: gatewayv1.Kind("Secret"),
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
						},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		listenerReasonInvalidCACertificateKind,
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionFalse,
		listenerReasonNoValidCACertificate,
		1,
	)
}

func TestReconcileKeepsAcceptedWhenFrontendValidationStillHasValidCA(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{
										{Name: "missing-ca"},
										{Name: "valid-ca"},
									},
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
						},
					}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "missing-ca", Namespace: "default"},
				Data:       map[string]string{},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-ca", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		listenerReasonInvalidCACertificateRef,
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonProgrammed),
		1,
	)
}

func TestReconcileRefreshesFrontendValidationRefsWhenReferenceGrantChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "security"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name:      "client-ca",
										Namespace: ptr(gatewayv1.Namespace("security")),
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
						},
					}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "client-ca", Namespace: "security"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonRefNotPermitted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionFalse,
		listenerReasonNoValidCACertificate,
		1,
	)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "security"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
				Kind:      gatewayv1beta1.Kind("Gateway"),
				Namespace: gatewayv1beta1.Namespace("default"),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("ConfigMap"),
				Name:  ptr[gatewayv1beta1.ObjectName]("client-ca"),
			}},
		},
	}
	if err := k8sClient.Create(context.Background(), grant); err != nil {
		t.Fatalf("Create ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonResolvedRefs),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonAccepted),
		1,
	)

	if err := k8sClient.Delete(context.Background(), grant); err != nil {
		t.Fatalf("Delete ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonRefNotPermitted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionFalse,
		listenerReasonNoValidCACertificate,
		1,
	)
}

func TestReconcileIgnoresFrontendValidationForTLSPassthroughListener(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	passthrough := gatewayv1.TLSModePassthrough

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "client-ca",
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
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "client-ca", Namespace: "default"},
				Data:       map[string]string{},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonResolvedRefs),
		1,
	)
}

func TestReconcileSetsInsecureFrontendValidationModeCondition(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "client-ca",
									}},
									Mode: gatewayv1.AllowInsecureFallback,
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
						},
					}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "client-ca", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionInsecureFrontendValidationMode),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonConfigurationChanged),
		1,
	)
}
