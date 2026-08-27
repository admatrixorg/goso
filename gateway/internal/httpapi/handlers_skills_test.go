// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestGetSkills_EmptyEnvFailClosed(t *testing.T) {
	t.Setenv("GOSO_SKILLS_DIR", "")
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Skills []map[string]any `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Skills == nil || len(listed.Skills) != 0 {
		t.Fatalf("want empty list %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=demo", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
}

func TestGetSkills_TempDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "SKILL.md"), []byte("# Demo\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"demo"`) {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "# Demo") {
		t.Fatal("list must not include body")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=demo", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "# Demo") {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=../demo", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "path escape") {
		t.Fatalf("escape %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(`{"connector":"builtin","tool":"use_skill","arguments":{"name":"demo"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "# Demo") {
		t.Fatalf("invoke %d %s", w.Code, w.Body.String())
	}
}
