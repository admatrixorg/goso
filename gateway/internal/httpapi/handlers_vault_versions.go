package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerVaultVersionRoutes serves document history: the list, one snapshot and
// a restore that writes the old body back through the normal update path.
func registerVaultVersionRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/vault/documents/{id}/versions", handleVaultVersionList(st))
	mux.HandleFunc("GET /v1/vault/documents/{id}/versions/{version}", handleVaultVersionGet(st))
	mux.HandleFunc("POST /v1/vault/documents/{id}/versions/{version}/restore", handleVaultVersionRestore(st))
}

// snapshotVaultDoc writes the current body as a version before an edit lands.
func snapshotVaultDoc(st store.StoreIface, tenant, docID, title, body, changedBy string) {
	x := store.VaultVersions(st)
	if x == nil || strings.TrimSpace(docID) == "" {
		return
	}
	_, _ = x.InsertVaultVersion(store.VaultVersion{
		TenantID: tenant, DocID: docID, Title: title, Body: body, ChangedBy: changedBy,
	})
}

func handleVaultVersionList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.VaultVersions(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"versions": []any{}, "total": 0})
			return
		}
		rows := x.ListVaultVersions(requestTenant(r), r.PathValue("id"))
		out := make([]map[string]any, 0, len(rows))
		for _, v := range rows {
			out = append(out, map[string]any{
				"id": v.ID, "doc_id": v.DocID, "version": v.Version, "title": v.Title,
				"changed_by": v.ChangedBy, "created_at": v.CreatedAt,
				"body_bytes": len(v.Body), "scope": v.CustomScope,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": out, "total": len(out)})
	}
}

func handleVaultVersionGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.VaultVersions(st)
		if x == nil {
			writeErr(w, http.StatusNotFound, "version history unavailable")
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(r.PathValue("version")))
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "version must be a positive number")
			return
		}
		v, gerr := x.GetVaultVersion(requestTenant(r), r.PathValue("id"), n)
		if gerr != nil || v == nil {
			writeErr(w, http.StatusNotFound, "version not found")
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func handleVaultVersionRestore(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.VaultVersions(st)
		if x == nil {
			writeErr(w, http.StatusNotFound, "version history unavailable")
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(r.PathValue("version")))
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "version must be a positive number")
			return
		}
		docID := strings.TrimSpace(r.PathValue("id"))
		v, gerr := x.GetVaultVersion(requestTenant(r), docID, n)
		if gerr != nil || v == nil {
			writeErr(w, http.StatusNotFound, "version not found")
			return
		}
		doc, derr := st.GetVaultDoc(docID)
		if derr != nil || doc == nil || !store.SameTenant(doc.TenantID, requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "document not found")
			return
		}
		// Snapshot the body being replaced so a restore is itself undoable.
		snapshotVaultDoc(st, requestTenant(r), docID, doc.Title, doc.Body, "restore")
		doc.Body = v.Body
		if strings.TrimSpace(v.Title) != "" {
			doc.Title = v.Title
		}
		updated, perr := st.PutVaultDoc(*doc)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "restored_version": n, "document": updated,
		})
	}
}
