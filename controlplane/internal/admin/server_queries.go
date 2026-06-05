package admin

import (
	"net/http"
	"strings"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func (s *Server) handleListeners(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		s.respondJSON(w, []ir.Listener{})
		return
	}

	items, err := filterListeners(displayListeners(snapshot.Listeners), r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	s.respondJSON(w, items)
}

func (s *Server) handleListenerDetail(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		s.respondNotFound(w, "listener not found")
		return
	}

	item, ok := s.snapshotDetailIndex(snapshot).listener(strings.TrimSpace(r.PathValue("name")))
	if !ok {
		s.respondNotFound(w, "listener not found")
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	items, err := filterRoutes(s.store.Current(), r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	s.respondJSON(w, items)
}

func (s *Server) handleRouteDetail(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	item, ok, err := s.snapshotDetailIndex(snapshot).route(
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	if err != nil {
		s.respondQueryError(w, err)
		return
	}
	if !ok {
		s.respondNotFound(w, "route not found")
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleBackends(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		s.respondJSON(w, []ir.BackendCluster{})
		return
	}

	items, err := filterBackends(snapshot, r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	s.respondJSON(w, items)
}

func (s *Server) handleBackendDetail(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		s.respondNotFound(w, "backend not found")
		return
	}

	item, ok := s.snapshotDetailIndex(snapshot).backend(r.PathValue("namespace"), r.PathValue("name"))
	if !ok {
		s.respondNotFound(w, "backend not found")
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	items, err := filterNodes(s.currentNodes(r.Context(), snapshot), r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	s.respondJSON(w, items)
}

func (s *Server) handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	item, ok := findNode(s.currentNodes(r.Context(), s.store.Current()), strings.TrimSpace(r.PathValue("nodeId")))
	if !ok {
		s.respondNotFound(w, "node not found")
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	filter, err := parseTopologyFilter(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.respondJSON(w, filterTopology(buildTopology(snapshot, s.currentNodes(r.Context(), snapshot)), filter))
}
