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

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildGatewayListenersForSnapshotIncludesListenerSets(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	listenerHostname := gatewayv1.Hostname("listener-set.example.com")

	cl := testutil.NewTranslatorClientBuilder(scheme).
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
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &listenerHostname,
					}},
				},
			},
		).
		Build()

	next, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).BuildGatewayListenersForSnapshot(
		context.Background(),
		cl,
		&ir.Snapshot{},
		[]client.ObjectKey{{Namespace: "default", Name: "gw"}},
	)
	if err != nil {
		t.Fatalf("BuildGatewayListenersForSnapshot returned error: %v", err)
	}

	found := false
	for _, listener := range next.Listeners {
		if listener.Name == "default/gw/default/ls/ls-listener" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ListenerSet listener in rebuilt gateway listeners, got %#v", next.Listeners)
	}
}

func TestBuildGatewayListenersForSnapshotAddsSecondListenerSetToExistingSnapshot(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	ls1Hostname := gatewayv1.Hostname("listener-set-1.example.com")
	ls2Hostname := gatewayv1.Hostname("listener-set-2.example.com")

	cl := testutil.NewTranslatorClientBuilder(scheme).
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
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "listener-1",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &ls1Hostname,
					}},
				},
			},
		).
		Build()

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if err := cl.Create(context.Background(), &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-2", Namespace: "default"},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     "listener-2",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: &ls2Hostname,
			}},
		},
	}); err != nil {
		t.Fatalf("create second listenerset: %v", err)
	}

	next, err := translator.BuildGatewayListenersForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "gw"}},
	)
	if err != nil {
		t.Fatalf("BuildGatewayListenersForSnapshot returned error: %v", err)
	}

	found := make(map[string]bool, len(next.Listeners))
	for _, listener := range next.Listeners {
		found[listener.Name] = true
	}
	if !found["default/gw/default/ls-1/listener-1"] {
		t.Fatalf("expected first ListenerSet listener to remain, got %#v", next.Listeners)
	}
	if !found["default/gw/default/ls-2/listener-2"] {
		t.Fatalf("expected second ListenerSet listener to be added, got %#v", next.Listeners)
	}
}

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
			"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
			"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
		},
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
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
	if got := findSnapshotSecret(t, next, "default", "shared-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected preserved cert material: %q", got)
	}
}
