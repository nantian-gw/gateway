package admin

import (
	"net/http/httptest"
	"testing"
)

func TestResourceMutationIdentityFallsBackToRequestBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "/v1/resources", nil)
	body := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  namespace: default
  name: created
`)

	kind, namespace, name := resourceMutationIdentity(req, body)
	if kind != "HTTPRoute" || namespace != "default" || name != "created" {
		t.Fatalf("unexpected identity: kind=%q namespace=%q name=%q", kind, namespace, name)
	}
}

func TestLimitedBufferRejectsWritesBeyondLimit(t *testing.T) {
	t.Parallel()

	buffer := newLimitedBuffer(4, errPayloadTooLarge("too large"))
	if _, err := buffer.Write([]byte("ping")); err != nil {
		t.Fatalf("expected first write to fit, got %v", err)
	}
	if string(buffer.Bytes()) != "ping" {
		t.Fatalf("unexpected buffer contents: %q", buffer.Bytes())
	}

	if _, err := buffer.Write([]byte("x")); !isPayloadTooLarge(err) {
		t.Fatalf("expected payload too large error, got %v", err)
	}
}
