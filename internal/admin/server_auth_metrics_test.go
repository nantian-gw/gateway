package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/nantian-gw/gateway/internal/observability"
)

func TestAdminAuthProtectsManagementEndpoints(t *testing.T) {
	server := newTestServerWithOptions(t, Options{BearerToken: "top-secret"})

	recorder := performRequest(t, server, http.MethodGet, "/v1/summary", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", recorder.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer top-secret")
	recorder = httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", recorder.Code)
	}
}

func TestAdminServerRecordsSuccessRequestMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	server := newTestServerWithOptions(t, Options{Metrics: metrics})

	recorder := performRequest(t, server, http.MethodGet, "/v1/summary", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "summary", "2xx")); got != 1 {
		t.Fatalf("admin request total = %v, want 1", got)
	}
	if got := histogramVecSampleCount(t, metrics.AdminAPIRequestDurationSeconds, http.MethodGet, "summary", "2xx"); got != 1 {
		t.Fatalf("admin request duration sample count = %d, want 1", got)
	}
}

func TestAdminServerRecordsUnauthorizedRequestMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	server := newTestServerWithOptions(t, Options{
		BearerToken: "top-secret",
		Metrics:     metrics,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	recorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", recorder.Code)
	}

	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "summary", "4xx")); got != 1 {
		t.Fatalf("admin unauthorized request total = %v, want 1", got)
	}
	if got := histogramVecSampleCount(t, metrics.AdminAPIRequestDurationSeconds, http.MethodGet, "summary", "4xx"); got != 1 {
		t.Fatalf("admin request duration sample count = %d", got)
	}
}

func TestAdminMetricsRecordsFirstWrittenStatusCode(t *testing.T) {
	metrics := observability.NewMetrics()
	handler := wrapMetricsHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		http.Error(w, "late error after headers", http.StatusInternalServerError)
	}), metrics)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unmatched", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "unknown", "2xx")); got != 1 {
		t.Fatalf("admin request total for first status class = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "unknown", "5xx")); got != 0 {
		t.Fatalf("admin request total for overwritten status class = %v, want 0", got)
	}
}

func TestAdminMetricsNormalizesUnknownHTTPMethods(t *testing.T) {
	metrics := observability.NewMetrics()
	handler := wrapMetricsHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), metrics)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("BREW", "/v1/summary", nil))

	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues("OTHER", "summary", "2xx")); got != 1 {
		t.Fatalf("admin request total for normalized method = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues("BREW", "summary", "2xx")); got != 0 {
		t.Fatalf("admin request total for raw unknown method = %v, want 0", got)
	}
	if got := histogramVecSampleCount(t, metrics.AdminAPIRequestDurationSeconds, "OTHER", "summary", "2xx"); got != 1 {
		t.Fatalf("admin request duration samples for normalized method = %d, want 1", got)
	}
}

func TestAdminServerRejectsOversizedJSONResponse(t *testing.T) {
	t.Parallel()

	server := newTestServerWithOptions(t, Options{MaxResponseBodyBytes: 1})

	recorder := performRequest(t, server, http.MethodGet, "/v1/summary", nil)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "response exceeds admin response size limit") {
		t.Fatalf("expected oversized response explanation, got %q", recorder.Body.String())
	}
}

func TestAdminAuthSkipsProbeEndpoints(t *testing.T) {
	server := newTestServerWithOptions(t, Options{BearerToken: "top-secret"})

	recorder := performRequest(t, server, http.MethodGet, "/livez", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected livez to bypass auth, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/readyz", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected readyz to bypass auth, got %d", recorder.Code)
	}
}

func TestAdminRateLimiterUsesBurstAndRecords429Metrics(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	server := newTestServerWithOptions(t, Options{
		RateLimitRPS:   1,
		RateLimitBurst: 1,
		Metrics:        metrics,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	req.RemoteAddr = "203.0.113.10:12345"
	recorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", recorder.Code)
	}

	if got := testutil.ToFloat64(metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "summary", "4xx")); got != 1 {
		t.Fatalf("unexpected 4xx admin request metric total: %v", got)
	}
}

func TestAdminRateLimiterBucketsByClientIPWithoutPort(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(1, 1)
	now := time.Unix(1, 0).UTC()
	rl.now = func() time.Time { return now }

	if !rl.allow("203.0.113.10:1000") {
		t.Fatal("expected initial request to pass")
	}
	if rl.allow("203.0.113.10:2000") {
		t.Fatal("expected same client IP with different port to share the same bucket")
	}

	now = now.Add(time.Second)
	if !rl.allow("203.0.113.10:3000") {
		t.Fatal("expected bucket refill after one second")
	}
}

func TestAdminTracingMiddlewareCreatesRequestSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	handler := wrapTracingHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), "summary")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/summary", nil))

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("unexpected span count: %d", len(spans))
	}
	if spans[0].Name != "admin GET /v1/summary" {
		t.Fatalf("unexpected span name: %q", spans[0].Name)
	}
}

func TestReadinessCurrentSnapshotAnyRequiresReadyCurrentNode(t *testing.T) {
	server := newTestServerWithOptions(t, Options{ReadinessMode: readinessModeCurrentSnapshotAny})

	now := time.Now().UTC()
	server.nodes.ObserveReport(context.Background(), "dp-1", "", false, "warming", now)

	recorder := performRequest(t, server, http.MethodGet, "/readyz", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected strict any readiness to fail without a ready current node, got %d", recorder.Code)
	}

	server.nodes.ObserveReport(context.Background(), "dp-1", server.store.Current().ID, true, "ready", now)
	recorder = performRequest(t, server, http.MethodGet, "/readyz", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected strict any readiness to recover, got %d", recorder.Code)
	}
}

func TestReadinessCurrentSnapshotAllRequiresAllConnectedNodesOnCurrentVersion(t *testing.T) {
	server := newTestServerWithOptions(t, Options{ReadinessMode: readinessModeCurrentSnapshotAll})

	now := time.Now().UTC()
	server.nodes.Connect(context.Background(), "dp-2", "kind", []string{"routes"}, now)
	server.nodes.ObserveReport(context.Background(), "dp-2", "legacy", true, "warming", now)

	recorder := performRequest(t, server, http.MethodGet, "/readyz", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected strict all readiness to fail while a connected node is behind, got %d", recorder.Code)
	}

	server.nodes.ObserveReport(context.Background(), "dp-2", server.store.Current().ID, true, "ready", now)
	recorder = performRequest(t, server, http.MethodGet, "/readyz", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected strict all readiness to recover, got %d", recorder.Code)
	}
}

func TestAdminAuthReloadsBearerTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin-token")
	if err := os.WriteFile(path, []byte("old-secret\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	server := newTestServerWithOptions(t, Options{BearerTokenFile: path})

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer old-secret")
	recorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 with initial token, got %d", recorder.Code)
	}

	if err := os.WriteFile(path, []byte("new-secret\n"), 0o600); err != nil {
		t.Fatalf("rewrite token: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer old-secret")
	recorder = httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with stale token after rotation, got %d", recorder.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.Header.Set("Authorization", "Bearer new-secret")
	recorder = httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 with rotated token, got %d", recorder.Code)
	}
}
