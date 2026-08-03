package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
)

func newChatbotTestServer(t *testing.T, k8sObjects ...client.Object) *Server {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sObjects...).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		logger,
		noderegistry.Options{PersistTimeout: time.Second},
	)

	return NewServer(
		":0",
		store,
		nodes,
		NewResourceManager(k8sClient, logger),
		logger,
		Options{BearerToken: testAuthToken},
	)
}

func testSecret(apiKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chatbot-config",
			Namespace: "nantian-gw",
		},
		Data: map[string][]byte{
			"provider":     []byte("openai"),
			"api-endpoint": []byte("https://api.openai.com/v1"),
			"api-key":      []byte(apiKey),
			"model":        []byte("gpt-4o"),
			"temperature":  []byte("0.1"),
		},
	}
}

func testSecretWithEndpoint(apiKey, endpoint string) *corev1.Secret {
	secret := testSecret(apiKey)
	secret.Data["api-endpoint"] = []byte(endpoint)
	return secret
}

func TestGetChatbotConfig_ReturnsMaskedConfig(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t, testSecret("sk-proj-abc123xyz"))

	recorder := performRequest(t, server, http.MethodGet, "/v1/chatbot/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got, ok := resp["configured"]; !ok || got != true {
		t.Fatalf("expected configured=true, got %v", got)
	}
	if got := resp["provider"].(string); got != "openai" {
		t.Errorf("provider = %q, want %q", got, "openai")
	}
	if got := resp["apiEndpoint"].(string); got != "https://api.openai.com/v1" {
		t.Errorf("apiEndpoint = %q, want %q", got, "https://api.openai.com/v1")
	}
	if got := resp["model"].(string); got != "gpt-4o" {
		t.Errorf("model = %q, want %q", got, "gpt-4o")
	}

	apiKey, _ := resp["apiKey"].(string)
	if apiKey == "sk-proj-abc123xyz" { //nolint:gosec
		t.Errorf("API key must be masked, got raw: %q", apiKey)
	}
	if !strings.Contains(apiKey, "••••") {
		t.Errorf("expected masked API key (contains ••••), got: %q", apiKey)
	}
	if !strings.HasPrefix(apiKey, "sk-") {
		t.Errorf("masked key should start with 'sk-', got: %q", apiKey)
	}
	if !strings.HasSuffix(apiKey, "3xyz") {
		t.Errorf("masked key should end with '3xyz', got: %q", apiKey)
	}
}

func TestGetChatbotConfig_NoSecretReturnsNotConfigured(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/chatbot/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got, ok := resp["configured"]; !ok || got != false {
		t.Fatalf("expected configured=false, got %v", got)
	}
	if got := resp["apiKey"].(string); got != "" {
		t.Errorf("expected empty apiKey, got %q", got)
	}
}

func TestPutChatbotConfig_CreatesNewSecret(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t)

	body := []byte(`{
		"provider": "openai",
		"apiEndpoint": "https://api.openai.com/v1",
		"apiKey": "sk-fresh-key",
		"model": "gpt-4o",
		"temperature": 0.1
	}`)

	recorder := performRequestWithBody(t, server, http.MethodPut, "/v1/chatbot/config", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	secret := &corev1.Secret{}
	err := server.resources.client.Get(
		context.Background(),
		client.ObjectKey{Namespace: "nantian-gw", Name: "chatbot-config"},
		secret,
	)
	if err != nil {
		t.Fatalf("failed to read created secret: %v", err)
	}

	if got := string(secret.Data["provider"]); got != "openai" {
		t.Errorf("provider = %q, want %q", got, "openai")
	}
	if got := string(secret.Data["api-key"]); got != "sk-fresh-key" {
		t.Errorf("api-key = %q, want %q", got, "sk-fresh-key")
	}
	if got := string(secret.Data["model"]); got != "gpt-4o" {
		t.Errorf("model = %q, want %q", got, "gpt-4o")
	}
}

func TestPutChatbotConfig_MaskedKeyPreservesExisting(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t, testSecret("sk-original-key"))

	body := []byte(`{
		"provider": "deepseek",
		"apiEndpoint": "https://api.deepseek.com/v1",
		"apiKey": "sk-••••4key"
	}`)

	recorder := performRequestWithBody(t, server, http.MethodPut, "/v1/chatbot/config", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	secret := &corev1.Secret{}
	err := server.resources.client.Get(
		context.Background(),
		client.ObjectKey{Namespace: "nantian-gw", Name: "chatbot-config"},
		secret,
	)
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if got := string(secret.Data["api-key"]); got != "sk-original-key" {
		t.Errorf("api-key = %q, want %q", got, "sk-original-key")
	}
	if got := string(secret.Data["provider"]); got != "deepseek" {
		t.Errorf("provider = %q, want %q", got, "deepseek")
	}
}

func TestPutChatbotConfig_EmptyKeyPreservesExisting(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t, testSecret("sk-original-key"))

	body := []byte(`{
		"provider": "openai",
		"apiEndpoint": "https://api.openai.com/v1",
		"apiKey": "",
		"model": "gpt-4-turbo"
	}`)

	recorder := performRequestWithBody(t, server, http.MethodPut, "/v1/chatbot/config", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	secret := &corev1.Secret{}
	err := server.resources.client.Get(
		context.Background(),
		client.ObjectKey{Namespace: "nantian-gw", Name: "chatbot-config"},
		secret,
	)
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if got := string(secret.Data["api-key"]); got != "sk-original-key" {
		t.Errorf("api-key = %q, want %q", got, "sk-original-key")
	}
	if got := string(secret.Data["model"]); got != "gpt-4-turbo" {
		t.Errorf("model = %q, want %q", got, "gpt-4-turbo")
	}
}

func TestPostChatbotChat_ReturnsSSEStream(t *testing.T) {
	t.Parallel()

	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher on mock server")
		}

		chunks := []string{
			`data: {"choices": [{"delta": {"content": "Hello"}, "index": 0}]}`,
			`data: {"choices": [{"delta": {"content": " from"}, "index": 0}]}`,
			`data: {"choices": [{"delta": {"content": " the chatbot"}, "index": 0}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk + "\n"))
			flusher.Flush()
		}
	}))
	defer mockOpenAI.Close()

	server := newChatbotTestServer(
		t,
		testSecretWithEndpoint("sk-test-key", mockOpenAI.URL),
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	)

	body := []byte(`{"prompt": "What is a Gateway?"}`)

	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/chatbot/chat", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	respBody := recorder.Body.String()

	if ct := recorder.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	if !strings.Contains(respBody, "data: Hello") {
		t.Errorf("response missing 'data: Hello': %s", respBody)
	}
	if !strings.Contains(respBody, "data:  from") {
		t.Errorf("response missing 'data:  from': %s", respBody)
	}
	if !strings.Contains(respBody, "data:  the chatbot") {
		t.Errorf("response missing 'data:  the chatbot': %s", respBody)
	}
	if !strings.Contains(respBody, "data: [DONE]") {
		t.Errorf("response missing 'data: [DONE]': %s", respBody)
	}
}

func TestPostChatbotChat_NotConfiguredReturnsError(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t)

	body := []byte(`{"prompt": "What is a Gateway?"}`)

	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/chatbot/chat", body)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unconfigured chatbot, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "not configured") {
		t.Errorf("expected 'not configured' message, got: %s", recorder.Body.String())
	}
}

func TestPostChatbotChat_EmptyPromptReturnsError(t *testing.T) {
	t.Parallel()

	server := newChatbotTestServer(t, testSecret("sk-test-key"))

	body := []byte(`{"prompt": ""}`)

	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/chatbot/chat", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty prompt, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "prompt is required") {
		t.Errorf("expected 'prompt is required' message, got: %s", recorder.Body.String())
	}
}

func TestPostChatbotChat_StreamsWithHistory(t *testing.T) {
	t.Parallel()

	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices": [{"delta": {"content": "Acknowledged."}, "index": 0}]}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n"))
	}))
	defer mockOpenAI.Close()

	server := newChatbotTestServer(
		t,
		testSecretWithEndpoint("sk-test-key", mockOpenAI.URL),
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	)

	body := []byte(`{
		"prompt": "Thanks",
		"history": [
			{"role": "user", "content": "What is a Gateway?"},
			{"role": "assistant", "content": "A Gateway manages inbound traffic."}
		]
	}`)

	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/chatbot/chat", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	respBody := recorder.Body.String()
	if !strings.Contains(respBody, "data: Acknowledged") {
		t.Errorf("response missing expected token: %s", respBody)
	}
	if !strings.Contains(respBody, "data: [DONE]") {
		t.Errorf("response missing [DONE] signal: %s", respBody)
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"standard key", "sk-proj-abc123xyz", "sk-••••3xyz"},
		{"short key", "abc", "***"},
		{"exactly 7 chars", "1234567", "*******"},
		{"8 chars", "12345678", "123••••5678"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskAPIKey(tt.input)
			if got != tt.expect {
				t.Errorf("maskAPIKey(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestIsMaskedKey(t *testing.T) {
	t.Parallel()

	if !isMaskedKey("sk-••••3xyz") {
		t.Error("expected 'sk-••••3xyz' to be detected as masked")
	}
	if isMaskedKey("sk-fresh-clear-key") {
		t.Error("expected clear key to NOT be detected as masked")
	}
	if isMaskedKey("") {
		t.Error("expected empty key to NOT be detected as masked")
	}
}
