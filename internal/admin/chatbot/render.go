package chatbot

import (
	"fmt"
	"strings"
)

// detailBlockCap bounds a single resource's rendered detail block so one large
// resource cannot starve the budget or blow up a single block.
const detailBlockCap = 2500

// renderContext produces the two-section Markdown RAG context: a lightweight
// index of every resource, then detailed blocks for selected resources up to
// a character budget.
func renderContext(index ClusterIndex, selected []ResourceRef, budget int, usedFallback bool) string {
	var sb strings.Builder

	sb.WriteString("## Topology Index\n\n")
	indexBudget := budget / 2
	indexUsed := 0
	indexTruncated := false
	for _, e := range index.Entries {
		line := fmt.Sprintf("- %s %s/%s", e.Ref.Kind, e.Ref.Namespace, e.Ref.Name)
		if e.StatusSummary != "" {
			line += " [" + e.StatusSummary + "]"
		}
		line += "\n"
		if indexUsed+len(line) > indexBudget {
			indexTruncated = true
			break
		}
		sb.WriteString(line)
		indexUsed += len(line)
	}
	if indexTruncated {
		sb.WriteString("_(index truncated)_\n")
	}

	sb.WriteString("\n## Relevant Resources\n\n")
	if usedFallback {
		sb.WriteString("_No specific resource matched; showing a breadth sample._\n\n")
	}

	detailByRef := make(map[string]IndexEntry, len(index.Entries))
	for _, e := range index.Entries {
		detailByRef[e.Ref.String()] = e
	}

	detailBudget := budget - sb.Len()
	detailUsed := 0
	truncated := false
	for _, ref := range selected {
		e, ok := detailByRef[ref.String()]
		if !ok {
			continue
		}
		body := renderDetail(index.objects[ref], e)
		if len(body) > detailBlockCap {
			body = strings.ToValidUTF8(body[:detailBlockCap], "") + "…(truncated)\n"
		}
		block := fmt.Sprintf("### %s %s/%s\n%s\n", e.Ref.Kind, e.Ref.Namespace, e.Ref.Name, body)
		if detailUsed+len(block) > detailBudget {
			truncated = true
			break
		}
		sb.WriteString(block)
		detailUsed += len(block)
	}

	if truncated || indexTruncated {
		sb.WriteString("\n_(detail truncated to fit budget; ask about specific resources for more)_\n")
	}

	return sb.String()
}
