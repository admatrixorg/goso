package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/pkgmgr"
	"github.com/mqglobal/goso/gateway/internal/skill"
)

// registerSkillsExtraRoutes serves skill bundles, dependency scans and
// runtime probes. Skill bodies live on disk; this never executes scripts.
func registerSkillsExtraRoutes(mux *http.ServeMux, opt Options) {
	mux.HandleFunc("GET /v1/skills/export/preview", handleSkillsExportPreview())
	mux.HandleFunc("GET /v1/skills/export", handleSkillsExport())
	mux.HandleFunc("POST /v1/skills/import", handleSkillsImport())
	mux.HandleFunc("POST /v1/skills/rescan-deps", handleSkillsRescanDeps())
	mux.HandleFunc("GET /v1/skills/runtimes", handleSkillsRuntimes(opt.Packages))
	mux.HandleFunc("POST /v1/skills/install-deps", skillInstallDisabled)
	mux.HandleFunc("POST /v1/skills/install-dep", skillInstallDisabled)
	mux.HandleFunc("POST /v1/skills/upload", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "skill archive upload is not enabled; create the skill from text")
	})
}

type skillBundleDoc struct {
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"`
	Body  string `json:"body"`
}

type skillBundle struct {
	Version    int              `json:"version"`
	Tenant     string           `json:"tenant"`
	ExportedAt string           `json:"exported_at"`
	Skills     []skillBundleDoc `json:"skills"`
}

func collectSkillBundle(tenant string, withBodies bool) skillBundle {
	b := skillBundle{Version: 1, Tenant: tenant, ExportedAt: time.Now().UTC().Format(time.RFC3339), Skills: []skillBundleDoc{}}
	infos, err := skill.List(tenant)
	if err != nil {
		return b
	}
	for _, info := range infos {
		if strings.TrimSpace(info.Name) == "" {
			continue
		}
		row := skillBundleDoc{Name: info.Name, Path: info.Path}
		if withBodies {
			if doc, lerr := skill.Load(tenant, info.Name); lerr == nil && doc != nil {
				row.Body = doc.Body
			}
		}
		b.Skills = append(b.Skills, row)
	}
	return b
}

func handleSkillsExportPreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := collectSkillBundle(requestTenant(r), false)
		bytes := 0
		for _, s := range b.Skills {
			bytes += len(s.Body)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"skills": b.Skills, "count": len(b.Skills), "bytes": bytes,
			"configured": skill.Configured(),
		})
	}
}

func handleSkillsExport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, collectSkillBundle(requestTenant(r), true))
	}
}

func handleSkillsImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body skillBundle
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid bundle")
			return
		}
		tenant := requestTenant(r)
		created, skipped := 0, 0
		failures := []string{}
		for _, row := range body.Skills {
			name := strings.TrimSpace(row.Name)
			if name == "" {
				skipped++
				continue
			}
			if _, err := skill.Create(tenant, name, row.Body); err != nil {
				if _, lerr := skill.Load(tenant, name); lerr == nil {
					skipped++
					continue
				}
				failures = append(failures, name+": "+err.Error())
				continue
			}
			created++
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": len(failures) == 0, "created": created, "skipped": skipped, "failed": failures,
		})
	}
}

func handleSkillsRuntimes(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m == nil {
			m = pkgmgr.Default()
		}
		snap := m.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"runtimes": snap.Runtimes, "packages": snap.Packages})
	}
}

func handleSkillsRescanDeps() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root := skill.Dir()
		manifests := []map[string]any{}
		if root != "" {
			_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					if strings.Count(path, string(os.PathSeparator))-strings.Count(root, string(os.PathSeparator)) > 3 {
						return filepath.SkipDir
					}
					if d.Name() == ".git" || d.Name() == "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				switch d.Name() {
				case "requirements.txt", "package.json", "pyproject.toml", "Pipfile", "go.mod":
					manifests = append(manifests, map[string]any{
						"skill":    filepath.Base(filepath.Dir(path)),
						"manifest": d.Name(),
						"path":     strings.TrimPrefix(path, root),
					})
				}
				return nil
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"scanned": len(manifests), "manifests": manifests,
			"skills_dir": root, "install_enabled": false,
		})
	}
}

func skillInstallDisabled(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusBadRequest, "dependency install is not enabled; declare manifests and install them in the image")
}
