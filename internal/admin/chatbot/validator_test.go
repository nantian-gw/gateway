package chatbot

import (
	"context"
	"strings"
	"testing"

	"github.com/nantian-gw/gateway/internal/ir"
)

// mockLLMClient implements LLMClient with canned responses used in
// DryRunValidate auto-correction tests.
type mockLLMClient struct {
	responses []string // Each call returns the next response.
	callCount int
}

func (m *mockLLMClient) ChatCompletionStream(_ context.Context, _ string, _ []Message, chunkChan chan<- string) error {
	if m.callCount >= len(m.responses) {
		chunkChan <- m.responses[len(m.responses)-1]
		return nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	// Simulate chunked streaming by sending the whole response as one chunk.
	chunkChan <- resp
	return nil
}

const testControllerName = "gateway.networking.k8s.io/nantian-gw"

func TestDryRunValidate_ValidGatewayAndHTTPRoute(t *testing.T) {
	t.Parallel()

	validYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: default
spec:
  gatewayClassName: nantian-gw
  listeners:
    - name: http
      port: 80
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api
  namespace: default
spec:
  parentRefs:
    - name: public
  rules:
    - backendRefs:
        - name: backend-svc
          port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
  namespace: default
spec:
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 8080
`

	snapshot, err := DryRunValidate(context.Background(), testControllerName, validYAML)
	if err != nil {
		t.Fatalf("unexpected error for valid YAML: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil *ir.Snapshot from valid YAML")
	}
	if !snapshot.GeneratedAt.IsZero() == false {
		t.Error("expected GeneratedAt to be set")
	}
}

func TestDryRunValidate_InvalidYAMLSyntax(t *testing.T) {
	t.Parallel()

	malformedYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind HTTPRoute
  metadata: [this is broken
    name: api
`

	_, err := DryRunValidate(context.Background(), testControllerName, malformedYAML)
	if err == nil {
		t.Fatal("expected error for malformed YAML syntax, got nil")
	}
	if !strings.Contains(err.Error(), "YAML to JSON") && !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected YAML parsing or decode error, got: %v", err)
	}
}

func TestDryRunValidate_InvalidYAMLMissingKind(t *testing.T) {
	t.Parallel()

	noKindYAML := `
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
`

	_, err := DryRunValidate(context.Background(), testControllerName, noKindYAML)
	if err == nil {
		t.Fatal("expected error for YAML without kind field, got nil")
	}
}

func TestDryRunValidate_SemanticallyInvalid_BadParentRef(t *testing.T) {
	t.Parallel()

	// HTTPRoute references a non-existent Gateway parent.
	// The translator builds a best-effort snapshot; orphan routes are listed
	// but not attached to any listener. This does not cause a build error.
	badParentYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orphan
  namespace: default
spec:
  parentRefs:
    - name: nonexistent-gateway
  rules:
    - backendRefs:
        - name: backend-svc
          port: 80
`

	snapshot, err := DryRunValidate(context.Background(), testControllerName, badParentYAML)
	if err != nil {
		t.Fatalf("translator should not error on orphan routes: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot even with orphan routes")
	}
}

func TestDryRunValidate_SemanticallyInvalid_BadBackendRef(t *testing.T) {
	t.Parallel()

	// HTTPRoute references a non-existent backend Service.
	// The translator handles missing backends gracefully and still produces
	// a snapshot; this does not cause a build error.
	badBackendYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: default
spec:
  gatewayClassName: nantian-gw
  listeners:
    - name: http
      port: 80
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: bogus
  namespace: default
spec:
  parentRefs:
    - name: public
  rules:
    - backendRefs:
        - name: no-such-service
          port: 80
`

	snapshot, err := DryRunValidate(context.Background(), testControllerName, badBackendYAML)
	if err != nil {
		t.Fatalf("translator should not error on missing backends: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot even with missing backends")
	}
}

func TestDryRunValidate_NoDocuments(t *testing.T) {
	t.Parallel()

	_, err := DryRunValidate(context.Background(), testControllerName, "")
	if err == nil {
		t.Fatal("expected error for empty YAML input")
	}
	if !strings.Contains(err.Error(), "no YAML documents") {
		t.Errorf("expected 'no YAML documents found' error, got: %v", err)
	}
}

func TestDryRunValidate_EmptyDocuments(t *testing.T) {
	t.Parallel()

	_, err := DryRunValidate(context.Background(), testControllerName, "---\n---\n")
	if err == nil {
		t.Fatal("expected error for input with only empty documents")
	}
	if !strings.Contains(err.Error(), "no YAML documents") {
		t.Errorf("expected 'no YAML documents found' error, got: %v", err)
	}
}

func TestAutoCorrectGenerate_SuccessOnFirstTry(t *testing.T) {
	t.Parallel()

	validResponse := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: default
spec:
  gatewayClassName: nantian-gw
  listeners:
    - name: http
      port: 80
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api
  namespace: default
spec:
  parentRefs:
    - name: public
  rules:
    - backendRefs:
        - name: backend-svc
          port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
  namespace: default
spec:
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 8080
`

	mockLLM := &mockLLMClient{
		responses: []string{validResponse},
	}

	snapshot, err := AutoCorrectGenerate(
		context.Background(),
		mockLLM,
		"You are a Gateway API assistant. Output only YAML.",
		testControllerName,
		"Create an HTTPRoute for the api service.",
		0,
	)
	if err != nil {
		t.Fatalf("unexpected error on valid generation: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil *ir.Snapshot")
	}
}

func TestAutoCorrectGenerate_RetrySucceedsAfterCorrection(t *testing.T) {
	t.Parallel()

	// First response is malformed YAML — will fail parsing.
	badResponse := "this is not valid YAML at all: [broken <<<"

	// Second response (after correction feedback) provides valid manifests.
	correctedResponse := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: default
spec:
  gatewayClassName: nantian-gw
  listeners:
    - name: http
      port: 80
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api
  namespace: default
spec:
  parentRefs:
    - name: public
  rules:
    - backendRefs:
        - name: backend-svc
          port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
  namespace: default
spec:
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 8080
`

	mockLLM := &mockLLMClient{
		responses: []string{badResponse, correctedResponse},
	}

	snapshot, err := AutoCorrectGenerate(
		context.Background(),
		mockLLM,
		"You are a Gateway API assistant. Output only YAML.",
		testControllerName,
		"Create an HTTPRoute for the api service.",
		2,
	)
	if err != nil {
		t.Fatalf("expected auto-correction to succeed on retry, got: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil *ir.Snapshot after correction")
	}

	if mockLLM.callCount != 2 {
		t.Errorf("expected 2 LLM calls (initial + correction), got %d", mockLLM.callCount)
	}

	_ = ir.Snapshot{}
}

func TestAutoCorrectGenerate_ExhaustsRetries(t *testing.T) {
	t.Parallel()

	// All responses are malformed — parsing will always fail.
	badResponse := "not: valid: yaml: <<<NOPE>>>"

	// Give more responses than retries — it should still stop at maxRetries.
	mockLLM := &mockLLMClient{
		responses: []string{badResponse, badResponse, badResponse, badResponse},
	}

	_, err := AutoCorrectGenerate(
		context.Background(),
		mockLLM,
		"You are a Gateway API assistant. Output only YAML.",
		testControllerName,
		"Create an HTTPRoute for the api service.",
		2,
	)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed after") {
		t.Errorf("expected 'validation failed after N retries' error, got: %v", err)
	}
}

func TestDryRunValidate_GRPCRoute(t *testing.T) {
	t.Parallel()

	grpcYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian-gw
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: grpc-gw
  namespace: default
spec:
  gatewayClassName: nantian-gw
  listeners:
    - name: grpc
      port: 50051
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: users
  namespace: default
spec:
  parentRefs:
    - name: grpc-gw
  rules:
    - matches:
        - method:
            service: users.UserService
      backendRefs:
        - name: grpc-backend
          port: 50051
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-backend
  namespace: default
spec:
  ports:
    - name: grpc
      port: 50051
      protocol: TCP
      targetPort: 50051
`

	snapshot, err := DryRunValidate(context.Background(), testControllerName, grpcYAML)
	if err != nil {
		t.Fatalf("unexpected error for valid GRPCRoute YAML: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil *ir.Snapshot from valid GRPCRoute")
	}
}

func TestDryRunValidate_NoManagedGatewayClass(t *testing.T) {
	t.Parallel()

	// When no GatewayClass is managed by the controller, translation still
	// produces a valid (empty) snapshot — it does not error.
	noGCYAML := `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: other-controller
spec:
  controllerName: some.other.controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: default
spec:
  gatewayClassName: other-controller
  listeners:
    - name: http
      port: 80
      protocol: HTTP
`

	snapshot, err := DryRunValidate(context.Background(), testControllerName, noGCYAML)
	if err != nil {
		t.Fatalf("translator should not error with unmanaged GatewayClass: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot even without managed GatewayClass")
	}
}
