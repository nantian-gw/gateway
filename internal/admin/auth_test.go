package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		{"both set", Options{BearerToken: "abc", BearerTokenFile: "/tmp/token"}, true},
		{"empty strings", Options{BearerToken: "", BearerTokenFile: ""}, false},
		{"whitespace only", Options{BearerToken: "  ", BearerTokenFile: "  "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthConfigured(tt.opts); got != tt.want {
				t.Errorf("IsAuthConfigured() = %v, want %v", got, tt.want)
			}
		})
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
