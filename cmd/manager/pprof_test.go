package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPprofHandlerServesIndexAndNamedProfiles(t *testing.T) {
	t.Parallel()

	handler := newPprofHandler()
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "index",
			method: http.MethodGet,
			path:   "/debug/pprof/",
		},
		{
			name:   "goroutine",
			method: http.MethodGet,
			path:   "/debug/pprof/goroutine",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, rec.Code, http.StatusOK)
			}
		})
	}
}

func TestNewPprofServerAppliesRuntimeLimits(t *testing.T) {
	server := newPprofServer("127.0.0.1:6060", "", "", nil)

	if server.Addr != "127.0.0.1:6060" {
		t.Fatalf("pprof server addr = %q, want 127.0.0.1:6060", server.Addr)
	}
	if server.Handler == nil {
		t.Fatal("expected pprof server handler")
	}
	if server.ReadHeaderTimeout != defaultPprofReadHeaderTimeout {
		t.Fatalf("pprof read header timeout = %s, want %s", server.ReadHeaderTimeout, defaultPprofReadHeaderTimeout)
	}
	if server.ReadTimeout != defaultPprofReadTimeout {
		t.Fatalf("pprof read timeout = %s, want %s", server.ReadTimeout, defaultPprofReadTimeout)
	}
	if server.WriteTimeout != defaultPprofWriteTimeout {
		t.Fatalf("pprof write timeout = %s, want %s", server.WriteTimeout, defaultPprofWriteTimeout)
	}
	if server.IdleTimeout != defaultPprofIdleTimeout {
		t.Fatalf("pprof idle timeout = %s, want %s", server.IdleTimeout, defaultPprofIdleTimeout)
	}
	if server.MaxHeaderBytes != defaultPprofMaxHeaderBytes {
		t.Fatalf("pprof max header bytes = %d, want %d", server.MaxHeaderBytes, defaultPprofMaxHeaderBytes)
	}
}

func TestPprofAuthMiddleware(t *testing.T) {
	token := "test-pprof-token"

	t.Run("no auth header returns 401", func(t *testing.T) {
		handler := pprofAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), token)

		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		handler := pprofAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), token)

		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("correct token passes through", func(t *testing.T) {
		handler := pprofAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), token)

		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestNewPprofServerWithAuth(t *testing.T) {
	token := "secure-token"
	server := newPprofServer("127.0.0.1:6060", token, "", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without auth header, status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with correct token, status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewPprofServerWithTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "pprof-token")
	if err := os.WriteFile(tokenPath, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := newPprofServer("127.0.0.1:6060", "", tokenPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer file-token")
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResolvePprofToken(t *testing.T) {
	t.Run("direct token", func(t *testing.T) {
		got := resolvePprofToken("my-token", "")
		if got != "my-token" {
			t.Fatalf("got %q, want my-token", got)
		}
	})

	t.Run("token file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "token")
		if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := resolvePprofToken("", path)
		if got != "file-token" {
			t.Fatalf("got %q, want file-token", got)
		}
	})

	t.Run("empty returns empty", func(t *testing.T) {
		got := resolvePprofToken("", "")
		if got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}