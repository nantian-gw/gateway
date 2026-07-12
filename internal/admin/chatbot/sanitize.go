package chatbot

import (
	"strings"
	"unicode/utf8"
)

// maxUntrustedLen bounds a single untrusted field value so a free-text field
// cannot exhaust the RAG budget or smuggle a large injection payload.
const maxUntrustedLen = 200

// sanitizeUntrusted neutralizes a single untrusted string value sourced from a
// live cluster resource before it is interpolated into the RAG Markdown context.
//
// It is a structural defense, not a semantic one: it prevents a value from
// breaking out of its inline position (forging a new Markdown line/heading, a
// code fence, or the nonce boundary marker) but does not attempt to detect
// natural-language injection phrases — that is the job of the nonce framing in
// buildSystemPrompt.
//
// Rules, applied in order:
//  1. Replace newlines and other C0 control characters (and DEL) with a single
//     space, forcing the value onto one line.
//  2. Replace backticks with single quotes so a value cannot open or forge a
//     code fence (which would also disturb downstream YAML extraction).
//  3. Replace "<<"/">>" sequences with unicode angle quotes so a value cannot
//     assemble a real boundary marker even if the per-request nonce were known.
//  4. Cap the length (rune-aware), appending an ellipsis when truncated.
func sanitizeUntrusted(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x7F || r < 0x20:
			// C0 control characters, including \n \r \t.
			b.WriteByte(' ')
		case r == '`':
			b.WriteByte('\'')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()

	out = strings.ReplaceAll(out, "<<", "‹‹")
	out = strings.ReplaceAll(out, ">>", "››")

	if utf8.RuneCountInString(out) > maxUntrustedLen {
		out = string([]rune(out)[:maxUntrustedLen]) + "…"
	}
	return out
}
