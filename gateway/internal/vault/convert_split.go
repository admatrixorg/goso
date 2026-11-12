// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// splitMarkdown breaks markdown above max into pieces of at most max bytes.
// Pieces follow heading boundaries first. A heading section that is itself
// too long is split on a paragraph, then a line, then a rune. A piece never
// starts or ends in the middle of a UTF-8 character. Joining the pieces
// reproduces the input.
func splitMarkdown(body string, max int) ([]string, error) {
	if max < 4 {
		return nil, errors.New("split limit")
	}
	if len(body) <= max {
		if !utf8.ValidString(body) {
			return nil, errors.New("split utf-8")
		}
		return []string{body}, nil
	}
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		parts = append(parts, cur.String())
		cur.Reset()
	}
	for _, section := range headingSections(body) {
		if len(section) > max {
			flush()
			chunks, err := splitLarge(section, max)
			if err != nil {
				return nil, err
			}
			parts = append(parts, chunks...)
			continue
		}
		if cur.Len() > 0 && cur.Len()+len(section) > max {
			flush()
		}
		cur.WriteString(section)
	}
	flush()
	if strings.Join(parts, "") != body {
		return nil, errors.New("split mismatch")
	}
	for _, part := range parts {
		if len(part) > max || !utf8.ValidString(part) {
			return nil, errors.New("split part")
		}
	}
	if len(parts) == 0 {
		return nil, errors.New("split empty")
	}
	return parts, nil
}

func headingSections(body string) []string {
	lines := strings.SplitAfter(body, "\n")
	var sections []string
	var cur strings.Builder
	started := false
	for _, line := range lines {
		if started && isHeadingLine(line) && cur.Len() > 0 {
			sections = append(sections, cur.String())
			cur.Reset()
		}
		started = true
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		sections = append(sections, cur.String())
	}
	if len(sections) == 0 {
		return []string{body}
	}
	return sections
}

func isHeadingLine(line string) bool {
	trim := strings.TrimRight(line, "\r\n")
	n := 0
	for n < len(trim) && trim[n] == '#' {
		n++
	}
	if n < 1 || n > 6 || n == len(trim) {
		return false
	}
	return trim[n] == ' ' || trim[n] == '\t'
}

func splitLarge(s string, max int) ([]string, error) {
	var out []string
	for len(s) > max {
		cut := bestCut(s, max)
		if cut <= 0 || cut > len(s) {
			return nil, errors.New("split part")
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	if s != "" {
		out = append(out, s)
	}
	return out, nil
}

// bestCut returns a byte offset at or before max that is a rune boundary.
// It prefers the last blank line, then the last newline, when that break is
// not at the start of the window.
func bestCut(s string, max int) int {
	if max > len(s) {
		max = len(s)
	}
	end := 0
	for end < len(s) {
		_, size := utf8.DecodeRuneInString(s[end:])
		if size <= 0 {
			break
		}
		if end+size > max {
			break
		}
		end += size
	}
	if end <= 0 {
		return 0
	}
	window := s[:end]
	if i := strings.LastIndex(window, "\n\n"); i > 0 {
		return i + 2
	}
	if i := strings.LastIndex(window, "\n"); i > 0 {
		return i + 1
	}
	return end
}
