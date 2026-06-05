package backendtls

import (
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseSubjectAltNamesAcceptsSingleHostname(t *testing.T) {
	items, err := ParseSubjectAltNames([]gatewayv1.SubjectAltName{
		{
			Type:     gatewayv1.HostnameSubjectAltNameType,
			Hostname: "orders.default.svc",
		},
	})
	if err != nil {
		t.Fatalf("ParseSubjectAltNames returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 subject alt name, got %d", len(items))
	}
	if items[0].Type != "Hostname" || items[0].Value != "orders.default.svc" {
		t.Fatalf("unexpected hostname subject alt name: %#v", items[0])
	}
}

func TestParseSubjectAltNamesRejectsHostnameWithURIField(t *testing.T) {
	_, err := ParseSubjectAltNames([]gatewayv1.SubjectAltName{{
		Type:     gatewayv1.HostnameSubjectAltNameType,
		Hostname: "orders.default.svc",
		URI:      "spiffe://cluster.local/ns/default/sa/orders",
	}})
	if err == nil {
		t.Fatal("expected ParseSubjectAltNames to reject Hostname SAN with URI field")
	}
}

func TestParseSubjectAltNamesAcceptsURIAndHostnameEntries(t *testing.T) {
	items, err := ParseSubjectAltNames([]gatewayv1.SubjectAltName{
		{
			Type:     gatewayv1.HostnameSubjectAltNameType,
			Hostname: "orders.default.svc",
		},
		{
			Type: gatewayv1.URISubjectAltNameType,
			URI:  "spiffe://cluster.local/ns/default/sa/orders",
		},
	})
	if err != nil {
		t.Fatalf("ParseSubjectAltNames returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 subject alt names, got %d", len(items))
	}
	if items[0].Type != "Hostname" || items[0].Value != "orders.default.svc" {
		t.Fatalf("unexpected hostname subject alt name: %#v", items[0])
	}
	if items[1].Type != "URI" || items[1].Value != "spiffe://cluster.local/ns/default/sa/orders" {
		t.Fatalf("unexpected URI subject alt name: %#v", items[1])
	}
}

func TestParseSubjectAltNamesAcceptsMultipleHostnames(t *testing.T) {
	items, err := ParseSubjectAltNames([]gatewayv1.SubjectAltName{
		{
			Type:     gatewayv1.HostnameSubjectAltNameType,
			Hostname: "orders.default.svc",
		},
		{
			Type:     gatewayv1.HostnameSubjectAltNameType,
			Hostname: "orders.alt.svc",
		},
	})
	if err != nil {
		t.Fatalf("ParseSubjectAltNames returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 subject alt names, got %d", len(items))
	}
	if items[0].Value != "orders.default.svc" || items[1].Value != "orders.alt.svc" {
		t.Fatalf("unexpected hostname subject alt names: %#v", items)
	}
}

func TestParseSubjectAltNamesRejectsRelativeURI(t *testing.T) {
	_, err := ParseSubjectAltNames([]gatewayv1.SubjectAltName{{
		Type: gatewayv1.URISubjectAltNameType,
		URI:  "/ns/default/sa/orders",
	}})
	if err == nil {
		t.Fatal("expected ParseSubjectAltNames to reject relative URI SAN")
	}
}
