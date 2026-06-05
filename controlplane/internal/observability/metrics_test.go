package observability

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestHandlerIncludesControllerRuntimeRegistryMetrics(t *testing.T) {
	metrics := NewMetrics()
	metrics.BuildsTotal.Inc()

	metricName := fmt.Sprintf(
		"test_controller_runtime_registry_metric_%d",
		time.Now().UnixNano(),
	)
	testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricName,
		Help: "test controller-runtime registry metric",
	})
	if err := ctrlmetrics.Registry.Register(testGauge); err != nil {
		t.Fatalf("register controller-runtime metric: %v", err)
	}
	defer ctrlmetrics.Registry.Unregister(testGauge)
	testGauge.Set(1)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler(metrics).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "aether_gateway_snapshot_builds_total") {
		t.Fatalf("expected custom snapshot metrics in response, body=%q", body)
	}
	if !strings.Contains(body, metricName) {
		t.Fatalf("expected controller-runtime registry metric %q in response", metricName)
	}
}

func TestHandlerAvoidsDuplicateMetricFamiliesFromDefaultGatherer(t *testing.T) {
	metrics := NewMetrics()

	dupName := "go_threads"
	dupGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: dupName,
		Help: "duplicate go collector metric name for regression coverage",
	})
	if err := ctrlmetrics.Registry.Register(dupGauge); err != nil {
		t.Fatalf("register duplicate controller-runtime metric: %v", err)
	}
	defer ctrlmetrics.Registry.Unregister(dupGauge)
	dupGauge.Set(7)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler(metrics).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "\n"+dupName+" ") {
		t.Fatalf("expected duplicate-name metric %q in response", dupName)
	}
}

func TestHandlerIncludesGoAndProcessCollectors(t *testing.T) {
	metrics := NewMetrics()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler(metrics).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	for _, metricName := range []string{
		"go_threads",
		"process_cpu_seconds_total",
	} {
		if !strings.Contains(body, "\n"+metricName+" ") {
			t.Fatalf("expected runtime metric %q in response, body=%q", metricName, body)
		}
	}
}

func TestHandlerExposesCustomMetricValuesAndPrometheusContentType(t *testing.T) {
	metrics := NewMetrics()
	metrics.BuildsTotal.Inc()
	metrics.LastBuildSuccess.Set(1)
	metrics.AdminAPIRequestsTotal.WithLabelValues(http.MethodGet, "summary", "2xx").Add(2)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler(metrics).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain; version=0.0.4") {
		t.Fatalf("expected Prometheus content type, got %q", contentType)
	}

	body := recorder.Body.String()
	for _, sample := range []string{
		"\naether_gateway_snapshot_builds_total 1",
		"\naether_gateway_snapshot_last_build_success 1",
		"\naether_gateway_controlplane_admin_requests_total{method=\"GET\",route=\"summary\",status_class=\"2xx\"} 2",
	} {
		if !strings.Contains(body, sample) {
			t.Fatalf("expected metric sample %q in response body", sample)
		}
	}
}

func TestHandlerLimitsConcurrentMetricScrapes(t *testing.T) {
	gatherer := &blockingGatherer{
		entered: make(chan struct{}, defaultMetricsHandlerMaxRequestsInFlight),
		release: make(chan struct{}),
	}
	handler := metricsHandlerForGatherer(gatherer)

	var wg sync.WaitGroup
	errs := make(chan string, defaultMetricsHandlerMaxRequestsInFlight)
	for range defaultMetricsHandlerMaxRequestsInFlight {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				errs <- fmt.Sprintf("blocked scrape status = %d, want %d", recorder.Code, http.StatusOK)
			}
		}()
	}

	for range defaultMetricsHandlerMaxRequestsInFlight {
		select {
		case <-gatherer.entered:
		case <-time.After(time.Second):
			close(gatherer.release)
			t.Fatal("timed out waiting for in-flight metric scrapes")
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(recorder, request)

	close(gatherer.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("overflow scrape status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestHandlerAppliesScrapeTimeout(t *testing.T) {
	opts := metricsHandlerOptions()
	if opts.Timeout != defaultMetricsHandlerTimeout {
		t.Fatalf("metrics handler timeout = %s, want %s", opts.Timeout, defaultMetricsHandlerTimeout)
	}
}

type blockingGatherer struct {
	entered chan struct{}
	release chan struct{}
}

func (g *blockingGatherer) Gather() ([]*dto.MetricFamily, error) {
	g.entered <- struct{}{}
	<-g.release
	return nil, nil
}
