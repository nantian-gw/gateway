package translator

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
)

func TestBuildSnapshotIncludesBackendClientCertificateRef(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	client := newTranslatorClientBuilder(scheme).
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
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name: "client-cert",
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-cert",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readTestTLSAsset(t, "client.crt"),
					"tls.key": readTestTLSAsset(t, "client.key"),
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}

	backendTLS := snapshot.Listeners[0].BackendTLS
	if backendTLS == nil {
		t.Fatal("expected backend tls config in translated listener")
	}
	if backendTLS.ClientCertificateRef != "default/client-cert" {
		t.Fatalf("unexpected client certificate ref: %q", backendTLS.ClientCertificateRef)
	}
}

func TestBuildSnapshotIncludesCrossNamespaceBackendClientCertificateRefWithReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
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
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name:      "client-cert",
								Namespace: ptr(gatewayv1.Namespace("shared")),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readTestTLSAsset(t, "client.crt"),
					"tls.key": readTestTLSAsset(t, "client.key"),
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
						Name:  ptr[gatewayv1beta1.ObjectName]("client-cert"),
					}},
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	backendTLS := snapshot.Listeners[0].BackendTLS
	if backendTLS == nil {
		t.Fatal("expected backend tls config in translated listener")
	}
	if backendTLS.ClientCertificateRef != "shared/client-cert" {
		t.Fatalf("unexpected cross-namespace client certificate ref: %q", backendTLS.ClientCertificateRef)
	}
}

func TestBuildSnapshotSkipsCrossNamespaceBackendClientCertificateRefWithoutReferenceGrant(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
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
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name:      "client-cert",
								Namespace: ptr(gatewayv1.Namespace("shared")),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readTestTLSAsset(t, "client.crt"),
					"tls.key": readTestTLSAsset(t, "client.key"),
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	backendTLS := snapshot.Listeners[0].BackendTLS
	if backendTLS != nil {
		t.Fatalf("expected cross-namespace backend client certificate ref without ReferenceGrant to be skipped, got %#v", backendTLS)
	}
	if len(snapshot.Secrets) != 0 {
		t.Fatalf("expected unauthorized backend client certificate secret to be omitted from snapshot, got %#v", snapshot.Secrets)
	}
}

func TestBuildSnapshotSkipsBackendClientCertificateRefWithUnsupportedKind(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	client := newTranslatorClientBuilder(scheme).
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
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name: "client-cert",
								Kind: ptr(gatewayv1.Kind("ConfigMap")),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	backendTLS := snapshot.Listeners[0].BackendTLS
	if backendTLS != nil {
		t.Fatalf("expected unsupported backend client certificate ref kind to be skipped, got %#v", backendTLS)
	}
	if len(snapshot.Secrets) != 0 {
		t.Fatalf("expected unsupported backend client certificate ref kind to omit snapshot secrets, got %#v", snapshot.Secrets)
	}
}

func readTestTLSAsset(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "tests", "testdata", "tls", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
