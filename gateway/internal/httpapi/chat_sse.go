// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func chatWantsStream(r *http.Request, body chatBody) bool {
	if body.Stream {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
}

type sseWriter struct {
	w       http.ResponseWriter
	f       http.Flusher
	started bool
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	f, _ := w.(http.Flusher)
	return &sseWriter{w: w, f: f}
}

func (s *sseWriter) start() {
	if s.started {
		return
	}
	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache")
	s.w.WriteHeader(http.StatusOK)
	s.started = true
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseWriter) data(payload string) {
	s.start()
	fmt.Fprintf(s.w, "data: %s\n\n", payload)
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseWriter) event(name, payload string) {
	s.start()
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, payload)
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseWriter) comment(text string) {
	s.start()
	fmt.Fprintf(s.w, ": %s\n\n", text)
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseWriter) idEvent(id, name, payload string) {
	s.start()
	if id != "" {
		fmt.Fprintf(s.w, "id: %s\n", id)
	}
	if name != "" {
		fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, payload)
	} else {
		fmt.Fprintf(s.w, "data: %s\n\n", payload)
	}
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseWriter) delta(text string) {
	if text == "" {
		return
	}
	payload, err := json.Marshal(map[string]string{"delta": text})
	if err != nil {
		return
	}
	s.data(string(payload))
}

func (s *sseWriter) errEvent(msg string) {
	payload, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		s.event("error", `{"error":"stream error"}`)
		return
	}
	s.event("error", string(payload))
}

func writeChatSSE(w http.ResponseWriter, reply string) {
	sw := newSSEWriter(w)
	sw.delta(reply)
	sw.data("[DONE]")
}

func writeChatSSEError(w http.ResponseWriter, msg string) {
	sw := newSSEWriter(w)
	sw.errEvent(msg)
}

func respondChat(w http.ResponseWriter, r *http.Request, body chatBody, reply string, jsonOK any, err error, errStatus int) {
	stream := chatWantsStream(r, body)
	if err != nil {
		if stream {
			writeChatSSEError(w, err.Error())
			return
		}
		writeErr(w, errStatus, err.Error())
		return
	}
	if stream {
		writeChatSSE(w, reply)
		return
	}
	writeJSON(w, http.StatusOK, jsonOK)
}
