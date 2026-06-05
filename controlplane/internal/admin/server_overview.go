package admin

import (
	"net/http"
	"time"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		http.Error(w, "snapshot not ready", http.StatusServiceUnavailable)
		return
	}
	if !requiresCurrentSnapshotReadiness(s.readinessMode) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}

	nodes := s.currentNodes(r.Context(), snapshot)
	sync := summarizeNodeSync(snapshot, nodes, s.driftWarningThreshold, s.now())
	switch s.readinessMode {
	case readinessModeCurrentSnapshotAny:
		if sync.currentVersionReadyCount == 0 {
			http.Error(w, "no ready dataplane has acknowledged the current snapshot", http.StatusServiceUnavailable)
			return
		}
	case readinessModeCurrentSnapshotAll:
		if sync.connectedNodeCount == 0 {
			http.Error(w, "no connected dataplane nodes", http.StatusServiceUnavailable)
			return
		}
		if sync.currentVersionReadyCount != sync.connectedNodeCount {
			http.Error(w, "not all connected dataplane nodes have acknowledged the current snapshot", http.StatusServiceUnavailable)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	s.respondJSON(w, s.store.Current())
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	s.respondJSON(w, buildSummary(snapshot, s.currentNodes(r.Context(), snapshot), s.driftWarningThreshold, s.now()))
}

func (s *Server) handleSnapshotSync(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	s.respondJSON(w, buildSnapshotSync(
		snapshot,
		s.currentNodes(r.Context(), snapshot),
		s.readinessMode,
		s.driftWarningThreshold,
		s.now(),
	))
}

func buildSummary(snapshot *ir.Snapshot, nodes []ir.NodeStatus, driftWarningThreshold time.Duration, now time.Time) Summary {
	summary := Summary{
		NodeCount: len(nodes),
	}

	for _, node := range nodes {
		if node.Connected {
			summary.ConnectedNodeCount++
		}
		if node.Ready {
			summary.ReadyNodeCount++
		}
	}

	if snapshot == nil {
		return summary
	}
	sync := summarizeNodeSync(snapshot, nodes, driftWarningThreshold, now)

	summary.SnapshotVersion = snapshot.ID
	summary.GeneratedAt = snapshot.GeneratedAt
	summary.ListenerCount = len(snapshot.Listeners)
	for _, l := range snapshot.Listeners {
		switch {
		case l.Status == nil || l.Status.Programmed == nil:
			summary.WarningListenerCount++
		case l.Status.Programmed.Status == "True":
			summary.ReadyListenerCount++
		case l.Status.Programmed.Status == "Unknown":
			summary.WarningListenerCount++
		case l.Status.Programmed.Status == "False":
			summary.FailedListenerCount++
		default:
			summary.WarningListenerCount++
		}
	}
	summary.HTTPRouteCount = len(snapshot.HTTPRoutes)
	summary.GRPCRouteCount = len(snapshot.GRPCRoutes)
	summary.StreamRouteCount = len(snapshot.StreamRoutes)
	summary.RouteCount = summary.HTTPRouteCount + summary.GRPCRouteCount + summary.StreamRouteCount
	summary.BackendCount = referencedBackendCount(snapshot)
	summary.SecretCount = len(snapshot.Secrets)
	summary.CurrentVersionNodeCount = sync.currentVersionNodeCount
	summary.CurrentVersionReadyCount = sync.currentVersionReadyCount
	summary.DriftedNodeCount = sync.driftedNodeCount
	summary.Warnings = sync.warnings

	return summary
}
