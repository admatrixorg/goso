package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// registerVaultStorageRoutes tells an operator where this tenant's files belong.
// The sync layout is <root>/t/<tenant>, which is not obvious from the flag alone.
func registerVaultStorageRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/vault/storage", handleVaultStorage(st))
}

func handleVaultStorage(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := requestTenant(r)
		svc := vaultSvc(st)
		root := ""
		if svc != nil {
			root = svc.Root
		}
		if strings.TrimSpace(root) == "" {
			root = vault.Dir()
		}
		dir, err := vault.StorageDir(root, tenant)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		files := 0
		bytes := int64(0)
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, werr error) error {
			if werr != nil || info == nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
				files++
				bytes += info.Size()
			}
			return nil
		})
		_, statErr := os.Stat(dir)
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id":      tenant,
			"root":           root,
			"tenant_dir":     dir,
			"exists":         statErr == nil,
			"markdown_files": files,
			"markdown_bytes": bytes,
			"sync_enabled":   vault.ScopedSyncEnabled(),
			"layout":         "<root>/t/<tenant>/<relative path>.md",
		})
	}
}
