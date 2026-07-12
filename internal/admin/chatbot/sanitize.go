package chatbot

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxUntrustedValueLen bounds a single sanitized value so one field cannot
// dominate the RAG context.
const maxUntrustedValueLen = 200

// sanitizeUntrusted neutralizes an untrusted, cluster-sourced string before it
// is embedded into the Markdown RAG context. It uses visible escaping so values
// stay readable while losing the ability to break out of their line or forge the
// system-prompt data boundary:
//
//   - backtick -> apostrophe            (cannot open/close a code fence)
//   - ASCII '<' -> '‹' (U+2039)         (cannot emit ASCII '<' -> cannot forge <<nonce>>)
//   - ASCII '>' -> '›' (U+203A)
//   - any whitespace run -> a single space (cannot start a new line)
//   - other control chars (< 0x20, 0x7F) -> dropped
//   - longer than maxUntrustedValueLen runes -> truncated with an ellipsis
//
// Normal DNS-1123 names and typical field values contain none of these, so they
// pass through unchanged. The function is total.
func sanitizeUntrusted(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		case r == '`':
			b.WriteByte('\'')
		case r == '<':
			b.WriteRune('‹')
		case r == '>':
			b.WriteRune('›')
		case r < 0x20 || r == 0x7f:
			continue
		default:
			b.WriteRune(r)
		}
		prevSpace = false
	}
	out := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(out) > maxUntrustedValueLen {
		count := 0
		for i := range out {
			if count == maxUntrustedValueLen {
				out = out[:i]
				break
			}
			count++
		}
		out += "…"
	}
	return out
}

// suAny sanitizes any value's default string form for safe embedding at an
// injection point. Numeric/bool values are unaffected (no special chars).
func suAny(v any) string { return sanitizeUntrusted(fmt.Sprint(v)) }
