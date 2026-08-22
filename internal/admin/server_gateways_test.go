package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
)

func TestGatewaysEndpoint_returnsEmptyList_whenNoGateways(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		logger,
		noderegistry.Options{PersistTimeout: time.Second},
	)
	now := time.Now().UTC()
	store.Publish(&ir.Snapshot{GeneratedAt: now})
	server := NewServer(":0", store, nodes, nil, logger, Options{})

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Gateways) != 0 {
		t.Fatalf("expected empty gateways, got %d", len(resp.Gateways))
	}
	if resp.Total != 0 {
		t.Fatalf("expected total 0, got %d", resp.Total)
	}
}

func TestGatewaysEndpoint_returnsGatewaysFromRouteParentRefs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	found := false
	for _, gw := range resp.Gateways {
		if gw.Name == "gw" && gw.Namespace == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected gateway 'gw/default' in response, got %+v", resp.Gateways)
	}
}

func TestGatewaysEndpoint_withIncludeRoutes(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways?include=routes", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	for _, gw := range resp.Gateways {
		if gw.Routes == nil {
			t.Fatalf("expected routes to be included for gateway %s/%s", gw.Namespace, gw.Name)
		}
		if gw.Routes.HTTP == nil {
			t.Fatalf("expected http routes to be non-nil")
		}
		if gw.Routes.GRPC == nil {
			t.Fatalf("expected grpc routes to be non-nil")
		}
		if gw.Routes.Stream == nil {
			t.Fatalf("expected stream routes to be non-nil")
		}
	}
}

func TestGatewaysEndpoint_withIncludeListeners(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways?include=listeners", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	for _, gw := range resp.Gateways {
		if gw.Name == "gw" && gw.Namespace == "default" {
			if gw.Listeners == nil {
				t.Fatal("expected listeners to be included")
			}
			break
		}
	}
}

func TestGatewaysEndpoint_withIncludeBackends(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways?include=backends", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	for _, gw := range resp.Gateways {
		if gw.Name == "gw" && gw.Namespace == "default" {
			if gw.Backends == nil {
				t.Fatal("expected backends to be included")
			}
			break
		}
	}
}

func TestGatewaysEndpoint_withIncludeSummary(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways?include=summary", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	for _, gw := range resp.Gateways {
		if gw.Name == "gw" && gw.Namespace == "default" {
			if gw.Summary == nil {
				t.Fatal("expected summary to be included")
			}
			if gw.Summary.RouteCount <= 0 {
				t.Fatalf("expected positive route count, got %d", gw.Summary.RouteCount)
			}
			if gw.Summary.ListenerCount <= 0 {
				t.Fatalf("expected positive listener count, got %d", gw.Summary.ListenerCount)
			}
			break
		}
	}
}

func TestGatewaysEndpoint_withAllIncludes(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways?include=routes,listeners,backends,summary", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total == 0 {
		t.Fatal("expected at least one gateway")
	}

	for _, gw := range resp.Gateways {
		if gw.Routes == nil {
			t.Fatalf("expected routes with include=all for %s/%s", gw.Namespace, gw.Name)
		}
		if gw.Listeners == nil {
			t.Fatalf("expected listeners with include=all for %s/%s", gw.Namespace, gw.Name)
		}
		if gw.Backends == nil {
			t.Fatalf("expected backends with include=all for %s/%s", gw.Namespace, gw.Name)
		}
		if gw.Summary == nil {
			t.Fatalf("expected summary with include=all for %s/%s", gw.Namespace, gw.Name)
		}
	}
}

func TestGatewaysEndpoint_omitsFields_whenNoInclude(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/gateways", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Gateways []gatewayDetail `json:"gateways"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, gw := range resp.Gateways {
		if gw.Routes != nil {
			t.Fatal("expected routes to be omitted when not requested")
		}
		if gw.Listeners != nil {
			t.Fatal("expected listeners to be omitted when not requested")
		}
		if gw.Backends != nil {
			t.Fatal("expected backends to be omitted when not requested")
		}
		if gw.Summary != nil {
			t.Fatal("expected summary to be omitted when not requested")
		}
	}
}

func TestParseIncludeFlagsUsesExactTokensAndAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want includeFlags
	}{
		{
			name: "comma separated tokens",
			raw:  " routes, LISTENERS ,backends,summary ",
			want: includeFlags{routes: true, listeners: true, backends: true, summary: true},
		},
		{
			name: "all token",
			raw:  "all",
			want: includeFlags{routes: true, listeners: true, backends: true, summary: true},
		},
		{
			name: "substring false positives ignored",
			raw:  "notroutes,listener,backend-summary",
			want: includeFlags{},
		},
		{
			name: "unknown tokens ignored",
			raw:  "routes,unknown",
			want: includeFlags{routes: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseIncludeFlags(tt.raw); got != tt.want {
				t.Fatalf("parseIncludeFlags(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildGatewayDetailsSortsByNamespaceAndName(t *testing.T) {
	t.Parallel()

	got := buildGatewayDetails(&ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "route-b",
				Namespace: "z-ns",
				ParentRefs: []ir.ParentRef{{
					Namespace: "z-ns",
					Name:      "b",
				}},
			},
			{
				Name:      "route-a",
				Namespace: "a-ns",
				ParentRefs: []ir.ParentRef{{
					Namespace: "a-ns",
					Name:      "z",
				}},
			},
			{
				Name:      "route-c",
				Namespace: "a-ns",
				ParentRefs: []ir.ParentRef{{
					Namespace: "a-ns",
					Name:      "a",
				}},
			},
		},
	}, includeFlags{})

	want := []gatewayIdentity{
		{namespace: "a-ns", name: "a"},
		{namespace: "a-ns", name: "z"},
		{namespace: "z-ns", name: "b"},
	}
	if len(got) != len(want) {
		t.Fatalf("gateway count = %d, want %d", len(got), len(want))
	}
	for i, gw := range got {
		if gw.Namespace != want[i].namespace || gw.Name != want[i].name {
			t.Fatalf("gateway[%d] = %s/%s, want %s/%s", i, gw.Namespace, gw.Name, want[i].namespace, want[i].name)
		}
	}
}
