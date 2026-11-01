package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestSessionPreview_SkipsMarkersAndCollapsesSpace(t *testing.T) {
	got := sessionPreview([]*store.Message{
		{Role: "event", Content: "Chuyển sang"},
		{Role: "tool", Content: "ignored"},
		{Role: "user", Content: "  Đại lý   Bình Dương còn nợ bao nhiêu?  "},
		{Role: "assistant", Content: "sau"},
	})
	if got != "Đại lý Bình Dương còn nợ bao nhiêu?" {
		t.Fatalf("preview %q", got)
	}
	if sessionPreview(nil) != "" {
		t.Fatal("empty")
	}
}

func TestListSessions_PreviewInsteadOfRawID(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", strings.NewReader(`{"agent_key":"tai-chinh","display_name":"Cố vấn tài chính"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent %d %s", w.Code, w.Body.String())
	}
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", strings.NewReader(`{"agent_id":"`+agent["id"].(string)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("session %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	id := sess["id"].(string)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions/"+id+"/messages", strings.NewReader(`{"role":"user","content":"Đại lý Bình Dương còn nợ bao nhiêu?"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("message %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"preview":"Đại lý Bình Dương còn nợ bao nhiêu?"`) {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"label":"`+id+`"`) {
		t.Fatalf("raw id stored as label %s", w.Body.String())
	}
}
