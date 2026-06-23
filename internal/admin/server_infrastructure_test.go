package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func TestInfrastructureEndpointRequiresConfiguredInspector(t *testing.T) {
	server := newTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/infrastructure", nil)
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without inspector, got %d", recorder.Code)
	}
}

func TestInfrastructureEndpointReturnsDerivedResourceReport(t *testing.T) {
	server := newInfrastructureTestServer(t)

	var report infrastructure.InfrastructureReport
	recorder := performRequest(t, server, http.MethodGet, "/v1/infrastructure", &report)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if report.Summary.MissingCount == 0 || report.Summary.DriftedCount == 0 {
		t.Fatalf("expected drift report with issues, got %+v", report.Summary)
	}

	var sharedService infrastructure.InfrastructureResource
	for _, item := range report.Resources {
		if item.Kind == infrastructure.InfrastructureKindService &&
			item.Namespace == "nantian-gw" &&
			item.Name == "nantian-gw-dataplane" {
			sharedService = item
			break
		}
	}
	if sharedService.Name == "" || sharedService.State != infrastructure.InfrastructureStateMissing {
		t.Fatalf("expected missing shared service in report, got %+v", report.Resources)
	}
}

func TestInfrastructureEndpointIncludesGatewayConvergenceSummary(t *testing.T) {
	server := newInfrastructureTestServer(t)

	recorder := performRequest(t, server, http.MethodGet, "/v1/infrastructure", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode infrastructure response: %v", err)
	}

	summary, ok := response["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary object, got %#v", response["summary"])
	}
	convergence, ok := summary["gatewayConvergence"].(map[string]any)
	if !ok {
		t.Fatalf("expected gatewayConvergence summary, got %#v", summary["gatewayConvergence"])
	}

	if got := convergence["gatewayCount"]; got != float64(1) {
		t.Fatalf("gatewayCount = %#v, want 1", got)
	}
	if got := convergence["pendingServiceMetadataCount"]; got != float64(1) {
		t.Fatalf("pendingServiceMetadataCount = %#v, want 1", got)
	}
	if got := convergence["pendingFrontendEndpointSliceCount"]; got != float64(0) {
		t.Fatalf("pendingFrontendEndpointSliceCount = %#v, want 0", got)
	}
	if got := convergence["pendingProgrammedObservedGenerationCount"]; got != float64(0) {
		t.Fatalf("pendingProgrammedObservedGenerationCount = %#v, want 0", got)
	}
}

func TestInfrastructureEndpointSupportsFilteringSortingAndPagination(t *testing.T) {
	server := newInfrastructureTestServer(t)

	var report infrastructure.InfrastructureReport
	recorder := performRequest(
		t,
		server,
		http.MethodGet,
		"/v1/infrastructure?kind=service&state=drifted&sort=name&order=desc",
		&report,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if report.Summary.ResourceCount != 8 || report.Summary.MissingCount != 5 || report.Summary.DriftedCount != 2 || report.Summary.OrphanCount != 1 {
		t.Fatalf("expected full infrastructure summary to stay intact, got %+v", report.Summary)
	}
	if got := infrastructureResourceKeys(report.Resources); strings.Join(got, ",") != "default/"+infrastructure.GatewayServiceName("public")+",default/echo" {
		t.Fatalf("unexpected filtered infrastructure resources: %+v", got)
	}
	for _, item := range report.Resources {
		if item.Kind != infrastructure.InfrastructureKindService || item.State != infrastructure.InfrastructureStateDrifted {
			t.Fatalf("unexpected filtered infrastructure item: %+v", item)
		}
	}

	recorder = performRequest(
		t,
		server,
		http.MethodGet,
		"/v1/infrastructure?kind=service&namespace=default&sort=name&offset=1&limit=2",
		&report,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := infrastructureResourceKeys(report.Resources); strings.Join(got, ",") != "default/"+infrastructure.GatewayServiceName("public")+",default/"+mesh.ShadowServiceName("default", "echo") {
		t.Fatalf("unexpected paginated infrastructure resources: %+v", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?state=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure state, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?role=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure role, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?kind=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure kind, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?sort=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure order, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?limit=0", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure limit, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/infrastructure?offset=-1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid infrastructure offset, got %d", recorder.Code)
	}
}

func TestInfrastructureEmitsPaginationHeaders(t *testing.T) {
	t.Parallel()

	server := newInfrastructureTestServer(t)

	var report infrastructure.InfrastructureReport
	recorder := performRequest(t, server, http.MethodGet, "/v1/infrastructure?kind=service&offset=1&limit=2", &report)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "2" {
		t.Fatalf("unexpected page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "1" {
		t.Fatalf("unexpected page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "5" {
		t.Fatalf("unexpected total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected has-next-page header: %q", got)
	}
}
