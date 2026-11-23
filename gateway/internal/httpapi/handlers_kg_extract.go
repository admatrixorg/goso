package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// kgExtractPrompt asks for the shape the importer writes, so an extracted entity
// lands in the same graph as an imported one.
const kgExtractPrompt = `You extract a knowledge graph from text.
Return only JSON, no prose, in this shape:
{"entities":[{"name":"...","kind":"person|org|product|topic|place","body":"one short sentence"}],
 "relations":[{"from":"entity name","to":"entity name","rel":"relationship"}]}
Rules: entity names are short and stable, reuse an existing name instead of a variant,
kind comes from the list, relations only reference entities you returned.`

// registerKGExtractRoutes serves text to graph extraction.
func registerKGExtractRoutes(mux *http.ServeMux, st store.StoreIface, provider llm.Provider) {
	mux.HandleFunc("POST /v1/kg/extract", handleKGExtract(st, provider))
}

type kgExtractBody struct {
	Text          string  `json:"text"`
	MinConfidence float64 `json:"min_confidence"`
	DryRun        bool    `json:"dry_run"`
}

type kgExtractEntity struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Body string `json:"body"`
}

type kgExtractRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rel  string `json:"rel"`
}

type kgExtraction struct {
	Entities  []kgExtractEntity   `json:"entities"`
	Relations []kgExtractRelation `json:"relations"`
}

func handleKGExtract(st store.StoreIface, provider llm.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body kgExtractBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		text := strings.TrimSpace(body.Text)
		if text == "" {
			writeErr(w, http.StatusBadRequest, "text is required")
			return
		}
		if provider == nil {
			writeErr(w, http.StatusNotImplemented, "no language model provider is configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		// One user message carries the instruction: some providers drop the system
		// role, and a chat model that only sees the text will reply in prose.
		prompt := kgExtractPrompt + "\n\nTEXT:\n" + text + "\n\nJSON only."
		reply, err := provider.Chat(ctx, []llm.Message{
			{Role: "user", Content: prompt},
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok": false, "error": "model call failed: " + err.Error(),
			})
			return
		}
		parsed, perr := parseKGExtraction(reply)
		if perr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok": false, "error": perr.Error(), "raw": clipText(reply, 400),
			})
			return
		}
		res := map[string]any{
			"ok": true, "dry_run": body.DryRun,
			"entities_found": len(parsed.Entities), "relations_found": len(parsed.Relations),
			"entities": parsed.Entities, "relations": parsed.Relations,
		}
		if body.DryRun {
			writeJSON(w, http.StatusOK, res)
			return
		}
		tenant := requestTenant(r)
		ids := map[string]string{}
		written := 0
		for _, e := range parsed.Entities {
			name := strings.TrimSpace(e.Name)
			if name == "" {
				continue
			}
			saved, serr := st.PutKGEntity(store.KGEntity{
				TenantID: tenant, Name: name, Kind: firstNonEmpty(e.Kind, "topic"),
				Body: strings.TrimSpace(e.Body), Source: "extract",
				ValidFrom: time.Now().UTC(), CreatedAt: time.Now().UTC(),
			})
			if serr != nil || saved == nil {
				continue
			}
			ids[strings.ToLower(name)] = saved.ID
			written++
		}
		edges := 0
		for _, rel := range parsed.Relations {
			from := ids[strings.ToLower(strings.TrimSpace(rel.From))]
			to := ids[strings.ToLower(strings.TrimSpace(rel.To))]
			if from == "" || to == "" || from == to {
				continue
			}
			if _, rerr := st.PutKGRelation(store.KGRelation{
				TenantID: tenant, FromID: from, ToID: to,
				Rel: firstNonEmpty(rel.Rel, "related_to"), Source: "extract",
				ValidFrom: time.Now().UTC(),
			}); rerr == nil {
				edges++
			}
		}
		res["entities_written"] = written
		res["relations_written"] = edges
		writeJSON(w, http.StatusOK, res)
	}
}

// parseKGExtraction tolerates a model that wraps the JSON in prose or a fence.
func parseKGExtraction(reply string) (*kgExtraction, error) {
	text := strings.TrimSpace(reply)
	if i := strings.Index(text, "{"); i >= 0 {
		text = text[i:]
	}
	if j := strings.LastIndex(text, "}"); j >= 0 {
		text = text[:j+1]
	}
	var out kgExtraction
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	if len(out.Entities) == 0 {
		return nil, errNoEntities
	}
	return &out, nil
}

var errNoEntities = &kgExtractError{"model returned no entities"}

type kgExtractError struct{ msg string }

func (e *kgExtractError) Error() string { return e.msg }

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
