// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// A channel instance is one bot owned by one tenant. Its name is
// "<tenant>/<catalog>/<instance>", for example "demo/telegram/minh-an-bot". The
// name is the channel_config key, the secret prefix and the health key, so two
// tenants can each run their own bot of the same catalog. A bare catalog name
// such as "telegram" is the older single bot: it keeps its env tokens and has
// no tenant.

// maxInstanceLen matches the importer's limit on GoClaw instance names.
const maxInstanceLen = 100

// ParseInstance splits an instance name. ok is false for a bare catalog name
// or any malformed name.
func ParseInstance(name string) (tenant, catalog, inst string, ok bool) {
	parts := strings.Split(strings.TrimSpace(name), "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	tenant, catalog, inst = parts[0], parts[1], parts[2]
	if !store.TenantOK(tenant) || !Known(catalog) || !instanceNameOK(inst) {
		return "", "", "", false
	}
	return tenant, catalog, inst, true
}

// InstanceName builds "<tenant>/<catalog>/<instance>".
func InstanceName(tenant, catalog, inst string) string {
	return tenant + "/" + catalog + "/" + inst
}

// KnownName reports a catalog name or a well-formed instance name.
func KnownName(name string) bool {
	if Known(name) {
		return true
	}
	_, _, _, ok := ParseInstance(name)
	return ok
}

// CatalogOf returns the catalog of a catalog or instance name, or "".
func CatalogOf(name string) string {
	if Known(name) {
		return name
	}
	if _, catalog, _, ok := ParseInstance(name); ok {
		return catalog
	}
	return ""
}

// TenantOf returns the tenant of an instance name. A bare catalog name has none.
func TenantOf(name string) (string, bool) {
	tenant, _, _, ok := ParseInstance(name)
	return tenant, ok
}

func instanceNameOK(s string) bool {
	if s == "" || len(s) > maxInstanceLen || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r == '%' || r == '?' || r == '#' || unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// InstanceNames lists the instance rows of one tenant, sorted.
func InstanceNames(st store.StoreIface, tenant string) []string {
	if st == nil {
		return nil
	}
	var out []string
	for _, cfg := range st.ListChannelConfigs() {
		if cfg == nil {
			continue
		}
		t, _, _, ok := ParseInstance(cfg.Name)
		if ok && store.SameTenant(t, tenant) {
			out = append(out, cfg.Name)
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// InstanceInfo is the GET /api/channels row for one instance. Only Telegram
// instances run live today; the other catalogs show their stored settings and
// health "parked".
func InstanceInfo(st store.StoreIface, mgr *Manager, name string) Info {
	catalog := CatalogOf(name)
	row := Info{Name: name, Writable: WritableFields(catalog), Phase: phaseOf(catalog), Transport: defaultTransport(catalog)}
	switch catalog {
	case "telegram":
		_, _, row.SecretSet = Credential(st, name, KindBot, nil)
	case "zalo-oa":
		_, _, a := Credential(st, name, KindAccess, nil)
		_, _, s := Credential(st, name, KindAppSecret, nil)
		row.SecretSet = a && s
	}
	row.Configured = row.SecretSet
	row.Missing = !row.SecretSet
	switch {
	case catalog != "telegram":
		row.Health = "parked"
	case store.LiteEnabled():
		row.Health = "stopped"
		if row.Missing {
			row.Health = "missing"
		}
	case row.Missing:
		row.Health = "missing"
	case mgr != nil && mgr.Running(name):
		row.Health = "running"
	case mgr != nil && mgr.Failed(name):
		row.Health = "failed"
	default:
		row.Health = "stopped"
	}
	if mgr != nil {
		if e := mgr.LastError(name); e != "" {
			row.LastError = redactErr(e)
		}
		if t := mgr.Transport(name); t != "" {
			row.Transport = t
		}
	}
	return row
}
