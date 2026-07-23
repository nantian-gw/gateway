package listeners_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildSnapshotIncludesFrontendValidationCAPEMs(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "default",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil || tls.FrontendValidation == nil {
		t.Fatal("expected frontend validation in translated listener")
	}
	if len(tls.FrontendValidation.ClientCAPEMs) != 1 {
		t.Fatalf("expected 1 client ca pem, got %d", len(tls.FrontendValidation.ClientCAPEMs))
	}
	if tls.FrontendValidation.ClientCAPEMs[0] != "PEM-DATA" {
		t.Fatalf("unexpected client ca pem: %q", tls.FrontendValidation.ClientCAPEMs[0])
	}
}

func TestBuildSnapshotIncludesFrontendValidationMode(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "default",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil || tls.FrontendValidation == nil {
		t.Fatal("expected frontend validation in translated listener")
	}
	if tls.FrontendValidation.Mode != string(gatewayv1.AllowInsecureFallback) {
		t.Fatalf("expected frontend validation mode %q, got %q", gatewayv1.AllowInsecureFallback, tls.FrontendValidation.Mode)
	}
}

func TestBuildSnapshotIncludesCrossNamespaceFrontendValidationWithReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "security",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
			&gatewayv1beta1.ReferenceGrant{
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
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil || tls.FrontendValidation == nil {
		t.Fatal("expected frontend validation in translated listener")
	}
	if len(tls.FrontendValidation.ClientCAPEMs) != 1 {
		t.Fatalf("expected 1 client ca pem, got %d", len(tls.FrontendValidation.ClientCAPEMs))
	}
	if tls.FrontendValidation.ClientCAPEMs[0] != "PEM-DATA" {
		t.Fatalf("unexpected client ca pem: %q", tls.FrontendValidation.ClientCAPEMs[0])
	}
}

func TestBuildSnapshotRejectsHTTPSListenerForCrossNamespaceFrontendValidationWithoutReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "security",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected HTTPS listener to remain with rejecting frontend validation, got %#v", snapshot.Listeners)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil || tls.FrontendValidation == nil {
		t.Fatalf("expected rejecting frontend validation, got %#v", snapshot.Listeners[0])
	}
	if tls.FrontendValidation.Mode != "RejectClientCertificate" {
		t.Fatalf("expected internal rejection mode, got %q", tls.FrontendValidation.Mode)
	}
	if len(tls.FrontendValidation.ClientCAPEMs) != 0 {
		t.Fatalf("expected rejection mode to avoid shared-bind CA material, got %d CA PEMs", len(tls.FrontendValidation.ClientCAPEMs))
	}
}

func TestBuildSnapshotIgnoresFrontendValidationForTLSPassthroughListener(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	passthrough := gatewayv1.TLSModePassthrough

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "default",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil {
		t.Fatal("expected translated listener tls config")
	}
	if !tls.Passthrough {
		t.Fatalf("expected passthrough listener, got %#v", tls)
	}
	if tls.FrontendValidation != nil {
		t.Fatalf("expected TLS passthrough listener to ignore frontend validation, got %#v", tls.FrontendValidation)
	}
}

func TestBuildSnapshotRejectsHTTPSListenerWhenDefaultFrontendValidationHasNoValidCA(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "missing-ca",
									}},
								},
							},
							PerPort: []gatewayv1.TLSPortConfig{{
								Port: 80,
								TLS: gatewayv1.TLSConfig{
									Validation: &gatewayv1.FrontendTLSValidation{
										CACertificateRefs: []gatewayv1.ObjectReference{{
											Name: "client-ca",
										}},
									},
								},
							}},
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &mode,
							},
						},
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
					},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "client-ca", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 2 {
		t.Fatalf("expected HTTP listener and rejecting HTTPS listener, got %#v", snapshot.Listeners)
	}

	var sawHTTP, sawRejectingHTTPS bool
	for _, listener := range snapshot.Listeners {
		if listener.Protocol == string(gatewayv1.HTTPProtocolType) && listener.Port == 80 {
			sawHTTP = true
			continue
		}
		if listener.Protocol == string(gatewayv1.HTTPSProtocolType) && listener.Port == 443 {
			if listener.TLS == nil || listener.TLS.FrontendValidation == nil {
				t.Fatalf("expected rejecting frontend validation on HTTPS listener, got %#v", listener)
			}
			if listener.TLS.FrontendValidation.Mode != "RejectClientCertificate" {
				t.Fatalf("expected internal rejection mode, got %q", listener.TLS.FrontendValidation.Mode)
			}
			if len(listener.TLS.FrontendValidation.ClientCAPEMs) != 0 {
				t.Fatalf("expected rejection mode to avoid shared-bind CA material, got %d CA PEMs", len(listener.TLS.FrontendValidation.ClientCAPEMs))
			}
			sawRejectingHTTPS = true
		}
	}
	if !sawHTTP || !sawRejectingHTTPS {
		t.Fatalf("expected HTTP listener and rejecting HTTPS listener, got %#v", snapshot.Listeners)
	}
}

func TestBuildSnapshotKeepsRejectedFrontendValidationListenerFromCurrentStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gw",
					Namespace:  "default",
					Generation: 7,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "missing-ca",
									}},
								},
							},
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &mode,
							},
						},
					},
				},
				Status: gatewayv1.GatewayStatus{
					Listeners: []gatewayv1.ListenerStatus{{
						Name: "https",
						Conditions: []metav1.Condition{
							{
								Type:               string(gatewayv1.ListenerConditionResolvedRefs),
								Status:             metav1.ConditionFalse,
								Reason:             string(gatewayv1.ListenerReasonInvalidCACertificateRef),
								ObservedGeneration: 7,
							},
							{
								Type:               string(gatewayv1.ListenerConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(gatewayv1.ListenerReasonNoValidCACertificate),
								ObservedGeneration: 7,
							},
							{
								Type:               string(gatewayv1.ListenerConditionProgrammed),
								Status:             metav1.ConditionFalse,
								Reason:             string(gatewayv1.ListenerReasonInvalid),
								ObservedGeneration: 7,
							},
						},
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected rejected HTTPS listener to remain in snapshot, got %#v", snapshot.Listeners)
	}
	tls := snapshot.Listeners[0].TLS
	if tls == nil || tls.FrontendValidation == nil {
		t.Fatalf("expected rejecting frontend validation, got %#v", snapshot.Listeners[0])
	}
	if tls.FrontendValidation.Mode != "RejectClientCertificate" {
		t.Fatalf("expected internal rejection mode, got %q", tls.FrontendValidation.Mode)
	}
	if len(tls.FrontendValidation.ClientCAPEMs) != 0 {
		t.Fatalf("expected rejection mode to avoid shared-bind CA material, got %d CA PEMs", len(tls.FrontendValidation.ClientCAPEMs))
	}
}

func TestBuildSnapshotSkipsCrossNamespaceCertificateRefWithoutReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name:      "shared-cert",
								Namespace: ptr(gatewayv1.Namespace("shared")),
							}},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil {
		t.Fatal("expected translated listener tls config")
	}
	if len(tls.SecretRefs) != 0 {
		t.Fatalf("expected cross-namespace certificate ref without ReferenceGrant to be skipped, got %#v", tls.SecretRefs)
	}
	if len(snapshot.Secrets) != 0 {
		t.Fatalf("expected unauthorized certificate secret to be omitted from snapshot, got %#v", snapshot.Secrets)
	}
}

func TestBuildSnapshotIncludesCrossNamespaceCertificateRefWithReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name:      "shared-cert",
								Namespace: ptr(gatewayv1.Namespace("shared")),
							}},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr[gatewayv1beta1.ObjectName]("shared-cert"),
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil {
		t.Fatal("expected translated listener tls config")
	}
	if len(tls.SecretRefs) != 1 || tls.SecretRefs[0] != "shared/shared-cert" {
		t.Fatalf("expected granted cross-namespace certificate ref, got %#v", tls.SecretRefs)
	}
	if got := findSnapshotSecret(t, snapshot, "shared", "shared-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected translated certificate secret material: %q", got)
	}
}

func TestBuildSnapshotDeduplicatesValidListenerCertificateRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{
								{Name: "example-cert"},
								{Name: "example-cert"},
								{Name: "shared-cert", Namespace: ptr(gatewayv1.Namespace("shared"))},
								{Name: "shared-cert", Namespace: ptr(gatewayv1.Namespace("shared"))},
							},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-cert",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readBackendTLSAsset(t, "server-san.crt"),
					"tls.key": readBackendTLSAsset(t, "server-san.key"),
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr[gatewayv1beta1.ObjectName]("shared-cert"),
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil {
		t.Fatal("expected translated listener tls config")
	}
	if len(tls.SecretRefs) != 2 {
		t.Fatalf("expected 2 unique secret refs, got %#v", tls.SecretRefs)
	}
	if tls.SecretRefs[0] != "default/example-cert" || tls.SecretRefs[1] != "shared/shared-cert" {
		t.Fatalf("unexpected deduplicated secret refs: %#v", tls.SecretRefs)
	}
}

func TestBuildSnapshotPreservesValidCertificateRefOrderAcrossMixedValidityRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{
								{Name: "blocked-cert", Namespace: ptr(gatewayv1.Namespace("blocked"))},
								{Name: "malformed-cert"},
								{Name: "default-cert"},
								{Name: "shared-cert", Namespace: ptr(gatewayv1.Namespace("shared"))},
								{Name: "default-cert"},
								{Name: "shared-cert", Namespace: ptr(gatewayv1.Namespace("shared"))},
							},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blocked-cert",
					Namespace: "blocked",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readBackendTLSAsset(t, "server-san.crt"),
					"tls.key": readBackendTLSAsset(t, "server-san.key"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "malformed-cert",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("not-a-cert"),
					"tls.key": []byte("not-a-key"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-cert",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readBackendTLSAsset(t, "server-san.crt"),
					"tls.key": readBackendTLSAsset(t, "server-san.key"),
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr[gatewayv1beta1.ObjectName]("shared-cert"),
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	tls := snapshot.Listeners[0].TLS
	if tls == nil {
		t.Fatal("expected translated listener tls config")
	}
	if len(tls.SecretRefs) != 2 {
		t.Fatalf("expected 2 surviving secret refs, got %#v", tls.SecretRefs)
	}
	if tls.SecretRefs[0] != "default/default-cert" || tls.SecretRefs[1] != "shared/shared-cert" {
		t.Fatalf("unexpected filtered secret ref order: %#v", tls.SecretRefs)
	}
	if len(snapshot.Secrets) != 2 {
		t.Fatalf("expected 2 translated secrets, got %#v", snapshot.Secrets)
	}
	if got := findSnapshotSecret(t, snapshot, "default", "default-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected default cert material: %q", got)
	}
	if got := findSnapshotSecret(t, snapshot, "shared", "shared-cert").CertPEM; got != string(readBackendTLSAsset(t, "server-san.crt")) {
		t.Fatalf("unexpected shared cert material: %q", got)
	}
}

// --- test helpers (local copies originally from translator package) ---

func ptr[T any](value T) *T {
	return &value
}

func findSnapshotSecret(t *testing.T, snapshot *ir.Snapshot, namespace, name string) ir.SecretMaterial {
	t.Helper()
	for _, secret := range snapshot.Secrets {
		if secret.Namespace == namespace && secret.Name == name {
			return secret
		}
	}
	t.Fatalf("secret %s/%s not found in snapshot", namespace, name)
	return ir.SecretMaterial{}
}

func readBackendTLSAsset(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "test", "testdata", "backendtls", name)
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func must(err error, t *testing.T) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
