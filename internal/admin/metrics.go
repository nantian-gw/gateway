package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/nantian-gw/gateway/internal/observability"
	"github.com/nantian-gw/gateway/internal/constants"
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

var routeClassification = map[string]string{
	constants.PathLivez:                    "livez",
	constants.PathReadyz:                    "readyz",
	"/v1/summary":                "summary",
	"/v1/snapshot-sync":          "snapshot_sync",
	"/v1/snapshot":               "snapshot",
	"/v1/listeners":              "listeners",
	"/v1/routes":                 "routes",
	"/v1/backends":               "backends",
	"/v1/nodes":                  "nodes",
	"/v1/infrastructure":         "infrastructure",
	"/v1/service-catalog":        "service_catalog",
	"/v1/resource-kinds":         "resource_kinds",
	"/v1/resources":              "resources",
	"/v1/topology":               "topology",
	"/v1/namespaces":             "namespaces",
	"/v1/dashboard/capabilities": "dashboard_capabilities",
	"/v1/auth/verify":            "auth_verify",
	"/v1/dataplanes":             "dataplanes",
	"/v1/chatbot/config":         "chatbot_config",
	"/v1/chatbot/chat":           "chatbot_chat",
	"/v1/metrics/config":         "metrics_config",
	"/v1/metrics/query":          "metrics_query",
	"/v1/metrics/query_range":    "metrics_query_range",
	"/v1/ai/overview":            "ai_overview",
	"/v1/ai/services":            "ai_services",
	"/v1/ai/token-usage":         "ai_token_usage",
	"/v1/ai/traces":              "ai_traces",
	"/v1/ai/cost":                "ai_cost",
}

var prefixRouteClassification = []struct {
	prefix string
	class  string
}{
	{"/v1/listeners/", "listener_detail"},
	{"/v1/routes/",    "route_detail"},
	{"/v1/backends/",  "backend_detail"},
	{"/v1/nodes/",     "node_detail"},
	{"/v1/dataplanes/", "dataplane_summary"},
	{"/v1/resources/", "resource_detail"},
}

func classifyAdminRoute(path string) string {
	if class, ok := routeClassification[path]; ok {
		return class
	}
	for _, entry := range prefixRouteClassification {
		if strings.HasPrefix(path, entry.prefix) {
			return entry.class
		}
	}
	return "unknown"
}
