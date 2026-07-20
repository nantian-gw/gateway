package admin

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

func (s *Server) handleInfrastructure(w http.ResponseWriter, r *http.Request) {
	if s.infra == nil {
		http.Error(w, "infrastructure inspector not configured", http.StatusNotImplemented)
		return
	}

	filter, err := parseInfrastructureQueryFilter(r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	report, err := s.infra.Inspect(r.Context())
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

	items, meta := filterInfrastructureReport(report, filter, s.maxListItems)
	writePaginationHeaders(w.Header(), meta)
	s.respondJSON(w, items)
}

func (s *Server) handleServiceCatalog(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	filter, err := parseServiceCatalogFilter(r.URL.Query())
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	items, meta, err := s.resources.ListServiceCatalog(r.Context(), filter, s.maxListItems)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

	writePaginationHeaders(w.Header(), meta)
	s.respondJSON(w, items)
}

func (s *Server) handleResourceKinds(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		s.respondJSON(w, SupportedResourceKinds())
		return
	}

	s.respondJSON(w, s.resources.DescribeKinds(r.Context()))
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	limit, err := parseOptionalPositiveInt(r.URL.Query().Get("limit"), "limit")
	if err != nil {
		s.respondQueryError(w, err)
		return
	}
	offset, err := parseOptionalNonNegativeInt(r.URL.Query().Get("offset"), "offset")
	if err != nil {
		s.respondQueryError(w, err)
		return
	}

	filter := ResourceListFilter{
		Kind:      strings.TrimSpace(r.URL.Query().Get("kind")),
		Namespace: strings.TrimSpace(r.URL.Query().Get("namespace")),
		Name:      strings.TrimSpace(r.URL.Query().Get("name")),
	}
	if limit != nil {
		filter.Limit = *limit
		filter.HasLimit = true
	}
	if offset != nil {
		filter.Offset = *offset
	}

	items, meta, err := s.resources.List(r.Context(), filter, s.maxListItems)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

	writePaginationHeaders(w.Header(), meta)
	s.respondJSON(w, items)
}

func (s *Server) handleResourceDetail(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	item, ok, err := s.resources.Get(
		r.Context(),
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}
	if !ok {
		s.respondNotFound(w, "resource not found")
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleResourceApply(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			s.respondRequestError(w, errPayloadTooLarge("request body exceeds admin request size limit"))
			return
		}

		s.respondRequestError(w, errInvalidRequest("failed to read request body"))
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		s.respondRequestError(w, errInvalidRequest("request body is required"))
		return
	}

	item, err := s.resources.Apply(
		r.Context(),
		body,
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

	s.respondJSON(w, item)
}

func (s *Server) handleResourceDelete(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	deleted, err := s.resources.Delete(
		r.Context(),
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}
	if !deleted {
		s.respondNotFound(w, "resource not found")
		return
	}

	s.respondJSON(w, map[string]any{"deleted": true})
}

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	namespaces, err := s.resources.ListNamespaces(r.Context())
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

	s.respondJSON(w, namespaces)
}
