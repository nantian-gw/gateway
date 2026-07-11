package chatbot

import "testing"

func idx(entries ...IndexEntry) ClusterIndex { return ClusterIndex{Entries: entries} }

func entry(kind, ns, name string, abnormal bool) IndexEntry {
	return IndexEntry{Ref: ResourceRef{Kind: kind, Namespace: ns, Name: name}, Abnormal: abnormal}
}

func refSet(refs []ResourceRef) map[string]bool {
	m := make(map[string]bool, len(refs))
	for _, r := range refs {
		m[r.String()] = true
	}
	return m
}

func TestSelectRelevant_NameHitWins(t *testing.T) {
	index := idx(
		entry(kindHTTPRoute, "default", "checkout", false),
		entry(kindHTTPRoute, "default", "unrelated", false),
	)
	got, fallback := selectRelevant(index, "why is checkout route failing")
	if fallback {
		t.Fatal("did not expect fallback when a name matched")
	}
	if len(got) == 0 || got[0].Name != "checkout" {
		t.Fatalf("expected checkout first, got %+v", got)
	}
	if refSet(got)["HTTPRoute default/unrelated"] {
		t.Error("unrelated route should not be selected on a specific name query")
	}
}

func TestSelectRelevant_KindKeywordChinese(t *testing.T) {
	index := idx(
		entry(kindAIService, "ai", "gpt", false),
		entry(kindService, "default", "web", false),
	)
	got, _ := selectRelevant(index, "列出所有 模型 服务")
	if !refSet(got)["AIService ai/gpt"] {
		t.Errorf("expected AIService selected for '模型', got %+v", got)
	}
}

func TestSelectRelevant_AbnormalWeighting(t *testing.T) {
	index := idx(
		entry(kindGateway, "default", "healthy", false),
		entry(kindGateway, "default", "broken", true),
	)
	got, _ := selectRelevant(index, "gateway")
	if len(got) < 2 || got[0].Name != "broken" {
		t.Fatalf("abnormal gateway should rank first, got %+v", got)
	}
}

func TestSelectRelevant_FallbackWhenNoHit(t *testing.T) {
	index := idx(entry(kindGateway, "default", "public", false))
	got, fallback := selectRelevant(index, "hello there")
	if !fallback {
		t.Error("expected fallback for a query with no hints")
	}
	if len(got) == 0 {
		t.Error("fallback should still return some breadth entries")
	}
}
