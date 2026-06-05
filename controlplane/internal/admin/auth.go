package admin

import (
	"crypto/tls"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/observability"
)

type Options struct {
	BearerToken               string
	BearerTokenFile           string
	ReadinessMode             string
	NodeDriftWarningThreshold time.Duration
	MaxRequestBodyBytes       int64
	MaxResponseBodyBytes      int64
	ReadHeaderTimeout         time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	Metrics                   *observability.Metrics
	TLSConfig                 *tls.Config
}

func wrapAuthHandler(next http.Handler, opts Options) http.Handler {
	if !authConfigured(opts) {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := resolveBearerToken(opts)
		if !ok || !isAuthorizedRequest(r, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="aether-gateway-controlplane-admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
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
