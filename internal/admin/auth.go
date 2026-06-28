package admin

import (
	"crypto/subtle"
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nantian-gw/gateway/internal/observability"
)

type Options struct {
	BearerToken               string
	BearerTokenFile           string
	ReadOnlyBearerToken       string
	ReadOnlyBearerTokenFile   string
	ReadinessMode             string
	NodeDriftWarningThreshold time.Duration
	MaxRequestBodyBytes       int64
	MaxResponseBodyBytes      int64
	MaxListItems              int
	ReadHeaderTimeout         time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	Metrics                   *observability.Metrics
	TLSConfig                 *tls.Config
	Logger                    *slog.Logger
	RateLimitRPS              int64
	RateLimitBurst            int64
	DashboardCapabilities     DashboardCapabilities
}

func wrapAuthHandler(next http.Handler, opts Options) http.Handler {
	if !authConfigured(opts) {
		if opts.Logger != nil {
			opts.Logger.Warn("admin API running without authentication — set adminAuth.bearerToken or adminAuth.bearerTokenFile in config")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isProbePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			// When no auth is configured, allow read-only access so the
			// dashboard can function out of the box. Write operations still
			// require a bearer token.
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-controlplane-admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		fullToken, _ := resolveBearerToken(opts)
		isWrite := r.Method != http.MethodGet && r.Method != http.MethodHead
		if isWrite {
			if !isAuthorizedRequest(r, fullToken) {
				deny(w)
				return
			}
		} else {
			readOnlyToken, _ := resolveReadOnlyToken(opts)
			if !isAuthorizedRequest(r, fullToken) && !isAuthorizedRequest(r, readOnlyToken) {
				deny(w)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func deny(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-controlplane-admin"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func authConfigured(opts Options) bool {
	return strings.TrimSpace(opts.BearerTokenFile) != "" || strings.TrimSpace(opts.BearerToken) != ""
}

func resolveBearerToken(opts Options) (string, bool) {
	if path := strings.TrimSpace(opts.BearerTokenFile); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		token := strings.TrimSpace(string(raw))
		return token, token != ""
	}

	token := strings.TrimSpace(opts.BearerToken)
	return token, token != ""
}

func resolveReadOnlyToken(opts Options) (string, bool) {
	if path := strings.TrimSpace(opts.ReadOnlyBearerTokenFile); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		token := strings.TrimSpace(string(raw))
		return token, token != ""
	}

	token := strings.TrimSpace(opts.ReadOnlyBearerToken)
	return token, token != ""
}

func isAuthorizedRequest(r *http.Request, token string) bool {
	provided, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
	if !ok {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func bearerTokenFromHeader(value string) (string, bool) {
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

func isProbePath(path string) bool {
	return path == "/livez" || path == "/readyz"
}
