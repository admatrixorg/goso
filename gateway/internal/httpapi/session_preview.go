package httpapi

import (
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const sessionPreviewRunes = 80

// sessionPreview is the first readable message, used when a session has no title.
// It is not stored. Event and tool rows are skipped.
func sessionPreview(msgs []*store.Message) string {
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if m.Role == "event" || m.Role == "tool" {
			continue
		}
		text := strings.Join(strings.Fields(m.Content), " ")
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) <= sessionPreviewRunes {
			return text
		}
		return string([]rune(text)[:sessionPreviewRunes]) + "…"
	}
	return ""
}
