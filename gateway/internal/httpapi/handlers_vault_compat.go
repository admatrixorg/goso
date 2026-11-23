package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// goClawVaultDocView adds the field names the GoClaw control plane reads onto
// a Goso vault document. Extra keys are kept so older callers still work.
func goClawVaultDocView(doc map[string]any) map[string]any {
	if doc == nil {
		return nil
	}
	out := make(map[string]any, len(doc)+8)
	for k, v := range doc {
		out[k] = v
	}
	if out["content_hash"] == nil {
		out["content_hash"] = out["sha256"]
	}
	if out["doc_type"] == nil || out["doc_type"] == "" {
		out["doc_type"] = "document"
	}
	if p, ok := out["path"].(string); ok {
		out["path_basename"] = path.Base(p)
	}
	if out["created_at"] == nil {
		out["created_at"] = out["mtime"]
	}
	if out["updated_at"] == nil {
		out["updated_at"] = out["mtime"]
	}
	if _, ok := out["metadata"]; !ok {
		out["metadata"] = nil
	}
	switch out["scope"] {
	case "tenant":
		out["scope"] = "shared"
	case "agent":
		out["scope"] = "personal"
	}
	return out
}

func goClawVaultDocList(raw any) []any {
	rows, ok := raw.([]any)
	if !ok {
		return []any{}
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, goClawVaultDocView(m))
			continue
		}
		out = append(out, row)
	}
	return out
}

// handleGoClawVaultDocGet wraps the vault get handler and remaps the document.
func handleGoClawVaultDocGet(st store.StoreIface) http.HandlerFunc {
	inner := handleGetVaultDoc(st)
	return func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner(rec, r)
		if rec.Code != http.StatusOK {
			w.Header().Set("Content-Type", rec.Header().Get("Content-Type"))
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeJSON(w, http.StatusOK, goClawVaultDocView(doc))
	}
}

type goClawVaultLink struct {
	ID        string `json:"id"`
	FromDocID string `json:"from_doc_id"`
	ToDocID   string `json:"to_doc_id,omitempty"`
	LinkType  string `json:"link_type"`
	Context   string `json:"context"`
	Path      string `json:"path,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
}

// handleGoClawVaultLinks answers GET /v1/vault/documents/{id}/links with the
// outlinks/backlinks/doc_names shape the vault detail dialog reads.
func handleGoClawVaultLinks(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		doc, err := st.GetVaultDoc(id)
		if err != nil || doc == nil || !store.SameTenant(doc.TenantID, requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		outbound, inbound, lerr := st.ListVaultDocLinks(id)
		if lerr != nil {
			writeErr(w, http.StatusBadRequest, lerr.Error())
			return
		}
		names := map[string]string{}
		nameFor := func(docID string) (string, string) {
			if docID == "" {
				return "", ""
			}
			if d, err := st.GetVaultDoc(docID); err == nil && d != nil {
				names[docID] = d.Title
				return d.Title, d.Path
			}
			return "", ""
		}
		outlinks := make([]goClawVaultLink, 0, len(outbound))
		for _, l := range outbound {
			if l.ToID == "" || l.FromID != id {
				continue
			}
			title, p := nameFor(l.ToID)
			outlinks = append(outlinks, goClawVaultLink{
				ID: l.FromID + "->" + l.ToID, FromDocID: l.FromID, ToDocID: l.ToID,
				LinkType: "wikilink", Context: l.Raw, Path: p, Title: title,
			})
		}
		backlinks := make([]goClawVaultLink, 0, len(inbound))
		for _, l := range inbound {
			if l.ToID != id || l.FromID == "" {
				continue
			}
			title, p := nameFor(l.FromID)
			backlinks = append(backlinks, goClawVaultLink{
				ID: l.FromID + "->" + l.ToID, FromDocID: l.FromID, ToDocID: id,
				LinkType: "wikilink", Context: l.Raw, Path: p, Title: title,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"outlinks": outlinks, "backlinks": backlinks, "doc_names": names,
		})
	}
}
