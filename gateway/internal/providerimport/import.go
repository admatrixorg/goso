package providerimport

import (
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Options selects the source tenant to read and the goso tenant to write.
type Options struct {
	SourceTenant string
	DestTenant   string
	Apply        bool
	Deny         map[string]struct{}
}

// TenantCount is one source tenant tally. It never carries a key.
type TenantCount struct {
	SourceTenant string         `json:"source_tenant"`
	Input        int            `json:"input"`
	Inserted     int            `json:"inserted"`
	Updated      int            `json:"updated"`
	Skipped      int            `json:"skipped"`
	Rejected     int            `json:"rejected"`
	Reasons      map[string]int `json:"reasons,omitempty"`
}

// Report is counts only.
type Report struct {
	DryRun        bool          `json:"dry_run"`
	SourceTenant  string        `json:"source_tenant"`
	DestTenant    string        `json:"dest_tenant"`
	Tenants       []TenantCount `json:"tenants"`
	DeniedTenants int           `json:"denied_tenants"`
	NotImported   []string      `json:"not_imported,omitempty"`
}

// NotImported names what this importer deliberately leaves behind.
func NotImported() []string {
	return []string{
		"llm_providers.api_key (never read: goso keeps the key in secrets)",
		"llm_providers.settings",
		"llm_providers.display_name",
		"llm_providers.created_at",
		"llm_providers.updated_at",
	}
}

const (
	reasonTenantMismatch = "tenant-mismatch"
	reasonNativeExists   = "native-exists"
	reasonInvalid        = "invalid"
)

// Import folds every row for opt.SourceTenant into goso llm_providers.
// A second run does not overwrite a row it already wrote, so an operator
// edit in the UI survives.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	if st == nil || ex == nil {
		return nil, errConfig("export is empty")
	}
	opt.SourceTenant = strings.TrimSpace(opt.SourceTenant)
	opt.DestTenant = strings.TrimSpace(opt.DestTenant)
	if opt.SourceTenant == "" || !plainToken(opt.SourceTenant) {
		return nil, errConfig("source tenant is required")
	}
	if !store.TenantOK(opt.DestTenant) {
		return nil, errConfig("dest tenant is invalid")
	}
	if denied(opt.Deny, opt.SourceTenant) || denied(opt.Deny, opt.DestTenant) {
		return nil, errConfig("tenant is denied")
	}

	sec := TenantCount{SourceTenant: opt.SourceTenant, Reasons: map[string]int{}}
	existing := map[string]struct{}{}
	for _, p := range st.ListLLMProviders() {
		if p == nil {
			continue
		}
		existing[p.Name] = struct{}{}
	}

	for _, row := range ex.Rows {
		sec.Input++
		if strings.TrimSpace(row.TenantID) != opt.SourceTenant {
			sec.Skipped++
			sec.Reasons[reasonTenantMismatch]++
			continue
		}
		name, kind, base, ok := normalize(row)
		if !ok {
			sec.Rejected++
			sec.Reasons[reasonInvalid]++
			continue
		}
		if _, dup := existing[name]; dup {
			sec.Skipped++
			sec.Reasons[reasonNativeExists]++
			continue
		}
		if !opt.Apply {
			sec.Inserted++
			existing[name] = struct{}{}
			continue
		}
		if _, err := st.CreateLLMProvider(store.LLMProvider{
			Name:     name,
			TenantID: opt.DestTenant,
			Type:     kind,
			BaseURL:  base,
			Model:    DefaultModel(kind),
			Enabled:  row.Enabled == nil || *row.Enabled,
		}); err != nil {
			sec.Rejected++
			sec.Reasons[reasonInvalid]++
			continue
		}
		sec.Inserted++
		existing[name] = struct{}{}
	}

	rep := &Report{
		DryRun:       !opt.Apply,
		SourceTenant: opt.SourceTenant,
		DestTenant:   opt.DestTenant,
		Tenants:      []TenantCount{sec},
		NotImported:  NotImported(),
	}
	if opt.Apply {
		rep.NotImported = append(rep.NotImported, "an existing goso row is not overwritten")
	}
	return rep, nil
}

// normalize maps one GoClaw row onto a goso provider row. GoClaw
// provider_type is not a goso type, so the type comes from the shape of
// the row: a known goso type is kept, anything with a base URL is
// openai-compat, and the rest is the echo provider.
func normalize(row Row) (name, kind, base string, ok bool) {
	name = strings.TrimSpace(row.Name)
	if name == "" {
		name = strings.TrimSpace(row.DisplayName)
	}
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
		return "", "", "", false
	}
	kind = strings.ToLower(strings.TrimSpace(row.ProviderType))
	switch kind {
	case "openai-compat", "anthropic", "echo", "router9":
	default:
		if strings.TrimSpace(row.APIBase) != "" {
			kind = "openai-compat"
		} else {
			kind = "echo"
		}
	}
	base = strings.TrimSpace(row.APIBase)
	if strings.ContainsAny(base, "\r\n\x00") {
		return "", "", "", false
	}
	return name, kind, base, true
}

// DefaultModel is the model goso addresses when GoClaw stored none.
func DefaultModel(kind string) string {
	switch kind {
	case "anthropic":
		return "claude-sonnet-4"
	case "echo":
		return "echo"
	default:
		return "gpt-4o-mini"
	}
}

func denied(set map[string]struct{}, id string) bool {
	if len(set) == 0 {
		return false
	}
	_, ok := set[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

func plainToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '/' || r == '\\' || r == ' ' {
			return false
		}
	}
	return true
}
