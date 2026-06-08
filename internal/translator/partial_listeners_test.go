package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildGatewayListenersForSnapshotPreservesSharedSecretUsedByUntouchedGateway(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	gatewayClass := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
	stableGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "stable-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Name: "shared-cert",
					}},
				},
			}},
		},
	}
	removedGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "removed-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Name: "shared-cert",
					}},
				},
			}},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-cert", Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": readTestTLSAsset(t, "client.crt"),
			"tls.key": readTestTLSAsset(t, "client.key"),
		},
	}

	cl := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			gatewayClass,
			stableGateway,
			removedGateway,
			secret,
		).
		Build()

	current := buildRotationSnapshot(t, cl, string(controllerName))
	if len(current.Secrets) != 1 {
		t.Fatalf("expected a single shared secret in current snapshot, got %#v", current.Secrets)
	}

	if err := cl.Delete(context.Background(), removedGateway); err != nil {
		t.Fatalf("delete removed gateway: %v", err)
	}

	next, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).BuildGatewayListenersForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "removed-gw"}},
	)
	if err != nil {
		t.Fatalf("BuildGatewayListenersForSnapshot returned error: %v", err)
	}

	if len(next.Listeners) != 1 {
		t.Fatalf("listener count = %d, want 1", len(next.Listeners))
	}
	if next.Listeners[0].Name != "default/stable-gw/https" {
		t.Fatalf("unexpected remaining listeners: %#v", next.Listeners)
	}
	if next.Listeners[0].TLS == nil {
		t.Fatalf("expected remaining listener to keep TLS config")
	}
	if got := next.Listeners[0].TLS.SecretRefs; len(got) != 1 || got[0] != "default/shared-cert" {
		t.Fatalf("unexpected remaining listener secret refs: %#v", got)
	}
	if len(next.Secrets) != 1 {
		t.Fatalf("expected shared secret to remain in snapshot, got %#v", next.Secrets)
	}
	if got := findSnapshotSecret(t, next, "default", "shared-cert").CertPEM; got != string(readTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected preserved cert material: %q", got)
	}
}
