package backends_test

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
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/translator"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildSnapshotIncludesBackendLBPolicySessionPersistence(t *testing.T) {
	scheme := runtime.NewScheme()
	testutil.Must(gatewayv1.Install(scheme), t)
	testutil.Must(gatewayv1alpha2.Install(scheme), t)
	testutil.Must(backend.Install(scheme), t)
	testutil.Must(gatewayv1alpha3.Install(scheme), t)
	testutil.Must(gatewayv1beta1.Install(scheme), t)
	testutil.Must(corev1.AddToScheme(scheme), t)
	testutil.Must(discoveryv1.AddToScheme(scheme), t)

	absolute := gatewayv1.Duration("5m")
	idle := gatewayv1.Duration("30s")
	sessionType := gatewayv1.HeaderBasedSessionPersistence
	sessionName := "x-orders-session"

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-sticky", Namespace: "default"},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{
						SessionName:     &sessionName,
						AbsoluteTimeout: &absolute,
						IdleTimeout:     &idle,
						Type:            &sessionType,
					},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}

	policy := snapshot.Backends[0].SessionPersistence
	if policy == nil {
		t.Fatal("expected backend session persistence")
	}
	if policy.SessionName != sessionName {
		t.Fatalf("unexpected session name: %q", policy.SessionName)
	}
	if policy.Type != "Header" {
		t.Fatalf("unexpected session type: %q", policy.Type)
	}
	if policy.AbsoluteTimeout == nil || *policy.AbsoluteTimeout != 5*time.Minute {
		t.Fatalf("unexpected absolute timeout: %#v", policy.AbsoluteTimeout)
	}
	if policy.IdleTimeout == nil || *policy.IdleTimeout != 30*time.Second {
		t.Fatalf("unexpected idle timeout: %#v", policy.IdleTimeout)
	}
	if policy.Cookie != nil {
		t.Fatalf("expected header session persistence to omit cookie config, got %#v", policy.Cookie)
	}
}

func TestBuildSnapshotIncludesBackendLBPolicyForServiceImport(t *testing.T) {
	scheme := runtime.NewScheme()
	testutil.Must(gatewayv1.Install(scheme), t)
	testutil.Must(gatewayv1alpha2.Install(scheme), t)
	testutil.Must(backend.Install(scheme), t)
	testutil.Must(gatewayv1alpha3.Install(scheme), t)
	testutil.Must(gatewayv1beta1.Install(scheme), t)
	testutil.Must(corev1.AddToScheme(scheme), t)
	testutil.Must(discoveryv1.AddToScheme(scheme), t)
	testutil.Must(mcsv1alpha1.AddToScheme(scheme), t)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&mcsv1alpha1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
				Spec: mcsv1alpha1.ServiceImportSpec{
					Type: mcsv1alpha1.ClusterSetIP,
					Ports: []mcsv1alpha1.ServicePort{{
						Name:     "grpc",
						Port:     9443,
						Protocol: corev1.ProtocolTCP,
					}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "payments-sticky", Namespace: "default"},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: mcsv1alpha1.GroupName,
						Kind:  "ServiceImport",
						Name:  "payments",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}

	policy := snapshot.Backends[0].SessionPersistence
	if policy == nil {
		t.Fatal("expected backend session persistence")
	}
	if policy.Type != "Cookie" {
		t.Fatalf("unexpected default session type: %q", policy.Type)
	}
	if policy.Cookie == nil || policy.Cookie.LifetimeType != "Session" {
		t.Fatalf("unexpected default cookie config: %#v", policy.Cookie)
	}
}

func TestBuildSnapshotIncludesBackendLBPolicyConsistentHash(t *testing.T) {
	scheme := runtime.NewScheme()
	testutil.Must(gatewayv1.Install(scheme), t)
	testutil.Must(gatewayv1alpha2.Install(scheme), t)
	testutil.Must(backend.Install(scheme), t)
	testutil.Must(gatewayv1alpha3.Install(scheme), t)
	testutil.Must(gatewayv1beta1.Install(scheme), t)
	testutil.Must(corev1.AddToScheme(scheme), t)
	testutil.Must(discoveryv1.AddToScheme(scheme), t)

	strategyType := backend.LoadBalancingStrategyTypeConsistentHash
	keyType := backend.HashKeyTypeHeader
	headerName := "x-user-id"

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-hash", Namespace: "default"},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					}},
					LoadBalancing: &backend.LoadBalancingPolicy{
						Type: &strategyType,
						ConsistentHash: &backend.ConsistentHashPolicy{
							KeyType:    &keyType,
							HeaderName: &headerName,
						},
					},
				},
			},
		).
		Build()

	snapshot, err := translator.New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}

	policy := snapshot.Backends[0].LoadBalancing
	if policy == nil {
		t.Fatal("expected backend load balancing policy")
	}
	if policy.Type != "ConsistentHash" {
		t.Fatalf("unexpected load balancing type: %q", policy.Type)
	}
	if policy.ConsistentHash == nil {
		t.Fatal("expected consistent hash policy")
	}
	if policy.ConsistentHash.KeyType != "Header" {
		t.Fatalf("unexpected consistent hash key type: %q", policy.ConsistentHash.KeyType)
	}
	if policy.ConsistentHash.HeaderName != headerName {
		t.Fatalf("unexpected consistent hash header name: %q", policy.ConsistentHash.HeaderName)
	}
}
