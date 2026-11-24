package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// slugRe keeps an instance slug to the characters a channel name may carry.
var slugRe = regexp.MustCompile(`[^a-z0-9._-]+`)

// The instance writes live under /v1/channel-instances because /v1/channels/{name}/secrets
// would make /v1/channels/instances/{id} ambiguous in the router. Without these routes
// create, update and delete answer 405 and the page can only report a failure toast.
func registerChannelInstanceWriteRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/channel-instances", handleChannelInstanceList(st))
	mux.HandleFunc("GET /v1/channel-instances/{tenant}/{kind}/{slug}", handleChannelInstanceGet(st))
	mux.HandleFunc("POST /v1/channel-instances", handleChannelInstanceCreate(st))
	mux.HandleFunc("PUT /v1/channel-instances/{tenant}/{kind}/{slug}", handleChannelInstanceUpdate(st))
	mux.HandleFunc("DELETE /v1/channel-instances/{tenant}/{kind}/{slug}", handleChannelInstanceDelete(st))
	mux.HandleFunc("GET /v1/channel-instances/{tenant}/{kind}/{slug}/writers", handleChannelInstanceWriters(st))
	mux.HandleFunc("GET /v1/channel-instances/{tenant}/{kind}/{slug}/writers/groups", handleChannelInstanceWriterGroups(st))
}

type channelInstanceInput struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name"`
	ChannelType string         `json:"channel_type"`
	Kind        string         `json:"kind"`
	AgentID     string         `json:"agent_id"`
	Config      map[string]any `json:"config"`
	Credentials map[string]any `json:"credentials"`
	Enabled     *bool          `json:"enabled"`
}

// normalizeKind accepts the underscored ids the control plane uses and the
// hyphenated names the channel catalog publishes.
func normalizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	kind = strings.ReplaceAll(kind, "_", "-")
	return kind
}

func knownChannelKind(kind string) bool {
	for _, name := range channel.Names {
		if name == kind {
			return true
		}
	}
	return false
}

// instanceName builds tenant/kind/slug, the shape every stored instance uses.
func instanceName(tenant, kind, raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slugRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-._")
	if slug == "" {
		slug = "instance-" + strconvUnix()
	}
	if strings.HasPrefix(slug, tenant+"/") {
		return slug
	}
	if strings.Count(slug, "/") >= 2 {
		return slug
	}
	return tenant + "/" + kind + "/" + slug
}

func strconvUnix() string {
	return time.Now().UTC().Format("150405")
}

func mapToStrings(in map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		switch val := v.(type) {
		case string:
			out[k] = val
		case bool:
			if val {
				out[k] = "true"
			} else {
				out[k] = "false"
			}
		case float64:
			out[k] = jsonNumber(val)
		case nil:
		default:
			raw, err := json.Marshal(val)
			if err == nil {
				out[k] = string(raw)
			}
		}
	}
	return out
}

func jsonNumber(f float64) string {
	if f == float64(int64(f)) {
		return itoaInt(int64(f))
	}
	raw, _ := json.Marshal(f)
	return string(raw)
}

func itoaInt(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// The channels page reads and writes one prefix, so list and detail live here
// too and share the view the older paths already return.
func handleChannelInstanceList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := requestTenant(r)
		out := []map[string]any{}
		if st != nil {
			for _, cfg := range st.ListChannelConfigs() {
				if cfg == nil {
					continue
				}
				if strings.Contains(cfg.Name, "/") {
					owner := strings.SplitN(cfg.Name, "/", 2)[0]
					if !store.SameTenant(owner, tenant) && tenant != store.DefaultTenant && tenant != "master" {
						continue
					}
				}
				out = append(out, channelInstanceView(cfg))
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"instances": out, "total": len(out)})
	}
}

func handleChannelInstanceGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		cfg, err := st.GetChannelConfig(id)
		if err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		writeJSON(w, http.StatusOK, channelInstanceView(cfg))
	}
}

func handleChannelInstanceCreate(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusServiceUnavailable, "store unavailable")
			return
		}
		var body channelInstanceInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		kind := strings.TrimSpace(body.ChannelType)
		if kind == "" {
			kind = strings.TrimSpace(body.Kind)
		}
		kind = normalizeKind(kind)
		if !knownChannelKind(kind) {
			writeErr(w, http.StatusBadRequest, "unknown channel_type \""+kind+"\"")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, http.StatusBadRequest, "name is required")
			return
		}
		tenant := requestTenant(r)
		name := instanceName(tenant, kind, body.Name)
		if existing, err := st.GetChannelConfig(name); err == nil && existing != nil {
			writeErr(w, http.StatusConflict, "an instance with that name already exists")
			return
		}
		agentID := strings.TrimSpace(body.AgentID)
		if agentID != "" {
			resolved, aerr := agentVisible(st, agentID, tenant)
			if aerr != nil || resolved == nil {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			agentID = resolved.ID
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		cfg := store.ChannelConfig{
			Name: name, Enabled: enabled, AgentID: agentID,
			DMPolicy: "pairing", GroupPolicy: "allowlist", RequireMention: true,
			Settings: mapToStrings(body.Config), UpdatedAt: time.Now().UTC(),
		}
		if err := st.PutChannelConfig(cfg); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if kind == "zalo-personal" && len(body.Credentials) > 0 {
			if raw, cerr := json.Marshal(body.Credentials); cerr == nil {
				if cred, perr := protocol.ParseCredential(string(raw)); perr == nil {
					encoded, _ := json.Marshal(cred)
					_ = secrets.Put(st, zaloCookieSecretName(name), encoded)
				}
			}
		}
		view := channelInstanceView(&cfg)
		view["id"] = name
		writeJSON(w, http.StatusCreated, view)
	}
}

// instanceIDFromPath rebuilds the tenant/kind/slug id from the path segments.
func instanceIDFromPath(r *http.Request) string {
	tenant := strings.TrimSpace(r.PathValue("tenant"))
	kind := strings.TrimSpace(r.PathValue("kind"))
	slug := strings.TrimSpace(r.PathValue("slug"))
	if tenant == "" || kind == "" || slug == "" {
		return ""
	}
	return tenant + "/" + kind + "/" + slug
}

func handleChannelInstanceUpdate(st store.StoreIface) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusServiceUnavailable, "store unavailable")
			return
		}
		id := instanceIDFromPath(r)
		cfg, err := st.GetChannelConfig(id)
		if err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if !store.SameTenant(strings.SplitN(cfg.Name, "/", 2)[0], requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		var body channelInstanceInput
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Enabled != nil {
			cfg.Enabled = *body.Enabled
		}
		if strings.TrimSpace(body.AgentID) != "" {
			if resolved, aerr := agentVisible(st, body.AgentID, requestTenant(r)); aerr == nil && resolved != nil {
				cfg.AgentID = resolved.ID
			}
		}
		if body.Config != nil {
			cfg.Settings = mapToStrings(body.Config)
		}
		cfg.UpdatedAt = time.Now().UTC()
		if perr := st.PutChannelConfig(*cfg); perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, channelInstanceView(cfg))
	}
}

func handleChannelInstanceDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusServiceUnavailable, "store unavailable")
			return
		}
		id := instanceIDFromPath(r)
		cfg, err := st.GetChannelConfig(id)
		if err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if !store.SameTenant(strings.SplitN(cfg.Name, "/", 2)[0], requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if derr := st.DeleteChannelConfig(id); derr != nil {
			writeErr(w, http.StatusBadRequest, derr.Error())
			return
		}
		_ = secrets.Delete(st, zaloCookieSecretName(id))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	}
}

func handleChannelInstanceWriters(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"writers": []any{}, "total": 0})
	}
}

func handleChannelInstanceWriterGroups(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"groups": []any{}, "total": 0})
	}
}
