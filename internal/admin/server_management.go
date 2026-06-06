package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	k8syaml "sigs.k8s.io/yaml"
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

	s.respondJSON(w, filterInfrastructureReport(report, filter))
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

	items, err := s.resources.ListServiceCatalog(r.Context(), filter)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

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

	items, err := s.resources.List(r.Context(), filter)
	if err != nil {
		s.respondRequestError(w, err)
		return
	}

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

	operation := resourceMutationOperation(r.Method)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			limitErr := errPayloadTooLarge("request body exceeds admin request size limit")
			s.logResourceMutationFailure(operation, r, nil, limitErr)
			s.respondRequestError(w, limitErr)
			return
		}

		invalidErr := errInvalidRequest("failed to read request body")
		s.logResourceMutationFailure(operation, r, nil, invalidErr)
		s.respondRequestError(w, invalidErr)
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		emptyBodyErr := errInvalidRequest("request body is required")
		s.logResourceMutationFailure(operation, r, body, emptyBodyErr)
		s.respondRequestError(w, emptyBodyErr)
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
		s.logResourceMutationFailure(operation, r, body, err)
		s.respondRequestError(w, err)
		return
	}

	s.logResourceMutationSuccess(operation, item.Kind, item.Namespace, item.Name)
	s.respondJSON(w, item)
}

func (s *Server) handleResourceDelete(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		http.Error(w, "resource manager not configured", http.StatusNotImplemented)
		return
	}

	operation := resourceMutationOperation(r.Method)
	deleted, err := s.resources.Delete(
		r.Context(),
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	if err != nil {
		s.logResourceMutationFailure(operation, r, nil, err)
		s.respondRequestError(w, err)
		return
	}
	if !deleted {
		s.logResourceMutationFailure(operation, r, nil, errors.New("resource not found"))
		s.respondNotFound(w, "resource not found")
		return
	}

	s.logResourceMutationSuccess(
		operation,
		r.PathValue("kind"),
		r.PathValue("namespace"),
		r.PathValue("name"),
	)
	s.respondJSON(w, map[string]any{"deleted": true})
}

func resourceMutationOperation(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(strings.TrimSpace(method))
	}
}

func (s *Server) logResourceMutationSuccess(operation, kind, namespace, name string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(
		"admin resource mutation completed",
		"operation",
		operation,
		"kind",
		strings.TrimSpace(kind),
		"namespace",
		strings.TrimSpace(namespace),
		"name",
		strings.TrimSpace(name),
	)
}

func (s *Server) logResourceMutationFailure(operation string, r *http.Request, body []byte, err error) {
	if s == nil || s.logger == nil {
		return
	}
	kind, namespace, name := resourceMutationIdentity(r, body)
	s.logger.Warn(
		"admin resource mutation failed",
		"operation",
		operation,
		"kind",
		kind,
		"namespace",
		namespace,
		"name",
		name,
		"error",
		err,
	)
}

func resourceMutationIdentity(r *http.Request, body []byte) (string, string, string) {
	kind := ""
	namespace := ""
	name := ""
	if r != nil {
		kind = strings.TrimSpace(r.PathValue("kind"))
		namespace = strings.TrimSpace(r.PathValue("namespace"))
		name = strings.TrimSpace(r.PathValue("name"))
	}
	if kind != "" && name != "" {
		return kind, namespace, name
	}

	var envelope struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"metadata"`
	}
	jsonBody, err := k8syaml.YAMLToJSON(body)
	if err != nil {
		return kind, namespace, name
	}
	if err := json.Unmarshal(jsonBody, &envelope); err != nil {
		return kind, namespace, name
	}
	if kind == "" {
		kind = strings.TrimSpace(envelope.Kind)
	}
	if namespace == "" {
		namespace = strings.TrimSpace(envelope.Metadata.Namespace)
	}
	if name == "" {
		name = strings.TrimSpace(envelope.Metadata.Name)
	}
	return kind, namespace, name
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
