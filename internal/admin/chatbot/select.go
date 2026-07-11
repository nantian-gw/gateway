package chatbot

import (
	"sort"
	"strings"
)

const (
	scoreName       = 10
	scoreNS         = 4
	scoreKind       = 3
	scoreAbnormal   = 2
	fallbackPerKind = 5
)

// kindKeywords maps a lowercase query token to a resource kind. Bilingual.
var kindKeywords = map[string]string{
	"gateway":   kindGateway,
	"网关":        kindGateway,
	"httproute": kindHTTPRoute,
	"http":      kindHTTPRoute,
	"路由":        kindHTTPRoute,
	"grpc":      kindGRPCRoute,
	"grpcroute": kindGRPCRoute,
	"tls":       kindTLSRoute,
	"tlsroute":  kindTLSRoute,
	"tcp":       kindTCPRoute,
	"tcproute":  kindTCPRoute,
	"udp":       kindUDPRoute,
	"udproute":  kindUDPRoute,
	"service":   kindService,
	"服务":        kindService,
	"ai":        kindAIService,
	"aiservice": kindAIService,
	"model":     kindAIService,
	"模型":        kindAIService,
	"token":     kindTokenPolicy,
	"限流":        kindTokenPolicy,
	"wasm":      kindWasmPlugin,
	"插件":        kindWasmPlugin,
	"backend":   kindBackendLBPolicy,
	"lb":        kindBackendLBPolicy,
	"负载":        kindBackendLBPolicy,
}

// selectRelevant scores index entries against the query and returns them in
// descending score order. usedFallback is true when no signal matched and a
// breadth view is returned instead.
func selectRelevant(index ClusterIndex, query string) (selected []ResourceRef, usedFallback bool) {
	lower := strings.ToLower(query)
	tokens := strings.Fields(lower)
	tokenSet := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tokenSet[t] = true
	}

	// Kind hints: keyword tokens plus Chinese keywords found as substrings.
	kindHits := make(map[string]bool)
	for kw, kind := range kindKeywords {
		if tokenSet[kw] || strings.Contains(lower, kw) {
			kindHits[kind] = true
		}
	}

	// Namespace / name hints derived from what actually exists in the index.
	nsHits := make(map[string]bool)
	for _, e := range index.Entries {
		if e.Ref.Namespace != "" && (tokenSet[strings.ToLower(e.Ref.Namespace)]) {
			nsHits[e.Ref.Namespace] = true
		}
	}

	type scored struct {
		ref   ResourceRef
		score int
	}
	var results []scored
	for _, e := range index.Entries {
		s := 0
		nameLower := strings.ToLower(e.Ref.Name)
		if e.Ref.Name != "" && (tokenSet[nameLower] || strings.Contains(lower, nameLower)) {
			s += scoreName
		}
		if nsHits[e.Ref.Namespace] {
			s += scoreNS
		}
		if kindHits[e.Ref.Kind] {
			s += scoreKind
		}
		if s > 0 && e.Abnormal {
			s += scoreAbnormal
		}
		if s > 0 {
			results = append(results, scored{ref: e.Ref, score: s})
		}
	}

	if len(results) > 0 {
		sort.SliceStable(results, func(i, j int) bool { return results[i].score > results[j].score })
		selected = make([]ResourceRef, 0, len(results))
		for _, r := range results {
			selected = append(selected, r.ref)
		}
		return selected, false
	}

	// Fallback: breadth mode — top-N per kind, abnormal first.
	return fallbackBreadth(index), true
}

func fallbackBreadth(index ClusterIndex) []ResourceRef {
	byKind := make(map[string][]IndexEntry)
	for _, e := range index.Entries {
		byKind[e.Ref.Kind] = append(byKind[e.Ref.Kind], e)
	}
	var out []ResourceRef
	for _, entries := range byKind {
		sort.SliceStable(entries, func(i, j int) bool {
			return boolToInt(entries[i].Abnormal) > boolToInt(entries[j].Abnormal)
		})
		for i, e := range entries {
			if i >= fallbackPerKind {
				break
			}
			out = append(out, e.Ref)
		}
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
