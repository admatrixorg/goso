// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ParseSSE reads a text/event-stream body and calls handle for each event.
// handle receives the event name (may be empty) and data payload.
func ParseSSE(r io.Reader, handle func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var event, data strings.Builder
	flush := func() error {
		d := strings.TrimRight(data.String(), "\n")
		e := strings.TrimSpace(event.String())
		event.Reset()
		data.Reset()
		if d == "" && e == "" {
			return nil
		}
		return handle(e, d)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "event:"); ok {
			event.Reset()
			event.WriteString(strings.TrimSpace(rest))
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(rest))
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return scanner.Err()
}

// ReadOpenAIStream concatenates delta.content from an OpenAI-compat SSE body.
func ReadOpenAIStream(r io.Reader) (string, Usage, error) {
	var b strings.Builder
	var u Usage
	err := ParseSSE(r, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		for _, c := range chunk.Choices {
			b.WriteString(c.Delta.Content)
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			u = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
			}
		}
		return nil
	})
	if err != nil {
		return "", Usage{}, err
	}
	return b.String(), u, nil
}
