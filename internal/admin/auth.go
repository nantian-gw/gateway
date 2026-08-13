package admin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/nantian-gw/gateway/internal/observability"
	"github.com/nantian-gw/gateway/internal/constants"
)

// contextKey is an unexported type used as a context key for admin request identity.
type contextKey string

// identityKey stores the authenticated caller identity in the request context.
const identityKey contextKey = "admin-identity"

// Identity holds the authenticated caller information extracted during auth.
type Identity struct {
	Username string   // Kubernetes user (empty for static-token auth)
	Groups   []string // Kubernetes groups (empty for static-token auth)
	Subject  string   // Human-readable identity label for audit logs
}

// IdentityFromContext extracts the authenticated Identity from the request context.
// Returns nil when no auth was configured (no-auth mode).
func IdentityFromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}

type Options struct {
	BearerToken               string
	BearerTokenFile           string
	ReadOnlyBearerToken       string
	ReadOnlyBearerTokenFile   string
	AuthMode                  string // "static" (default) or "kubernetes"
	RestConfig                *rest.Config
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
	RBAC                      *AdminRBACConfig
	RateLimitRPS              int64
	RateLimitBurst            int64
	TokenReviewAudiences      []string
	AllowedUsers              []string
	AllowedGroups             []string
	TrustedProxies            []string
	DashboardCapabilities     DashboardCapabilities
	AllowFromCIDRs            []string
}

func wrapAuthHandler(next http.Handler, opts Options) http.Handler {
	mode := strings.TrimSpace(opts.AuthMode)
	if mode == "" {
		mode = "static"
	}

	var handler http.Handler

	switch mode {
	case "kubernetes":
		if opts.RestConfig == nil {
			if opts.Logger != nil {
				opts.Logger.Error("authMode=kubernetes requires a valid RestConfig; falling back to no-auth mode")
			}
			handler = noAuthHandler(next, opts)
		} else {
			handler = kubernetesAuthHandler(next, opts)
		}
	case "static":
		if !IsAuthConfigured(opts) {
			handler = noAuthHandler(next, opts)
		} else {
			handler = staticAuthHandler(next, opts)
		}
	default:
		if opts.Logger != nil {
			opts.Logger.Warn("unknown authMode, falling back to no-auth mode", "mode", mode)
		}
		handler = noAuthHandler(next, opts)
	}

	return ipWhitelistMiddleware(handler, opts)
}

// ipWhitelistMiddleware wraps a handler to restrict access by client IP.  Probes
// (/livez, /readyz) are allowed through regardless.  When AllowFromCIDRs is empty
// (the default) no IP restriction is applied.  When CIDRs are configured, the
// middleware extracts the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr
// and returns 403 if the IP does not match any configured CIDR.
func ipWhitelistMiddleware(next http.Handler, opts Options) http.Handler {
	cidrs := opts.AllowFromCIDRs
	if len(cidrs) == 0 {
		return next
	}

	parsed := parseCIDRs(cidrs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r)
		if ip == nil || !cidrContains(parsed, ip) {
			if opts.Logger != nil {
				opts.Logger.Warn("admin API request denied by IP whitelist",
					"remote", r.RemoteAddr,
					"xff", r.Header.Get("X-Forwarded-For"),
				)
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the client IP from the request, checking X-Forwarded-For,
// X-Real-IP, and RemoteAddr in that order.
func clientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the leftmost (original client) IP.
		parts := strings.SplitN(xff, ",", 2)
		addr := strings.TrimSpace(parts[0])
		if ip := net.ParseIP(addr); ip != nil {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		addr := strings.TrimSpace(xri)
		if ip := net.ParseIP(addr); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

func parseCIDRs(entries []string) []*net.IPNet {
	parsed := make([]*net.IPNet, 0, len(entries))
	for _, raw := range entries {
		_, cidr, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		parsed = append(parsed, cidr)
	}
	return parsed
}

func cidrContains(cidrs []*net.IPNet, ip net.IP) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// noAuthHandler allows GET/HEAD but blocks writes when no auth is configured.
func noAuthHandler(next http.Handler, opts Options) http.Handler {
	if opts.Logger != nil {
		opts.Logger.Warn("admin API running without authentication — set adminAuth.authMode=kubernetes or adminAuth.bearerToken in config")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			r = r.WithContext(context.WithValue(r.Context(), identityKey, &Identity{
				Subject: "anonymous",
			}))
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-controlplane-admin"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// kubernetesAuthHandler validates tokens via the Kubernetes TokenReview API.
func kubernetesAuthHandler(next http.Handler, opts Options) http.Handler {
	var clientset kubernetes.Interface
	if opts.RestConfig != nil {
		cs, err := kubernetes.NewForConfig(opts.RestConfig)
		if err != nil {
			if opts.Logger != nil {
				opts.Logger.Error("failed to create kubernetes clientset for TokenReview", "error", err)
			}
		} else {
			clientset = cs
		}
	}

	auth := &tokenReviewAuthenticator{
		clientset: clientset,
		audiences: opts.TokenReviewAudiences,
		users:     toSet(opts.AllowedUsers),
		groups:    toSet(opts.AllowedGroups),
		logger:    opts.Logger,
		cacheTTL:  30 * time.Second,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
		if !ok {
			deny(w)
			return
		}

		result, err := auth.authenticate(r.Context(), token)
		if err != nil {
			if opts.Logger != nil {
				opts.Logger.Error("TokenReview verification error", "error", err)
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-controlplane-admin"`)
			http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
			return
		}
		if !result.authenticated {
			deny(w)
			return
		}

		// Reads require only authentication; writes additionally require
		// authorization, which fails closed when no allowlist is configured.
		isWrite := r.Method != http.MethodGet && r.Method != http.MethodHead
		if isWrite && !auth.authorizeWrite(result) {
			if opts.Logger != nil {
				opts.Logger.Warn("authenticated identity not authorized for write",
					"user", result.username, "groups", result.groups)
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), identityKey, &Identity{
			Username: result.username,
			Groups:   result.groups,
			Subject:  result.username,
		}))
		next.ServeHTTP(w, r)
	})
}

type authResult struct {
	authenticated bool
	username      string
	groups        []string
	cachedAt      time.Time
}

type tokenReviewAuthenticator struct {
	clientset kubernetes.Interface
	audiences []string
	users     map[string]struct{}
	groups    map[string]struct{}
	logger    *slog.Logger
	cacheTTL  time.Duration

	mu    sync.RWMutex
	cache map[string]authResult
}

func (a *tokenReviewAuthenticator) authenticate(ctx context.Context, token string) (authResult, error) {
	if a.clientset == nil {
		return authResult{}, fmt.Errorf("kubernetes clientset unavailable for TokenReview")
	}

	key := tokenHash(token)

	a.mu.RLock()
	if cached, ok := a.cache[key]; ok && time.Since(cached.cachedAt) < a.cacheTTL {
		a.mu.RUnlock()
		return cached, nil
	}
	a.mu.RUnlock()

	tr := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     token,
			Audiences: a.audiences,
		},
	}

	result, err := a.clientset.AuthenticationV1().TokenReviews().Create(
		ctx, tr, metav1.CreateOptions{},
	)
	if err != nil {
		return authResult{}, fmt.Errorf("TokenReview request failed: %w", err)
	}

	ar := authResult{
		authenticated: result.Status.Authenticated,
		username:      result.Status.User.Username,
		groups:        result.Status.User.Groups,
		cachedAt:      time.Now(),
	}

	if a.logger != nil {
		if ar.authenticated {
			a.logger.Debug("token authenticated via TokenReview",
				"user", ar.username, "groups", ar.groups)
		} else {
			a.logger.Warn("token not authenticated by TokenReview",
				"error", result.Status.Error)
		}
	}

	a.mu.Lock()
	if a.cache == nil {
		a.cache = make(map[string]authResult)
	}
	a.cache[key] = ar
	a.mu.Unlock()

	return ar, nil
}

// authorizeWrite reports whether an authenticated identity may perform writes.
// It fails closed: with no allowlist configured, writes are denied so that
// authMode=kubernetes is not wide-open by default. Operators opt specific users
// or groups in via adminAuth.allowedUsers / adminAuth.allowedGroups.
func (a *tokenReviewAuthenticator) authorizeWrite(ar authResult) bool {
	if len(a.users) == 0 && len(a.groups) == 0 {
		return false
	}
	if _, ok := a.users[ar.username]; ok {
		return true
	}
	for _, g := range ar.groups {
		if _, ok := a.groups[g]; ok {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	s := make(map[string]struct{}, len(items))
	for _, v := range items {
		s[v] = struct{}{}
	}
	return s
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// staticAuthHandler validates tokens against pre-configured static bearer tokens.
func staticAuthHandler(next http.Handler, opts Options) http.Handler {
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

		r = r.WithContext(context.WithValue(r.Context(), identityKey, &Identity{
			Subject: "static-token",
		}))
		next.ServeHTTP(w, r)
	})
}

func deny(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="nantian-controlplane-admin"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func IsAuthConfigured(opts Options) bool {
	return strings.TrimSpace(opts.BearerTokenFile) != "" || strings.TrimSpace(opts.BearerToken) != ""
}

func resolveBearerToken(opts Options) (string, bool) {
	if path := strings.TrimSpace(opts.BearerTokenFile); path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: file path is from trusted configuration (BearerTokenFile)
		if err != nil {
			return "", false
		}
		token := strings.TrimSpace(string(raw))
		return token, token != ""
	}

	token := strings.TrimSpace(opts.BearerToken)
	return token, token != ""
}

// IsTokenValid checks if a given Bearer token matches the configured token.
// Always returns false if auth is not configured (no token set).
func (opts Options) IsTokenValid(token string) bool {
	expected, ok := resolveBearerToken(opts)
	if !ok || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// extractBearerToken pulls the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func resolveReadOnlyToken(opts Options) (string, bool) {
	if path := strings.TrimSpace(opts.ReadOnlyBearerTokenFile); path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: file path is from trusted configuration (ReadOnlyBearerTokenFile)
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
	return path == constants.PathLivez || path == constants.PathReadyz
}
