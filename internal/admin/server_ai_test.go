package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestListAIServicesSortsByNamespaceThenName(t *testing.T) {
	restoreAIState(t)
	aiServices["z"] = &AIServiceSummary{Namespace: "team-b", Name: "api", Model: "gpt-4.1"}
	aiServices["a"] = &AIServiceSummary{Namespace: "team-a", Name: "worker", Model: "gpt-4.1-mini"}
	aiServices["b"] = &AIServiceSummary{Namespace: "team-a", Name: "api", Model: "gpt-4.1"}

	got := (&Server{}).listAIServices()
	gotNames := make([]string, 0, len(got))
	for _, service := range got {
		gotNames = append(gotNames, service.Namespace+"/"+service.Name)
	}
	wantNames := []string{"team-a/api", "team-a/worker", "team-b/api"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("listAIServices() order = %#v, want %#v", gotNames, wantNames)
	}
}

func TestHandleAIOverviewReturnsSortedModelsAndAverageLatency(t *testing.T) {
	restoreAIState(t)
	aiServices["orders"] = &AIServiceSummary{Namespace: "apps", Name: "orders", Model: "gpt-4.1-mini"}
	aiServices["billing"] = &AIServiceSummary{Namespace: "apps", Name: "billing", Model: "gpt-4.1"}
	aiServices["search"] = &AIServiceSummary{Namespace: "apps", Name: "search", Model: "gpt-4.1"}
	aiTotalTokens.Store(300)
	aiTotalRequests.Store(2)
	aiLatencySum.Store(5000)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/ai/overview", http.NoBody)
	(&Server{}).handleAIOverview(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got AIOverview
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.TotalServices != 3 {
		t.Fatalf("TotalServices = %d, want 3", got.TotalServices)
	}
	if got.TotalTokens != 300 {
		t.Fatalf("TotalTokens = %d, want 300", got.TotalTokens)
	}
	if got.TotalRequests != 2 {
		t.Fatalf("TotalRequests = %d, want 2", got.TotalRequests)
	}
	if got.AverageLatency != 2.5 {
		t.Fatalf("AverageLatency = %v, want 2.5", got.AverageLatency)
	}
	if !reflect.DeepEqual(got.ActiveModels, []string{"gpt-4.1", "gpt-4.1-mini"}) {
		t.Fatalf("ActiveModels = %#v, want sorted unique models", got.ActiveModels)
	}
}

func restoreAIState(t *testing.T) {
	t.Helper()
	oldServices := aiServices
	oldUsage := aiTokenUsage
	oldTraces := aiTraces
	oldCosts := aiModelCosts
	oldTotalCost := aiTotalCost.Load().(float64)
	oldTokens := aiTotalTokens.Load()
	oldRequests := aiTotalRequests.Load()
	oldLatency := aiLatencySum.Load()

	aiServices = make(map[string]*AIServiceSummary)
	aiTokenUsage = nil
	aiTraces = nil
	aiModelCosts = make(map[string]float64)
	aiTotalCost.Store(float64(0))
	aiTotalTokens.Store(0)
	aiTotalRequests.Store(0)
	aiLatencySum.Store(0)

	t.Cleanup(func() {
		aiServices = oldServices
		aiTokenUsage = oldUsage
		aiTraces = oldTraces
		aiModelCosts = oldCosts
		aiTotalCost.Store(oldTotalCost)
		aiTotalTokens.Store(oldTokens)
		aiTotalRequests.Store(oldRequests)
		aiLatencySum.Store(oldLatency)
	})
}
