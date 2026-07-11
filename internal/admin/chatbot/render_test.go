package chatbot

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderContext_TwoSections(t *testing.T) {
	index := ClusterIndex{Entries: []IndexEntry{
		{Ref: ResourceRef{Kind: kindGateway, Namespace: "default", Name: "public"}, Summary: "class=nantian-gw listeners=1", StatusSummary: "Programmed=True"},
	}}
	selected := []ResourceRef{{Kind: kindGateway, Namespace: "default", Name: "public"}}
	out := renderContext(index, selected, 4000, false)
	if !strings.Contains(out, "## Topology Index") {
		t.Error("missing index section")
	}
	if !strings.Contains(out, "## Relevant Resources") {
		t.Error("missing detail section")
	}
	if !strings.Contains(out, "public") {
		t.Error("missing resource")
	}
}

func TestRenderContext_BudgetTruncation(t *testing.T) {
	var entries []IndexEntry
	var selected []ResourceRef
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("route-%d", i)
		entries = append(entries, IndexEntry{Ref: ResourceRef{Kind: kindHTTPRoute, Namespace: "default", Name: name}, Summary: "rules=1"})
		selected = append(selected, ResourceRef{Kind: kindHTTPRoute, Namespace: "default", Name: name})
	}
	index := ClusterIndex{Entries: entries}
	out := renderContext(index, selected, 500, false)
	if len(out) > 1500 {
		t.Errorf("output %d exceeds budget-ish bound", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Error("expected truncation notice")
	}
}
