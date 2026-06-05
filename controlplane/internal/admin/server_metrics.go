package admin

import (
	"encoding/json"
	"net/http"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/admin/metrics"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	metricsConfigNamespace = "aether-gateway"
)

type metricsConfigRequest struct {
	PrometheusURL string `json:"prometheusUrl"`
}

type metricsQueryRequest struct {
	Query  string `json:"query"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
	Step   string `json:"step,omitempty"`
}

// handleMetricsConfigGet reads the metrics configuration and returns it.
func (s *Server) handleMetricsConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := metrics.LoadConfig(r.Context(), s.resources.client, metricsConfigNamespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.respondJSON(w, map[string]any{
				"prometheusUrl": "",
			})
			return
		}
		s.logger.Error("failed to load metrics config", "error", err)
		s.respondRequestError(w, err)
		return
	}

	s.respondJSON(w, map[string]any{
		"prometheusUrl": cfg.PrometheusURL,
	})
}

// handleMetricsConfigPut saves the metrics configuration.
func (s *Server) handleMetricsConfigPut(w http.ResponseWriter, r *http.Request) {
	var req metricsConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondRequestError(w, errInvalidRequest("invalid request body: "+err.Error()))
		return
	}

	ctx := r.Context()

	cfg := &metrics.MetricsConfig{PrometheusURL: req.PrometheusURL}
	if err := metrics.SaveConfig(ctx, s.resources.client, metricsConfigNamespace, cfg); err != nil {
		s.logger.Error("failed to save metrics config", "error", err)
		s.respondRequestError(w, err)
		return
	}

	s.respondJSON(w, map[string]any{
		"prometheusUrl": cfg.PrometheusURL,
	})
}

// handleMetricsQuery executes a PromQL query against the configured
// Prometheus instance and returns the raw Prometheus API response.
func (s *Server) handleMetricsQuery(w http.ResponseWriter, r *http.Request) {
	var req metricsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondRequestError(w, errInvalidRequest("invalid request body: "+err.Error()))
		return
	}
	if req.Query == "" {
		s.respondRequestError(w, errInvalidRequest("query is required"))
		return
	}

	cfg, err := metrics.LoadConfig(r.Context(), s.resources.client, metricsConfigNamespace)
	if err != nil || cfg.PrometheusURL == "" {
		s.respondJSON(w, emptyPrometheusResponse())
		return
	}

	client := metrics.NewPrometheusClient(cfg.PrometheusURL)
	result, err := client.InstantQuery(r.Context(), req.Query)
	if err != nil {
		s.logger.Error("prometheus query failed", "error", err, "query", req.Query)
		s.respondRequestError(w, errServiceUnavailable("prometheus query failed: "+err.Error()))
		return
	}

	s.respondJSON(w, result)
}

// handleMetricsRangeQuery executes a PromQL range query against the configured
// Prometheus instance and returns the raw Prometheus API response.
func (s *Server) handleMetricsRangeQuery(w http.ResponseWriter, r *http.Request) {
	var req metricsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondRequestError(w, errInvalidRequest("invalid request body: "+err.Error()))
		return
	}
	if req.Query == "" {
		s.respondRequestError(w, errInvalidRequest("query is required"))
		return
	}
	if req.Start == "" || req.End == "" || req.Step == "" {
		s.respondRequestError(w, errInvalidRequest("start, end, and step are required for range queries"))
		return
	}

	cfg, err := metrics.LoadConfig(r.Context(), s.resources.client, metricsConfigNamespace)
	if err != nil || cfg.PrometheusURL == "" {
		s.respondJSON(w, emptyPrometheusResponse())
		return
	}

	client := metrics.NewPrometheusClient(cfg.PrometheusURL)
	result, err := client.RangeQuery(r.Context(), req.Query, req.Start, req.End, req.Step)
	if err != nil {
		s.logger.Error("prometheus range query failed", "error", err, "query", req.Query)
		s.respondRequestError(w, errServiceUnavailable("prometheus range query failed: "+err.Error()))
		return
	}

	s.respondJSON(w, result)
}

// emptyPrometheusResponse returns a Prometheus-style success response with
// an empty result vector. Used when metrics are not configured.
func emptyPrometheusResponse() map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     []any{},
		},
	}
}