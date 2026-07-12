package admin

import (
	"strings"
	"testing"
)

// TestBuildSystemPrompt_FramesRAGWithNoncePair verifies that a non-empty RAG
// context is wrapped in exactly one matched pair of nonce boundary markers with
// the framing instruction present.
func TestBuildSystemPrompt_FramesRAGWithNoncePair(t *testing.T) {
	rag := "## Topology Index\n- Gateway default/public\n"
	out := buildSystemPrompt(rag)

	if got := strings.Count(out, "<<CLUSTER_DATA_"); got != 1 {
		t.Fatalf("want exactly one opening marker, got %d:\n%s", got, out)
	}
	if got := strings.Count(out, "<<END_CLUSTER_DATA_"); got != 1 {
		t.Fatalf("want exactly one closing marker, got %d:\n%s", got, out)
	}
	if !strings.Contains(out, "READ-ONLY cluster data") {
		t.Errorf("missing framing instruction:\n%s", out)
	}
	if !strings.Contains(out, rag) {
		t.Errorf("rag context not present between markers:\n%s", out)
	}

	open := markerToken(out, "<<CLUSTER_DATA_")
	closeTok := markerToken(out, "<<END_CLUSTER_DATA_")
	if open == "" || open != closeTok {
		t.Errorf("opening/closing nonce mismatch: open=%q close=%q", open, closeTok)
	}
}

// markerToken extracts the nonce that follows prefix up to the closing ">>".
func markerToken(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	j := strings.Index(rest, ">>")
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func TestBuildSystemPrompt_EmptyRAGNoMarkers(t *testing.T) {
	out := buildSystemPrompt("")
	if strings.Contains(out, "CLUSTER_DATA") {
		t.Errorf("empty ragContext must not add framing markers:\n%s", out)
	}
}

func TestBuildSystemPrompt_NonceDiffersAcrossCalls(t *testing.T) {
	a := buildSystemPrompt("x")
	b := buildSystemPrompt("x")
	if a == b {
		t.Error("expected per-request nonce to differ across calls")
	}
}
