package main

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	nethttppprof "net/http/pprof"
	"os"
	"strings"
	"time"
)

const (
	defaultPprofReadHeaderTimeout = 5 * time.Second
	defaultPprofReadTimeout       = 30 * time.Second
	defaultPprofWriteTimeout      = 2 * time.Minute
	defaultPprofIdleTimeout       = 2 * time.Minute
	defaultPprofMaxHeaderBytes    = 32 << 10
)

func newPprofServer(addr string, bearerToken, bearerTokenFile string, logger *slog.Logger) *http.Server {
	handler := newPprofHandler()
	if tok := resolvePprofToken(bearerToken, bearerTokenFile); tok != "" {
		handler = pprofAuthMiddleware(handler, tok)
	} else if logger != nil {
		logger.Warn("pprof server running without authentication — set pprof.bearerToken or pprof.bearerTokenFile in config")
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: defaultPprofReadHeaderTimeout,
		ReadTimeout:       defaultPprofReadTimeout,
		WriteTimeout:      defaultPprofWriteTimeout,
		IdleTimeout:       defaultPprofIdleTimeout,
		MaxHeaderBytes:    defaultPprofMaxHeaderBytes,
	}
}

func newPprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", nethttppprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", nethttppprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", nethttppprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", nethttppprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", nethttppprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", nethttppprof.Trace)
	mux.Handle("GET /debug/pprof/{profile}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nethttppprof.Handler(r.PathValue("profile")).ServeHTTP(w, r)
	}))
	return mux
}

func resolvePprofToken(bearerToken, bearerTokenFile string) string {
	if path := strings.TrimSpace(bearerTokenFile); path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(bearerToken)
}

func pprofAuthMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided, ok := pprofBearerTokenFromHeader(r.Header.Get("Authorization"))
		if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-pprof"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func pprofBearerTokenFromHeader(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}
