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
	reply, u, err := ReadOpenAIStreamDeltas(r, nil)
	return reply.Text, u, err
}

type openAIToolAcc struct {
	id, name, args string
}

// ReadOpenAIStreamDeltas parses upstream SSE as bytes arrive and calls onDelta
// for each choices[0].delta.content fragment.
func ReadOpenAIStreamDeltas(r io.Reader, onDelta StreamHandler) (Reply, Usage, error) {
	var b strings.Builder
	var u Usage
	acc := map[int]*openAIToolAcc{}
	err := ParseSSE(r, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
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
			if c.Delta.Content != "" {
				b.WriteString(c.Delta.Content)
				emitDelta(onDelta, c.Delta.Content)
			}
			for _, tc := range c.Delta.ToolCalls {
				a := acc[tc.Index]
				if a == nil {
					a = &openAIToolAcc{}
					acc[tc.Index] = a
				}
				if tc.ID != "" {
					a.id = tc.ID
				}
				if tc.Function.Name != "" {
					a.name = tc.Function.Name
				}
				a.args += tc.Function.Arguments
			}
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
		return Reply{}, Usage{}, err
	}
	reply := Reply{Text: b.String()}
	if len(acc) > 0 {
		max := -1
		for i := range acc {
			if i > max {
				max = i
			}
		}
		for i := 0; i <= max; i++ {
			a := acc[i]
			if a == nil {
				continue
			}
			args := map[string]any{}
			if a.args != "" {
				_ = json.Unmarshal([]byte(a.args), &args)
			}
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				ID:        a.id,
				Name:      a.name,
				Arguments: args,
			})
		}
	}
	return reply, u, nil
}

// ReadAnthropicStreamDeltas parses native Anthropic SSE content_block_delta text.
func ReadAnthropicStreamDeltas(r io.Reader, onDelta StreamHandler) (string, Usage, error) {
	var b strings.Builder
	var u Usage
	err := ParseSSE(r, func(event, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				InputTokens          int `json:"input_tokens"`
				OutputTokens         int `json:"output_tokens"`
				CacheReadInputTokens int `json:"cache_read_input_tokens"`
				CacheReadTokens      int `json:"cache_read_tokens"`
			} `json:"usage"`
			Message struct {
				Usage struct {
					InputTokens          int `json:"input_tokens"`
					OutputTokens         int `json:"output_tokens"`
					CacheReadInputTokens int `json:"cache_read_input_tokens"`
					CacheReadTokens      int `json:"cache_read_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		kind := chunk.Type
		if kind == "" {
			kind = event
		}
		switch kind {
		case "content_block_delta":
			if chunk.Delta.Text != "" {
				b.WriteString(chunk.Delta.Text)
				emitDelta(onDelta, chunk.Delta.Text)
			}
		case "message_start":
			applyAnthropicUsage(&u, chunk.Message.Usage.InputTokens, chunk.Message.Usage.OutputTokens, chunk.Message.Usage.CacheReadInputTokens, chunk.Message.Usage.CacheReadTokens)
		case "message_delta":
			applyAnthropicUsage(&u, chunk.Usage.InputTokens, chunk.Usage.OutputTokens, chunk.Usage.CacheReadInputTokens, chunk.Usage.CacheReadTokens)
		}
		return nil
	})
	if err != nil {
		return "", Usage{}, err
	}
	return b.String(), u, nil
}

func applyAnthropicUsage(u *Usage, in, out, cacheRead, cacheReadAlt int) {
	if in > 0 {
		u.PromptTokens = in
	}
	if out > 0 {
		u.CompletionTokens = out
	}
	if cacheRead > 0 {
		u.CacheReadTokens = cacheRead
	} else if cacheReadAlt > 0 {
		u.CacheReadTokens = cacheReadAlt
	}
}
