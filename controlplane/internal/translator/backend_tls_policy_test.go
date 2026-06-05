package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func TestBuildSnapshotIncludesBackendTLSPolicyValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
						SubjectAltNames: []gatewayv1.SubjectAltName{
							{
								Type:     gatewayv1.HostnameSubjectAltNameType,
								Hostname: "orders.backend.svc",
							},
							{
								Type: gatewayv1.URISubjectAltNameType,
								URI:  "spiffe://cluster.local/ns/default/sa/orders",
							},
						},
					},
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}

	validation := snapshot.Backends[0].BackendTLSValidation
	if validation == nil {
		t.Fatal("expected backend tls validation")
	}
	if validation.Hostname != "orders.internal.example" {
		t.Fatalf("unexpected validation hostname: %q", validation.Hostname)
	}
	if !validation.UseSystemCAs {
		t.Fatal("expected system CA validation to be enabled")
	}
	if len(validation.CAPEMs) != 0 {
		t.Fatalf("expected no custom CA PEMs, got %d", len(validation.CAPEMs))
	}
	if len(validation.SubjectAltNames) != 2 {
		t.Fatalf("expected 2 SANs, got %d", len(validation.SubjectAltNames))
	}
	if validation.SubjectAltNames[0].Type != "Hostname" {
		t.Fatalf("unexpected SAN type: %q", validation.SubjectAltNames[0].Type)
	}
	if validation.SubjectAltNames[0].Value != "orders.backend.svc" {
		t.Fatalf("unexpected hostname SAN value: %q", validation.SubjectAltNames[0].Value)
	}
	if validation.SubjectAltNames[1].Type != "URI" {
		t.Fatalf("unexpected SAN type: %q", validation.SubjectAltNames[1].Type)
	}
	if validation.SubjectAltNames[1].Value != "spiffe://cluster.local/ns/default/sa/orders" {
		t.Fatalf("unexpected URI SAN value: %q", validation.SubjectAltNames[1].Value)
	}
}

func TestBuildSnapshotIncludesBackendTLSPolicyCustomCAPEMs(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
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
					"ca.crt": string(readTestTLSAsset(t, "client.crt")),
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: "orders.internal.example",
						CACertificateRefs: []gatewayv1.LocalObjectReference{{
							Name: "orders-ca",
						}},
						SubjectAltNames: []gatewayv1.SubjectAltName{
							{
								Type:     gatewayv1.HostnameSubjectAltNameType,
								Hostname: "orders.backend.svc",
							},
						},
					},
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}

	validation := snapshot.Backends[0].BackendTLSValidation
	if validation == nil {
		t.Fatal("expected backend tls validation")
	}
	if validation.UseSystemCAs {
		t.Fatal("expected custom CA validation to disable system CA validation")
	}
	if len(validation.CAPEMs) != 1 {
		t.Fatalf("expected 1 custom CA PEM, got %d", len(validation.CAPEMs))
	}
	if validation.CAPEMs[0] != string(readTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected custom CA PEM: %q", validation.CAPEMs[0])
	}
	if len(validation.SubjectAltNames) != 1 {
		t.Fatalf("expected 1 SAN, got %d", len(validation.SubjectAltNames))
	}
	if validation.SubjectAltNames[0].Type != "Hostname" {
		t.Fatalf("unexpected SAN type: %q", validation.SubjectAltNames[0].Type)
	}
}

func TestBuildSnapshotSkipsBackendTLSPolicyWithUnsupportedOptions(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "https", Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
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

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if snapshot.Backends[0].BackendTLSValidation != nil {
		t.Fatal("expected unsupported options to prevent backend TLS validation translation")
	}
}

func TestBuildSnapshotIncludesBackendTLSPolicyWhenAtLeastOneTargetIsValid(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "https", Port: 8443}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
								Group: "",
								Kind:  "Service",
								Name:  "orders",
							},
							SectionName: sectionNamePtr("https"),
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

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if snapshot.Backends[0].BackendTLSValidation == nil {
		t.Fatal("expected backend tls validation for valid target")
	}
}

func TestBuildSnapshotAppliesBackendTLSPolicyPrecedence(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-newer",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(time.Unix(20, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "newer.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-older",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "older.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if snapshot.Backends[0].BackendTLSValidation == nil {
		t.Fatal("expected backend tls validation")
	}
	if snapshot.Backends[0].BackendTLSValidation.Hostname != "older.internal.example" {
		t.Fatalf("unexpected winning hostname: %q", snapshot.Backends[0].BackendTLSValidation.Hostname)
	}
}

func TestBuildSnapshotPrefersSectionScopedBackendTLSPolicyOverCatchAll(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "https-1", Port: 443},
						{Name: "https-2", Port: 8443},
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-all-ports",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "shared.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-port-443",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(time.Unix(20, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https-1"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "port443.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	validations := map[string]string{}
	for _, backend := range snapshot.Backends {
		if backend.BackendTLSValidation != nil {
			validations[backend.Name] = backend.BackendTLSValidation.Hostname
		}
	}

	if validations["orders:443"] != "port443.internal.example" {
		t.Fatalf("unexpected 443 hostname: %q", validations["orders:443"])
	}
	if validations["orders:8443"] != "shared.internal.example" {
		t.Fatalf("unexpected 8443 hostname: %q", validations["orders:8443"])
	}
}

func TestBuildSnapshotKeepsValidBackendTLSCAPEMsWhenSomeRefsAreInvalid(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "orders-ca-valid", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": string(readTestTLSAsset(t, "client.crt")),
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
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

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	validation := snapshot.Backends[0].BackendTLSValidation
	if validation == nil {
		t.Fatal("expected backend tls validation")
	}
	if validation.UseSystemCAs {
		t.Fatal("expected custom CA validation to disable system CA validation")
	}
	if len(validation.CAPEMs) != 1 {
		t.Fatalf("expected 1 valid custom CA PEM, got %d", len(validation.CAPEMs))
	}
	if validation.CAPEMs[0] != string(readTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected custom CA PEM: %q", validation.CAPEMs[0])
	}
}

func TestBuildSnapshotSkipsBackendTLSPolicyWithInvalidSubjectAltName(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: sectionNamePtr("https"),
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

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if snapshot.Backends[0].BackendTLSValidation != nil {
		t.Fatal("expected invalid subjectAltName to prevent backend TLS validation translation")
	}
}

func TestBuildSnapshotExternalAuthWithBackendTLSPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	systemCA := gatewayv1.WellKnownCACertificatesSystem

	client := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "http",
						Port: 8080,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: "https",
						Port: 8443,
					}},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "auth",
						},
						SectionName: sectionNamePtr("https"),
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "auth.default.svc.cluster.local",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	var authBackend *ir.BackendCluster
	for i := range snapshot.Backends {
		if snapshot.Backends[i].Name == "auth:8443" {
			authBackend = &snapshot.Backends[i]
			break
		}
	}
	if authBackend == nil {
		t.Fatalf("expected auth backend cluster, got backends: %v",
			func() []string {
				out := make([]string, len(snapshot.Backends))
				for i, b := range snapshot.Backends {
					out[i] = b.Name
				}
				return out
			}())
	}

	validation := authBackend.BackendTLSValidation
	if validation == nil {
		t.Fatal("expected auth backend to have BackendTLSValidation from BackendTLSPolicy")
	}
	if validation.Hostname != "auth.default.svc.cluster.local" {
		t.Fatalf("unexpected auth validation hostname: %q", validation.Hostname)
	}
	if !validation.UseSystemCAs {
		t.Fatal("expected auth backend to use system CA certificates")
	}
}

func sectionNamePtr(value string) *gatewayv1.SectionName {
	name := gatewayv1.SectionName(value)
	return &name
}
