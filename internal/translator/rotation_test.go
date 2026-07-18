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
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildSnapshotRefreshesGatewayListenerSecretMaterialAfterRotation(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	gatewayClass := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Name: "example-cert",
					}},
				},
			}},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "example-cert", Namespace: "default"},
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
			gateway,
			secret,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	if got := findSnapshotSecret(t, first, "default", "example-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected initial cert material: %q", got)
	}

	secret.Data["tls.crt"] = readBackendTLSAsset(t, "server-san.crt")
	secret.Data["tls.key"] = readBackendTLSAsset(t, "server-san.key")
	if err := cl.Update(context.Background(), secret); err != nil {
		t.Fatalf("update secret: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	if got := findSnapshotSecret(t, second, "default", "example-cert").CertPEM; got != string(readBackendTLSAsset(t, "server-san.crt")) {
		t.Fatalf("expected rotated cert material, got %q", got)
	}
}

func TestBuildSnapshotRefreshesSecondaryGatewayListenerSecretMaterialAfterRotation(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	gatewayClass := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{
						{Name: "default-cert"},
						{Name: "secondary-cert"},
					},
				},
			}},
		},
	}
	defaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "default-cert", Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": readBackendTLSAsset(t, "server-san.crt"),
			"tls.key": readBackendTLSAsset(t, "server-san.key"),
		},
	}
	secondarySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secondary-cert", Namespace: "default"},
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
			gateway,
			defaultSecret,
			secondarySecret,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	if got := findSnapshotSecret(t, first, "default", "secondary-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected initial secondary cert material: %q", got)
	}

	secondarySecret.Data["tls.crt"] = readBackendTLSAsset(t, "server-san.crt")
	secondarySecret.Data["tls.key"] = readBackendTLSAsset(t, "server-san.key")
	if err := cl.Update(context.Background(), secondarySecret); err != nil {
		t.Fatalf("update secondary secret: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	if got := findSnapshotSecret(t, second, "default", "secondary-cert").CertPEM; got != string(readBackendTLSAsset(t, "server-san.crt")) {
		t.Fatalf("expected rotated secondary cert material, got %q", got)
	}
}

func TestBuildSnapshotFallsBackToSecondaryCertificateWhenPrimaryBecomesInvalid(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	gatewayClass := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{
						{Name: "default-cert"},
						{Name: "secondary-cert"},
					},
				},
			}},
		},
	}
	defaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "default-cert", Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": readBackendTLSAsset(t, "server-san.crt"),
			"tls.key": readBackendTLSAsset(t, "server-san.key"),
		},
	}
	secondarySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secondary-cert", Namespace: "default"},
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
			gateway,
			defaultSecret,
			secondarySecret,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	if got := first.Listeners[0].TLS.SecretRefs; len(got) != 2 || got[0] != "default/default-cert" || got[1] != "default/secondary-cert" {
		t.Fatalf("unexpected initial secret refs: %#v", got)
	}

	defaultSecret.Data["tls.crt"] = []byte("not-a-cert")
	defaultSecret.Data["tls.key"] = []byte("not-a-key")
	if err := cl.Update(context.Background(), defaultSecret); err != nil {
		t.Fatalf("update default secret: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	if got := second.Listeners[0].TLS.SecretRefs; len(got) != 1 || got[0] != "default/secondary-cert" {
		t.Fatalf("expected fallback to surviving secondary cert, got %#v", got)
	}
	if len(second.Secrets) != 1 {
		t.Fatalf("expected only secondary secret to remain in snapshot, got %#v", second.Secrets)
	}
	if got := findSnapshotSecret(t, second, "default", "secondary-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected surviving secondary cert material: %q", got)
	}
}

func TestBuildSnapshotRefreshesFrontendValidationBundleAfterConfigMapRotation(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	clientCA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway-client-ca", Namespace: "default"},
		Data: map[string]string{
			"ca.crt": "PEM-OLD",
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			TLS: &gatewayv1.GatewayTLSConfig{
				Frontend: &gatewayv1.FrontendTLSConfig{
					Default: gatewayv1.TLSConfig{
						Validation: &gatewayv1.FrontendTLSValidation{
							CACertificateRefs: []gatewayv1.ObjectReference{{
								Name: "gateway-client-ca",
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
					Mode: ptr(gatewayv1.TLSModeTerminate),
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Name: "example-cert",
					}},
				},
			}},
		},
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gateway,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "example-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			clientCA,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	if got := first.Listeners[0].TLS.FrontendValidation.ClientCAPEMs[0]; got != "PEM-OLD" {
		t.Fatalf("unexpected initial client ca: %q", got)
	}

	clientCA.Data["ca.crt"] = "PEM-NEW"
	if err := cl.Update(context.Background(), clientCA); err != nil {
		t.Fatalf("update configmap: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	if got := second.Listeners[0].TLS.FrontendValidation.ClientCAPEMs[0]; got != "PEM-NEW" {
		t.Fatalf("expected rotated client ca, got %q", got)
	}
}

func TestBuildSnapshotRefreshesBackendClientCertificateSecretMaterialAfterRotation(t *testing.T) {
	scheme := rotationTestScheme(t, false)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	clientCert := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "client-cert", Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
			"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
		},
	}
	gateway := &gatewayv1.Gateway{
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
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gateway,
			clientCert,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	if got := first.Listeners[0].BackendTLS.ClientCertificateRef; got != "default/client-cert" {
		t.Fatalf("unexpected backend client cert ref: %q", got)
	}
	if got := findSnapshotSecret(t, first, "default", "client-cert").CertPEM; got != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected initial backend client cert material: %q", got)
	}

	clientCert.Data["tls.crt"] = readBackendTLSAsset(t, "server-san.crt")
	clientCert.Data["tls.key"] = readBackendTLSAsset(t, "server-san.key")
	if err := cl.Update(context.Background(), clientCert); err != nil {
		t.Fatalf("update backend client cert secret: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	if got := second.Listeners[0].BackendTLS.ClientCertificateRef; got != "default/client-cert" {
		t.Fatalf("expected backend client cert ref to stay stable, got %q", got)
	}
	if got := findSnapshotSecret(t, second, "default", "client-cert").CertPEM; got != string(readBackendTLSAsset(t, "server-san.crt")) {
		t.Fatalf("expected rotated backend client cert material, got %q", got)
	}
}

func TestBuildSnapshotRefreshesBackendTLSPolicyValidationAfterRotation(t *testing.T) {
	scheme := rotationTestScheme(t, true)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	ca := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-ca", Namespace: "default"},
		Data: map[string]string{
			"ca.crt": string(testutil.ReadTestTLSAsset(t, "client.crt")),
		},
	}
	policy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "orders",
				},
				SectionName: testutil.SectionNamePtr("https"),
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname: "orders.old.example",
				CACertificateRefs: []gatewayv1.LocalObjectReference{{
					Name: "orders-ca",
				}},
				SubjectAltNames: []gatewayv1.SubjectAltName{{
					Type:     gatewayv1.HostnameSubjectAltNameType,
					Hostname: "orders.old.svc",
				}},
			},
		},
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
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
					Ports: []corev1.ServicePort{{Name: "https", Port: 8443}},
				},
			},
			ca,
			policy,
		).
		Build()

	first := buildRotationSnapshot(t, cl, string(controllerName))
	firstValidation := first.Backends[0].BackendTLSValidation
	if firstValidation == nil {
		t.Fatal("expected initial backend tls validation")
	}
	if firstValidation.Hostname != "orders.old.example" {
		t.Fatalf("unexpected initial hostname: %q", firstValidation.Hostname)
	}
	if firstValidation.CAPEMs[0] != string(testutil.ReadTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected initial CA bundle: %q", firstValidation.CAPEMs[0])
	}
	if firstValidation.SubjectAltNames[0].Value != "orders.old.svc" {
		t.Fatalf("unexpected initial SAN: %#v", firstValidation.SubjectAltNames)
	}

	ca.Data["ca.crt"] = string(readBackendTLSAsset(t, "server-san.crt"))
	if err := cl.Update(context.Background(), ca); err != nil {
		t.Fatalf("update backend tls ca: %v", err)
	}
	policy.Spec.Validation.Hostname = "orders.new.example"
	policy.Spec.Validation.SubjectAltNames = []gatewayv1.SubjectAltName{{
		Type:     gatewayv1.HostnameSubjectAltNameType,
		Hostname: "orders.new.svc",
	}}
	if err := cl.Update(context.Background(), policy); err != nil {
		t.Fatalf("update backend tls policy: %v", err)
	}

	second := buildRotationSnapshot(t, cl, string(controllerName))
	secondValidation := second.Backends[0].BackendTLSValidation
	if secondValidation == nil {
		t.Fatal("expected rotated backend tls validation")
	}
	if secondValidation.Hostname != "orders.new.example" {
		t.Fatalf("expected rotated hostname, got %q", secondValidation.Hostname)
	}
	if secondValidation.CAPEMs[0] != string(readBackendTLSAsset(t, "server-san.crt")) {
		t.Fatalf("expected rotated CA bundle, got %q", secondValidation.CAPEMs[0])
	}
	if len(secondValidation.SubjectAltNames) != 1 || secondValidation.SubjectAltNames[0].Value != "orders.new.svc" {
		t.Fatalf("unexpected rotated SANs: %#v", secondValidation.SubjectAltNames)
	}
}

func buildRotationSnapshot(t *testing.T, cl client.Client, controllerName string) *ir.Snapshot {
	t.Helper()

	snapshot, err := New(
		controllerName,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return snapshot
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

func rotationTestScheme(t *testing.T, includeBackendTLSPolicy bool) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	if includeBackendTLSPolicy {
		must(gatewayv1alpha3.Install(scheme), t)
	}
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)
	return scheme
}

func readBackendTLSAsset(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "test", "testdata", "backendtls", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
