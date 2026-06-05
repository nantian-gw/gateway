package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/nantian-gw/gateway/controlplane/internal/observability"
)

func wrapMetricsHandler(next http.Handler, metrics *observability.Metrics) http.Handler {
	if next == nil || metrics == nil || metrics.AdminAPIRequestsTotal == nil || metrics.AdminAPIRequestDurationSeconds == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecordingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		method := normalizeAdminMethod(r.Method)
		statusClass := responseStatusClass(recorder.statusCode())
		route := classifyAdminRoute(r.URL.Path)
		metrics.AdminAPIRequestsTotal.WithLabelValues(method, route, statusClass).Inc()
		metrics.AdminAPIRequestDurationSeconds.WithLabelValues(method, route, statusClass).Observe(time.Since(started).Seconds())
	})
}

type statusRecordingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusRecordingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecordingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusRecordingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusRecordingResponseWriter) statusCode() int {
	if w == nil || w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func responseStatusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

func normalizeAdminMethod(method string) string {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	switch normalized {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return normalized
	default:
		return "OTHER"
	}
}

func classifyAdminRoute(path string) string {
	switch {
	case path == "/livez":
		return "livez"
	case path == "/readyz":
		return "readyz"
	case path == "/v1/summary":
		return "summary"
	case path == "/v1/snapshot-sync":
		return "snapshot_sync"
	case path == "/v1/snapshot":
		return "snapshot"
	case path == "/v1/listeners":
		return "listeners"
	case strings.HasPrefix(path, "/v1/listeners/"):
		return "listener_detail"
	case path == "/v1/routes":
		return "routes"
	case strings.HasPrefix(path, "/v1/routes/"):
		return "route_detail"
	case path == "/v1/backends":
		return "backends"
	case strings.HasPrefix(path, "/v1/backends/"):
		return "backend_detail"
	case path == "/v1/nodes":
		return "nodes"
	case strings.HasPrefix(path, "/v1/nodes/"):
		return "node_detail"
	case path == "/v1/infrastructure":
		return "infrastructure"
	case path == "/v1/service-catalog":
		return "service_catalog"
	case path == "/v1/resource-kinds":
		return "resource_kinds"
	case path == "/v1/resources":
		return "resources"
	case strings.HasPrefix(path, "/v1/resources/"):
		return "resource_detail"
	case path == "/v1/topology":
		return "topology"
	default:
		return "unknown"
	}
}
