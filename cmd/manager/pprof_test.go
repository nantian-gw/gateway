package main

import (
	"net/http"
	"net/http/httptest"
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
	server := newPprofServer("127.0.0.1:6060")

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
