package tls

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFallbackCertManagerPersistsLeafAcrossManagers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"}}).
		Build()
	ctx := context.Background()

	firstManager := NewFallbackCertManager()
	if err := firstManager.LoadOrCreateCA(ctx, cl, "nantian-gw"); err != nil {
		t.Fatalf("first LoadOrCreateCA: %v", err)
	}
	firstLeaf, err := firstManager.IssueLeafCert(ctx, cl, "nantian-gw", []string{"default/gw/https"})
	if err != nil {
		t.Fatalf("first IssueLeafCert: %v", err)
	}

	secondManager := NewFallbackCertManager()
	if err := secondManager.LoadOrCreateCA(ctx, cl, "nantian-gw"); err != nil {
		t.Fatalf("second LoadOrCreateCA: %v", err)
	}
	secondLeaf, err := secondManager.IssueLeafCert(ctx, cl, "nantian-gw", []string{"default/gw/https"})
	if err != nil {
		t.Fatalf("second IssueLeafCert: %v", err)
	}

	if firstLeaf.CertPEM != secondLeaf.CertPEM || firstLeaf.KeyPEM != secondLeaf.KeyPEM {
		t.Fatal("expected persisted fallback leaf material to be stable across managers")
	}
}
