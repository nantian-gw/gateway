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

// kindKeywordsASCII maps a whole-token ASCII query word to a resource kind.
// Matched by exact token (not substring) so short words like "ai"/"lb" do not
// fire inside unrelated words ("explain", "bulb").
var kindKeywordsASCII = map[string]string{
	"gateway":   kindGateway,
	"httproute": kindHTTPRoute,
	"http":      kindHTTPRoute,
	"grpc":      kindGRPCRoute,
	"grpcroute": kindGRPCRoute,
	"tls":       kindTLSRoute,
	"tlsroute":  kindTLSRoute,
	"tcp":       kindTCPRoute,
	"tcproute":  kindTCPRoute,
	"udp":       kindUDPRoute,
	"udproute":  kindUDPRoute,
	"service":   kindService,
	"ai":        kindAIService,
	"aiservice": kindAIService,
	"model":     kindAIService,
	"token":     kindTokenPolicy,
	"wasm":      kindWasmPlugin,
	"backend":   kindBackendLBPolicy,
	"lb":        kindBackendLBPolicy,
}

// kindKeywordsCJK maps CJK keywords to kinds. Matched by substring because
// strings.Fields cannot tokenize CJK text; these are all non-ASCII so they
// cannot false-match inside ASCII words.
var kindKeywordsCJK = map[string]string{
	"网关": kindGateway,
	"路由": kindHTTPRoute,
	"服务": kindService,
	"模型": kindAIService,
	"限流": kindTokenPolicy,
	"插件": kindWasmPlugin,
	"负载": kindBackendLBPolicy,
}

// selectRelevant scores index entries against the query and returns them in
// descending score order, then expands with one-hop associations. usedFallback
// is true when no signal matched and a breadth view is returned instead.
func selectRelevant(index ClusterIndex, query string) (selected []ResourceRef, usedFallback bool) {
	lower := strings.ToLower(query)
	tokens := strings.Fields(lower)
	tokenSet := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tokenSet[t] = true
	}

	kindHits := make(map[string]bool)
	for kw, kind := range kindKeywordsASCII {
		if tokenSet[kw] {
			kindHits[kind] = true
		}
	}
	for kw, kind := range kindKeywordsCJK {
		if strings.Contains(lower, kw) {
			kindHits[kind] = true
		}
	}

	nsHits := make(map[string]bool)
	for _, e := range index.Entries {
		if e.Ref.Namespace != "" && tokenSet[strings.ToLower(e.Ref.Namespace)] {
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
		if e.Ref.Name != "" && tokenSet[strings.ToLower(e.Ref.Name)] {
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

	if len(results) == 0 {
		return fallbackBreadth(index), true
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].score > results[j].score })

	entryByRef := make(map[ResourceRef]IndexEntry, len(index.Entries))
	for _, e := range index.Entries {
		entryByRef[e.Ref] = e
	}
	selectedSet := make(map[ResourceRef]bool, len(results))
	for _, r := range results {
		selectedSet[r.ref] = true
	}

	selected = make([]ResourceRef, 0, len(results))
	for _, r := range results {
		selected = append(selected, r.ref)
		for _, a := range entryByRef[r.ref].assoc {
			if selectedSet[a] {
				continue
			}
			if _, ok := entryByRef[a]; ok {
				selectedSet[a] = true
				selected = append(selected, a)
			}
		}
	}
	return selected, false
}

func fallbackBreadth(index ClusterIndex) []ResourceRef {
	byKind := make(map[string][]IndexEntry)
	for _, e := range index.Entries {
		byKind[e.Ref.Kind] = append(byKind[e.Ref.Kind], e)
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	var out []ResourceRef
	for _, k := range kinds {
		entries := byKind[k]
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
