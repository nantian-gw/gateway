package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestResourceEndpointsSupportCRUD(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &kinds); err != nil {
		t.Fatalf("decode resource kinds: %v", err)
	}
	if len(kinds) < 6 {
		t.Fatalf("expected resource kinds, got %+v", kinds)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources?kind=Gateway", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(resources) != 1 || resources[0].Kind != "Gateway" {
		t.Fatalf("unexpected gateway list: %+v", resources)
	}

	createBody := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: created
  namespace: default
spec:
  parentRefs:
    - name: edge
  rules:
    - backendRefs:
        - name: api
          port: 80
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", createBody, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created ManagedResource
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created resource: %v", err)
	}
	if created.Kind != "HTTPRoute" || created.Name != "created" {
		t.Fatalf("unexpected created resource: %+v", created)
	}

	updateBody := []byte(`{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "HTTPRoute",
  "metadata": {
    "name": "created",
    "namespace": "default",
    "labels": {
      "tier": "edge"
    }
  },
  "spec": {
    "parentRefs": [
      {"name": "edge"}
    ],
    "rules": [
      {
        "backendRefs": [
          {"name": "api", "port": 80}
        ]
      }
    ]
  }
}`)
	recorder = performRequestWithBody(t, server, http.MethodPut, "/v1/resources/httproute/default/created", updateBody, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var detail ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/httproute/default/created", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d", recorder.Code)
	}
	if detail.Labels["tier"] != "edge" {
		t.Fatalf("expected updated labels, got %+v", detail.Labels)
	}

	recorder = performRequest(t, server, http.MethodDelete, "/v1/resources/httproute/default/created", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/httproute/default/created", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", recorder.Code)
	}
}

func TestResourceMutationEndpointsEmitAuditLogs(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	server := newTestServerWithResourceManagerAndLogger(t, resourceManagerForTestWithLogger(t, logger), logger)

	createBody := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: created
  namespace: default
spec:
  rules:
    - backendRefs:
        - name: api
          port: 80
`)
	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/resources", createBody, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	logs := logBuf.String()
	if !strings.Contains(logs, "admin resource mutation completed") ||
		!strings.Contains(logs, "operation=create") ||
		!strings.Contains(logs, "kind=HTTPRoute") ||
		!strings.Contains(logs, "namespace=default") ||
		!strings.Contains(logs, "name=created") {
		t.Fatalf("expected create audit log, got %q", logs)
	}

	logBuf.Reset()
	recorder = performRequestWithBody(t, server, http.MethodPut, "/v1/resources/httproute/default/created", []byte(`{"kind":"Gateway"}`), nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid update 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	logs = logBuf.String()
	if !strings.Contains(logs, "admin resource mutation failed") ||
		!strings.Contains(logs, "operation=update") ||
		!strings.Contains(logs, "kind=httproute") ||
		!strings.Contains(logs, "namespace=default") ||
		!strings.Contains(logs, "name=created") {
		t.Fatalf("expected failed update audit log, got %q", logs)
	}
}

func TestResourceMutationRejectsOversizedRequestBody(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManagerAndOptions(
		t,
		resourceManagerForTest(t),
		Options{MaxRequestBodyBytes: 64},
	)

	body := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: created
  namespace: default
spec:
  rules:
    - backendRefs:
        - name: api
          port: 80
`)
	recorder := performRequestWithBody(t, server, http.MethodPost, "/v1/resources", body, nil)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "request body exceeds admin request size limit") {
		t.Fatalf("expected oversized request explanation, got %q", recorder.Body.String())
	}
}

func TestResourceEndpointSupportsStablePagination(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var resources []ManagedResource
	recorder := performRequest(t, server, http.MethodGet, "/v1/resources?limit=2&offset=1", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 paginated resources, got %+v", resources)
	}
	if resources[0].Kind != "GatewayClass" || resources[0].Name != "nantian-gw" {
		t.Fatalf("unexpected first paginated resource: %+v", resources[0])
	}
	if resources[1].Kind != "HTTPRoute" || resources[1].Namespace != "default" || resources[1].Name != "web" {
		t.Fatalf("unexpected second paginated resource: %+v", resources[1])
	}
}

func TestResourcesEmitPaginationHeaders(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManagerAndOptions(
		t,
		resourceManagerForTest(t),
		Options{MaxListItems: 2},
	)

	var resources []ManagedResource
	recorder := performRequest(t, server, http.MethodGet, "/v1/resources?limit=1&offset=1", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "1" {
		t.Fatalf("unexpected page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "3" {
		t.Fatalf("unexpected total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected has-next-page header: %q", got)
	}
}

func TestResourceListUsesDirectGetForExactNamespacedMatch(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	items, _, err := manager.List(context.Background(), ResourceListFilter{
		Kind:      "HTTPRoute",
		Namespace: "default",
		Name:      "web",
	}, 0)
	if err != nil {
		t.Fatalf("list exact namespaced resource: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "HTTPRoute" || items[0].Namespace != "default" || items[0].Name != "web" {
		t.Fatalf("unexpected exact namespaced result: %+v", items)
	}
	if got := counting.GetCalls(); got != 1 {
		t.Fatalf("get call count = %d, want 1", got)
	}
	if got := counting.ListCalls(); got != 0 {
		t.Fatalf("list call count = %d, want 0", got)
	}
}

func TestResourceListUsesDirectGetForExactClusterScopedMatch(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	items, _, err := manager.List(context.Background(), ResourceListFilter{
		Kind: "GatewayClass",
		Name: "nantian-gw",
	}, 0)
	if err != nil {
		t.Fatalf("list exact cluster-scoped resource: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "GatewayClass" || items[0].Name != "nantian-gw" {
		t.Fatalf("unexpected exact cluster-scoped result: %+v", items)
	}
	if got := counting.GetCalls(); got != 1 {
		t.Fatalf("get call count = %d, want 1", got)
	}
	if got := counting.ListCalls(); got != 0 {
		t.Fatalf("list call count = %d, want 0", got)
	}
}

func TestResourceListCachesRepeatedKindList(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	filter := ResourceListFilter{
		Kind:      "HTTPRoute",
		Namespace: "default",
		Limit:     25,
		HasLimit:  true,
	}

	for i := 0; i < 2; i++ {
		items, _, err := manager.List(context.Background(), filter, 0)
		if err != nil {
			t.Fatalf("list resources on iteration %d: %v", i, err)
		}
		if len(items) != 1 || items[0].Kind != "HTTPRoute" || items[0].Name != "web" {
			t.Fatalf("unexpected cached resource list on iteration %d: %+v", i, items)
		}
	}
	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("list call count = %d, want 1", got)
	}
}

func TestResourceListCacheExpires(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	now := time.Unix(1_700_000_000, 0)
	manager.listCache.ttl = time.Second
	manager.listCache.now = func() time.Time { return now }
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	filter := ResourceListFilter{Kind: "HTTPRoute", Namespace: "default"}
	if _, _, err := manager.List(context.Background(), filter, 0); err != nil {
		t.Fatalf("initial list resources: %v", err)
	}
	if _, _, err := manager.List(context.Background(), filter, 0); err != nil {
		t.Fatalf("cached list resources: %v", err)
	}
	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("list call count before expiry = %d, want 1", got)
	}

	now = now.Add(time.Second)
	if _, _, err := manager.List(context.Background(), filter, 0); err != nil {
		t.Fatalf("expired list resources: %v", err)
	}
	if got := counting.ListCalls(); got != 2 {
		t.Fatalf("list call count after expiry = %d, want 2", got)
	}
}

func TestResourceMutationInvalidatesListCache(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	manager.listCache.ttl = time.Minute
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	filter := ResourceListFilter{Kind: "Gateway", Namespace: "default"}
	items, _, err := manager.List(context.Background(), filter, 0)
	if err != nil {
		t.Fatalf("initial gateway list: %v", err)
	}
	if len(items) != 1 || items[0].Name != "edge" {
		t.Fatalf("unexpected initial gateway list: %+v", items)
	}
	if deleted, err := manager.Delete(context.Background(), "Gateway", "default", "edge"); err != nil {
		t.Fatalf("delete gateway: %v", err)
	} else if !deleted {
		t.Fatal("expected gateway delete to report deleted=true")
	}

	items, _, err = manager.List(context.Background(), filter, 0)
	if err != nil {
		t.Fatalf("gateway list after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty gateway list after delete, got %+v", items)
	}
	if got := counting.ListCalls(); got != 2 {
		t.Fatalf("list call count after invalidation = %d, want 2", got)
	}
}

func TestNamespaceListInvalidationClearsStringCache(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	manager.listCache.ttl = time.Minute
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	initial, err := manager.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("initial namespace list: %v", err)
	}
	if len(initial) != 1 || initial[0] != "backend" {
		t.Fatalf("unexpected initial namespace list: %+v", initial)
	}

	cached, err := manager.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("cached namespace list: %v", err)
	}
	if len(cached) != len(initial) || cached[0] != initial[0] {
		t.Fatalf("unexpected cached namespace list: initial=%+v cached=%+v", initial, cached)
	}
	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("namespace list call count before invalidation = %d, want 1", got)
	}

	if err := manager.client.Delete(context.Background(), &corev1.Namespace{
		TypeMeta:   metav1TypeMeta("v1", "Namespace"),
		ObjectMeta: metav1ObjectMeta("", "backend"),
	}); err != nil {
		t.Fatalf("delete namespace directly: %v", err)
	}

	stale, err := manager.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("namespace list before invalidation: %v", err)
	}
	if len(stale) != len(initial) || stale[0] != initial[0] {
		t.Fatalf("expected cached namespace list before invalidation: initial=%+v stale=%+v", initial, stale)
	}
	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("namespace list call count before cache invalidation trigger = %d, want 1", got)
	}

	if deleted, err := manager.Delete(context.Background(), "Gateway", "default", "edge"); err != nil {
		t.Fatalf("delete gateway: %v", err)
	} else if !deleted {
		t.Fatal("expected gateway delete to report deleted=true")
	}

	namespaces, err := manager.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("namespace list after invalidation: %v", err)
	}
	if len(namespaces) != 0 {
		t.Fatalf("expected empty namespace list after delete, got %+v", namespaces)
	}
	if got := counting.ListCalls(); got != 2 {
		t.Fatalf("namespace list call count after invalidation = %d, want 2", got)
	}
}

func TestResourceEndpointRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	for _, path := range []string{
		"/v1/resources?limit=0",
		"/v1/resources?limit=-1",
		"/v1/resources?offset=-1",
		"/v1/resources?limit=abc",
	} {
		recorder := performRequest(t, server, http.MethodGet, path, nil)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %q, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestResourceListSkipsUnavailableOptionalKinds(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	manager.client = noMatchListClient{Client: manager.client}
	server := newTestServerWithResourceManager(t, manager)

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", &kinds)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	backendLBPolicyAvailable := true
	for _, kind := range kinds {
		if kind.Kind == "BackendLBPolicy" {
			backendLBPolicyAvailable = kind.Available
			if kind.AvailabilityMessage == "" {
				t.Fatalf("expected availability message for unavailable kind, got %+v", kind)
			}
		}
	}
	if backendLBPolicyAvailable {
		t.Fatalf("expected BackendLBPolicy to be marked unavailable, got %+v", kinds)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(resources) == 0 {
		t.Fatalf("expected resources despite unavailable optional kinds, got %+v", resources)
	}
}

func TestResourceEndpointsMapKubernetesErrors(t *testing.T) {
	t.Parallel()

	baseManager := resourceManagerForTest(t)

	forbiddenManager := resourceManagerForTest(t)
	forbiddenManager.client = resourceErrorClient{
		Client: forbiddenManager.client,
		createErr: apierrors.NewForbidden(
			schema.GroupResource{Group: gatewayv1.GroupName, Resource: "httproutes"},
			"created",
			fmt.Errorf("rbac denied"),
		),
		listErr: apierrors.NewForbidden(
			schema.GroupResource{Group: gatewayv1.GroupName, Resource: "gateways"},
			"",
			fmt.Errorf("rbac denied"),
		),
	}

	conflictManager := resourceManagerForTest(t)
	conflictManager.client = resourceErrorClient{
		Client: conflictManager.client,
		updateErr: apierrors.NewConflict(
			schema.GroupResource{Group: gatewayv1.GroupName, Resource: "gateways"},
			"edge",
			fmt.Errorf("write conflict"),
		),
	}

	testCases := []struct {
		name       string
		server     *Server
		method     string
		path       string
		body       []byte
		statusCode int
	}{
		{
			name:       "list forbidden",
			server:     newTestServerWithResourceManager(t, forbiddenManager),
			method:     http.MethodGet,
			path:       "/v1/resources?kind=Gateway",
			statusCode: http.StatusForbidden,
		},
		{
			name:   "create forbidden",
			server: newTestServerWithResourceManager(t, forbiddenManager),
			method: http.MethodPost,
			path:   "/v1/resources",
			body: []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: created
  namespace: default
spec:
  rules:
    - backendRefs:
        - name: api
          port: 80
`),
			statusCode: http.StatusForbidden,
		},
		{
			name:   "update conflict",
			server: newTestServerWithResourceManager(t, conflictManager),
			method: http.MethodPut,
			path:   "/v1/resources/gateway/default/edge",
			body: []byte(`{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "Gateway",
  "metadata": {
    "name": "edge",
    "namespace": "default"
  },
  "spec": {
    "gatewayClassName": "nantian-gw"
  }
}`),
			statusCode: http.StatusConflict,
		},
		{
			name:       "invalid request stays bad request",
			server:     newTestServerWithResourceManager(t, baseManager),
			method:     http.MethodPut,
			path:       "/v1/resources/httproute/default/created",
			body:       []byte(`{"kind":"Gateway"}`),
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performRequestWithBody(t, tc.server, tc.method, tc.path, tc.body, nil)
			if recorder.Code != tc.statusCode {
				t.Fatalf("expected %d, got %d: %s", tc.statusCode, recorder.Code, recorder.Body.String())
			}
		})
	}
}
