package backendtls

import (
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseOptionsRejectsBackendTLSVersionOptions(t *testing.T) {
	_, err := ParseOptions(map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
		OptionMinVersion: "tlsv1.2",
		OptionMaxVersion: "1.3",
	})
	if err != nil {
		if got, want := err.Error(), `BackendTLSPolicy option "gateway.nantian.dev/backend-tls-min-version" is not supported by the upstream runtime`; got != want {
			t.Fatalf("unexpected error: %q, want %q", got, want)
		}
		return
	}

	t.Fatal("expected ParseOptions to reject backend TLS version options")
}

func TestParseOptionsRejectsUnsupportedKey(t *testing.T) {
	_, err := ParseOptions(map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
		"example.com/unknown": "value",
	})
	if err == nil {
		t.Fatal("expected ParseOptions to reject unsupported key")
	}
}

func TestParseOptionsAcceptsEmptyOptions(t *testing.T) {
	options, err := ParseOptions(nil)
	if err == nil {
		if options != (Options{}) {
			t.Fatalf("unexpected parsed options: %#v", options)
		}
		return
	}

	t.Fatalf("ParseOptions returned error: %v", err)
}
