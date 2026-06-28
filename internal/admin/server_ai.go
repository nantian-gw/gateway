package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// AIOverview is the summary returned by the AI overview endpoint.
type AIOverview struct {
	TotalServices  int      `json:"totalServices"`
	TotalTokens    uint64   `json:"totalTokens"`
	TotalRequests  uint64   `json:"totalRequests"`
	AverageLatency float64  `json:"averageLatency"`
	ActiveModels   []string `json:"activeModels"`
}

// AIServiceSummary is the per-service summary returned by the AI services endpoint.
type AIServiceSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Provider  string `json:"provider"`
	Format    string `json:"format"`
	Model     string `json:"model"`
	Status    string `json:"status"`
}

// AITokenUsage is a single token usage data point.
type AITokenUsage struct {
	Model            string `json:"model"`
	PromptTokens     uint64 `json:"promptTokens"`
	CompletionTokens uint64 `json:"completionTokens"`
	TotalTokens      uint64 `json:"totalTokens"`
	Timestamp        string `json:"timestamp"`
}

// AITraceSummary is a single trace entry.
type AITraceSummary struct {
	ID        string  `json:"id"`
	Model     string  `json:"model"`
	Duration  float64 `json:"duration"`
	Tokens    uint64  `json:"tokens"`
	Status    string  `json:"status"`
	Timestamp string  `json:"timestamp"`
}

// AICostSummary is the cost overview returned by the cost endpoint.
type AICostSummary struct {
	TotalCost float64          `json:"totalCost"`
	TodayCost float64          `json:"todayCost"`
	MonthCost float64          `json:"monthCost"`
	ByModel   []AICostByModel  `json:"byModel"`
	Trend     []AICostTrend    `json:"trend"`
}

// AICostByModel is a per-model cost breakdown.
type AICostByModel struct {
	Model        string  `json:"model"`
	InputTokens  uint64  `json:"inputTokens"`
	OutputTokens uint64  `json:"outputTokens"`
	Cost         float64 `json:"cost"`
	Requests     uint64  `json:"requests"`
}

// AICostTrend is a single cost trend data point.
type AICostTrend struct {
	Date  string  `json:"date"`
	Cost  float64 `json:"cost"`
	Model string  `json:"model"`
}

// In-memory store for AI admin data (populated by the dataplane or translator).
// These are atomic counters and simple maps; in production, this would come from
// Prometheus metrics, Langfuse, or a dedicated time-series store.
var (
	aiTotalTokens   atomic.Uint64
	aiTotalRequests atomic.Uint64
	aiServices      = make(map[string]*AIServiceSummary, 16)
	aiTokenUsage    []AITokenUsage
	aiTraces        []AITraceSummary
	aiModelCosts    = make(map[string]float64)
	aiTotalCost     atomic.Value // stores float64
	aiLatencySum    atomic.Uint64 // cumulative latency in microseconds
	aiMu            sync.RWMutex
)

func init() {
	aiTotalCost.Store(float64(0))
}

func (s *Server) handleAIOverview(w http.ResponseWriter, r *http.Request) {
	services := s.listAIServices()
	models := make([]string, 0, len(services))
	seen := make(map[string]bool)
	for _, svc := range services {
		if !seen[svc.Model] {
			models = append(models, svc.Model)
			seen[svc.Model] = true
		}
	}
	sort.Strings(models)

	avgLatency := float64(0)
	if requests := aiTotalRequests.Load(); requests > 0 {
		avgLatency = float64(aiLatencySum.Load()) / float64(requests) / 1000.0 // convert to ms
	}

	overview := AIOverview{
		TotalServices:  len(services),
		TotalTokens:    aiTotalTokens.Load(),
		TotalRequests:  aiTotalRequests.Load(),
		AverageLatency: avgLatency,
		ActiveModels:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}

func (s *Server) handleAIServices(w http.ResponseWriter, r *http.Request) {
	services := s.listAIServices()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (s *Server) handleAITokenUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	aiMu.RLock()
	json.NewEncoder(w).Encode(aiTokenUsage)
	aiMu.RUnlock()
}

func (s *Server) handleAITraces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	aiMu.RLock()
	json.NewEncoder(w).Encode(aiTraces)
	aiMu.RUnlock()
}

func (s *Server) handleAICost(w http.ResponseWriter, r *http.Request) {
	cost := AICostSummary{
		TotalCost: aiTotalCost.Load().(float64),
		TodayCost: aiTotalCost.Load().(float64) * 0.1, // placeholder
		MonthCost: aiTotalCost.Load().(float64),
		ByModel:   []AICostByModel{},
		Trend: []AICostTrend{
			{Date: time.Now().Format("2006-01-02"), Cost: aiTotalCost.Load().(float64), Model: "all"},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cost)
}

// listAIServices returns AI services from the resource cache.
func (s *Server) listAIServices() []AIServiceSummary {
	// Return in-memory services; in production, this would query the informer cache.
	// For now, return the cached services map.
	services := make([]AIServiceSummary, 0, len(aiServices))
	aiMu.RLock()
	for _, svc := range aiServices {
		services = append(services, *svc)
	}
	aiMu.RUnlock()
	sort.Slice(services, func(i, j int) bool {
		if services[i].Namespace != services[j].Namespace {
			return services[i].Namespace < services[j].Namespace
		}
		return services[i].Name < services[j].Name
	})
	return services
}