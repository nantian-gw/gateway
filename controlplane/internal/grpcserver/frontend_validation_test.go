package grpcserver

import (
	"testing"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func TestToProtoFrontendValidation(t *testing.T) {
	item := toProtoFrontendValidation(&ir.FrontendValidation{
		ClientCAPEMs: []string{"CA1", "CA2"},
		Mode:         "AllowInsecureFallback",
	})
	if item == nil {
		t.Fatal("expected frontend validation proto")
	}
	if len(item.CaPems) != 2 {
		t.Fatalf("expected 2 ca pems, got %d", len(item.CaPems))
	}
	if item.Mode != "AllowInsecureFallback" {
		t.Fatalf("expected insecure fallback mode, got %q", item.Mode)
	}
}

func TestToProtoFrontendValidationKeepsRejectModeWithoutCAPEMs(t *testing.T) {
	item := toProtoFrontendValidation(&ir.FrontendValidation{
		Mode: "RejectClientCertificate",
	})
	if item == nil {
		t.Fatal("expected rejection mode to be serialized without CA PEMs")
	}
	if len(item.CaPems) != 0 {
		t.Fatalf("expected no ca pems, got %d", len(item.CaPems))
	}
	if item.Mode != "RejectClientCertificate" {
		t.Fatalf("expected rejection mode, got %q", item.Mode)
	}
}
