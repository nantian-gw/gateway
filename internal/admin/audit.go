package admin

import (
	"log/slog"
	"net/http"
	"time"
)

// wrapAuditHandler logs an audit event for every admin API request.
// It must be placed after authentication middleware so that caller
// identity is available in the request context via IdentityFromContext.
func wrapAuditHandler(next http.Handler, opts Options) http.Handler {
	if next == nil {
		return nil
	}
	auditLogger := opts.Logger
	if auditLogger != nil {
		auditLogger = auditLogger.With(slog.String("log_type", "audit"))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecordingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		if auditLogger == nil {
			return
		}

		status := recorder.statusCode()
		route := classifyAdminRoute(r.URL.Path)
		id := IdentityFromContext(r.Context())
		who := "anonymous"
		if id != nil {
			who = id.Subject
		}

		attrs := []any{
			slog.String("who", who),
			slog.String("method", normalizeAdminMethod(r.Method)),
			slog.String("path", r.URL.Path),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
			slog.String("remote_addr", r.RemoteAddr),
		}

		switch {
		case status >= 500:
			auditLogger.Error("admin api request", attrs...)
		case status >= 400:
			auditLogger.Warn("admin api request", attrs...)
		default:
			auditLogger.Info("admin api request", attrs...)
		}
	})
}
