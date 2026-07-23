package xds

import (
	"testing"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestToProtoBackendTLS(t *testing.T) {
	item := toProtoBackendTLS(&ir.BackendTLSConfig{
		ClientCertificateRef: "default/client-cert",
	})
	if item == nil {
		t.Fatal("expected backend tls proto")
	}
	if item.ClientCertificateRef != "default/client-cert" {
		t.Fatalf("unexpected client certificate ref: %q", item.ClientCertificateRef)
	}
}

func TestToProtoBackendTLSValidation(t *testing.T) {
	item := toProtoBackendTLSValidation(&ir.BackendTLSValidation{
		Hostname:     "orders.internal.example",
		UseSystemCAs: true,
		CAPEMs:       []string{"PEM-A", "PEM-B"},
		MinVersion:   "TLS1_2",
		MaxVersion:   "TLS1_3",
		SubjectAltNames: []ir.BackendSubjectName{
			{Type: "Hostname", Value: "orders.backend.svc"},
			{Type: "URI", Value: "spiffe://cluster.local/ns/default/sa/orders"},
		},
	})
	if item == nil {
		t.Fatal("expected backend tls validation proto")
	}
	if item.Hostname != "orders.internal.example" {
		t.Fatalf("unexpected hostname: %q", item.Hostname)
	}
	if !item.UseSystemCaCertificates {
		t.Fatal("expected system CA flag to be true")
	}
	if len(item.CaPems) != 2 {
		t.Fatalf("unexpected CA PEM count: %d", len(item.CaPems))
	}
	if item.MinVersion != "TLS1_2" {
		t.Fatalf("unexpected min version: %q", item.MinVersion)
	}
	if item.MaxVersion != "TLS1_3" {
		t.Fatalf("unexpected max version: %q", item.MaxVersion)
	}
	if len(item.SubjectAltNames) != 2 {
		t.Fatalf("unexpected SAN count: %d", len(item.SubjectAltNames))
	}
	if item.SubjectAltNames[0].Type != controlv1.BackendTlsSubjectAltNameType_BACKEND_TLS_SUBJECT_ALT_NAME_TYPE_HOSTNAME {
		t.Fatalf("unexpected first SAN type: %v", item.SubjectAltNames[0].Type)
	}
}
