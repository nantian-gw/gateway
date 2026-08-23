package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type authVerifyTestResponse struct {
	Authenticated bool   `json:"authenticated"`
	Reason        string `json:"reason"`
	Subject       string `json:"subject"`
}

func TestBearerTokenFromHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantOK    bool
		wantToken string
	}{
		{"valid bearer", "Bearer abc123", true, "abc123"},
		{"valid bearer with extra whitespace", "Bearer  abc123  ", true, "abc123"},
		{"missing Bearer prefix", "abc123", false, ""},
		{"empty value", "", false, ""},
		{"Basic auth (not Bearer)", "Basic YWxhZGRpbjpvcGVuc2VzYW1l", false, ""},
		{"Bearer with no token", "Bearer ", false, ""},
		{"lowercase bearer", "bearer abc123", true, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := bearerTokenFromHeader(tt.header)
			if ok != tt.wantOK {
				t.Errorf("bearerTokenFromHeader(%q) ok = %v, want %v", tt.header, ok, tt.wantOK)
			}
			if ok && token != tt.wantToken {
				t.Errorf("bearerTokenFromHeader(%q) = %q, want %q", tt.header, token, tt.wantToken)
			}
		})
	}
}

func TestIsProbePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/livez", true},
		{"/readyz", true},
		{"/v1/summary", false},
		{"/", false},
		{"/livez/extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isProbePath(tt.path); got != tt.want {
				t.Errorf("isProbePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestAuthConfigured(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want bool
	}{
		{"no auth", Options{}, false},
		{"bearerToken set", Options{BearerToken: "abc"}, true},
		{"bearerTokenFile set", Options{BearerTokenFile: "/tmp/token"}, true},
		{"readOnlyBearerToken set", Options{ReadOnlyBearerToken: "abc"}, true},
		{"readOnlyBearerTokenFile set", Options{ReadOnlyBearerTokenFile: "/tmp/token"}, true},
		{"both set", Options{BearerToken: "abc", BearerTokenFile: "/tmp/token"}, true},
		{"empty strings", Options{BearerToken: "", BearerTokenFile: ""}, false},
		{
			"whitespace only",
			Options{
				BearerToken:             "  ",
				BearerTokenFile:         "  ",
				ReadOnlyBearerToken:     "  ",
				ReadOnlyBearerTokenFile: "  ",
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthConfigured(tt.opts); got != tt.want {
				t.Errorf("IsAuthConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthVerifyAcceptsStaticFullToken(t *testing.T) {
	server := newTestServerWithOptions(t, Options{BearerToken: "write-secret"})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/verify", http.NoBody)
	req.Header.Set("Authorization", "Bearer write-secret")
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got authVerifyTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode auth verify response: %v", err)
	}
	if !got.Authenticated || got.Subject != "static-token" {
		t.Fatalf("unexpected auth verify response: %+v", got)
	}
}

func TestAuthVerifyAcceptsStaticReadOnlyToken(t *testing.T) {
	server := newTestServerWithOptions(t, Options{
		BearerToken:         "write-secret",
		ReadOnlyBearerToken: "read-secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/verify", http.NoBody)
	req.Header.Set("Authorization", "Bearer read-secret")
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got authVerifyTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode auth verify response: %v", err)
	}
	if !got.Authenticated || got.Subject != "static-read-only-token" {
		t.Fatalf("unexpected auth verify response: %+v", got)
	}
}

func TestAuthVerifyAcceptsMiddlewareAuthenticatedIdentity(t *testing.T) {
	server := &Server{maxResponseBodyBytes: defaultMaxResponseBodyBytes}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/verify", http.NoBody)
	req.Header.Set("Authorization", "bearer kubernetes-token")
	req = req.WithContext(context.WithValue(req.Context(), identityKey, &Identity{
		Username: "system:serviceaccount:nantian-gw:dashboard",
		Groups:   []string{"system:serviceaccounts"},
		Subject:  "system:serviceaccount:nantian-gw:dashboard",
	}))
	rec := httptest.NewRecorder()

	server.handleAuthVerify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got authVerifyTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode auth verify response: %v", err)
	}
	if !got.Authenticated || got.Subject != "system:serviceaccount:nantian-gw:dashboard" {
		t.Fatalf("unexpected auth verify response: %+v", got)
	}
}

func TestAuthVerifyRejectsNoAuthAnonymousIdentity(t *testing.T) {
	server := &Server{maxResponseBodyBytes: defaultMaxResponseBodyBytes}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/verify", http.NoBody)
	req.Header.Set("Authorization", "Bearer any-token")
	req = req.WithContext(context.WithValue(req.Context(), identityKey, &Identity{Subject: "anonymous"}))
	rec := httptest.NewRecorder()

	server.handleAuthVerify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got authVerifyTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode auth verify response: %v", err)
	}
	if got.Authenticated || got.Reason != "invalid" {
		t.Fatalf("unexpected auth verify response: %+v", got)
	}
}

func TestNoAuthHandlerAllowsGetBlocksPost(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := noAuthHandler(okHandler, Options{})

	// GET should pass through
	req := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /v1/summary: got %d, want %d", rec.Code, http.StatusOK)
	}

	// POST should be blocked
	req = httptest.NewRequest(http.MethodPost, "/v1/resources", http.NoBody)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /v1/resources: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Probes always pass
	for _, path := range []string{"/livez", "/readyz"} {
		req = httptest.NewRequest(http.MethodPost, path, http.NoBody)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: got %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestKubernetesAuthHandlerRejectsMissingToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := kubernetesAuthHandler(okHandler, Options{
		AuthMode: "kubernetes",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no Authorization header: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestKubernetesAuthHandlerAllowsProbes(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := kubernetesAuthHandler(okHandler, Options{AuthMode: "kubernetes"})

	for _, path := range []string{"/livez", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
