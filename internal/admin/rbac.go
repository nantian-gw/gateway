package admin

import (
	"context"
	"log/slog"
	"net/http"
)

// routeContractContextKey is an unexported type for the route contract context key.
type routeContractContextKey struct{}

// routeContractKey stores the matched route contract in the request context.
var routeContractKey routeContractContextKey

// routeContractFromContext extracts the matched route contract from the request context.
// Returns nil when no route contract was stored (e.g., unmatched paths).
func routeContractFromContext(ctx context.Context) *routeContract {
	rc, _ := ctx.Value(routeContractKey).(*routeContract)
	return rc
}

// wrapRBACHandler enforces RBAC permissions on admin API requests.
// When RBAC is not configured or disabled, it passes through.
// The route contract (including Permission) must be stored in context
// by the route registration layer.
func wrapRBACHandler(next http.Handler, cfg *AdminRBACConfig, logger *slog.Logger) http.Handler {
	if cfg == nil || !cfg.IsEnabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorizeRBACRequest(w, r, cfg, logger, routeContractFromContext(r.Context())) {
			next.ServeHTTP(w, r)
			return
		}
	})
}

func authorizeRBACRequest(w http.ResponseWriter, r *http.Request, cfg *AdminRBACConfig, logger *slog.Logger, rc *routeContract) bool {
	if cfg == nil || !cfg.IsEnabled() || rc == nil || rc.Permission == "" {
		return true
	}

	identity := IdentityFromContext(r.Context())
	if identity == nil {
		identity = &Identity{}
	}
	roleName, allowed := cfg.Authorize(identity.Username, identity.Groups, rc.Permission)

	if allowed {
		if roleName != "" && logger != nil {
			logger.Debug("rbac allowed",
				"role", roleName,
				"permission", string(rc.Permission),
				"path", r.URL.Path,
				"method", r.Method,
			)
		}
		return true
	}

	if logger != nil {
		logger.Warn("rbac denied",
			"username", identity.Username,
			"groups", identity.Groups,
			"required_permission", string(rc.Permission),
			"path", r.URL.Path,
			"method", r.Method,
		)
	}

	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}
