package admin

import (
	jsoniter "github.com/json-iterator/go"
	"log/slog"
	"net/http"
	"strings"
)

func (s *Server) handleDataplanes(w http.ResponseWriter, r *http.Request) {
	if s.dataplaneDiscovery == nil {
		http.Error(w, `{"error":"dataplane discovery not configured"}`, http.StatusServiceUnavailable)
		return
	}

	endpoints, err := s.dataplaneDiscovery.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to discover dataplanes: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := jsoniter.NewEncoder(w).Encode(map[string]any{
		"dataplanes": endpoints,
	}); err != nil {
		slog.Warn("failed to encode dataplanes response", "path", "handleDataplanes", "error", err)
	}
}

func (s *Server) handleDataplaneSummary(w http.ResponseWriter, r *http.Request) {
	if s.dataplaneDiscovery == nil || s.dataplaneClient == nil {
		http.Error(w, `{"error":"dataplane aggregation not configured"}`, http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/dataplanes/")
	nodeID := strings.TrimSuffix(path, "/summary")
	if nodeID == "" {
		http.Error(w, `{"error":"nodeId required"}`, http.StatusBadRequest)
		return
	}

	endpoints, err := s.dataplaneDiscovery.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to discover dataplanes: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	var targetEndpoint *DataplaneEndpoint
	for _, ep := range endpoints {
		if ep.NodeID == nodeID {
			targetEndpoint = &ep
			break
		}
	}

	if targetEndpoint == nil {
		http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
		return
	}

	var summary map[string]any
	if err := s.dataplaneClient.GetJSON(r.Context(), targetEndpoint.URL, "/v1/summary", &summary); err != nil {
		http.Error(w, `{"error":"failed to get summary: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := jsoniter.NewEncoder(w).Encode(summary); err != nil {
		slog.Warn("failed to encode dataplane summary response", "path", "handleDataplaneSummary", "error", err)
	}
}