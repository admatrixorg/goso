// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ErrProviderNotFound is returned when a named provider is neither in the env
// registry nor in the SQLite overlay.
var ErrProviderNotFound = errors.New("provider not found")

// Resolve picks the provider for one chat request.
// Env registry wins on name clash. Empty name uses fallback (DefaultProvider /
// Runtime.LLM), then applies a non-empty model on a clone so registry
// singletons are not mutated. Named miss is an error (no silent steal).
func Resolve(st store.StoreIface, name, model string, fallback Provider) (Provider, error) {
	name = strings.TrimSpace(name)
	model = strings.TrimSpace(model)
	if name == "" {
		p := fallback
		if p == nil {
			p = NewRegistry().Preferred()
		}
		return applyModel(cloneProvider(p), model), nil
	}
	reg := NewRegistry()
	if reg.Has(name) {
		return applyModel(cloneProvider(reg.Get(name)), model), nil
	}
	if st != nil {
		row, err := st.GetLLMProvider(name)
		if err == nil && row != nil {
			key := ""
			if b, gerr := secrets.Get(st, APIKeySecretName(name)); gerr == nil {
				key = string(b)
			}
			p, berr := Build(row.Name, row.Type, row.BaseURL, row.Model, key)
			if berr != nil {
				return nil, berr
			}
			return applyModel(p, model), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
}

func cloneProvider(p Provider) Provider {
	if p == nil {
		return Echo{}
	}
	switch v := p.(type) {
	case *OpenAI:
		cp := *v
		return &cp
	case *Anthropic:
		cp := *v
		return &cp
	default:
		return p
	}
}

func applyModel(p Provider, model string) Provider {
	if model == "" || p == nil {
		return p
	}
	switch v := p.(type) {
	case *OpenAI:
		v.Model = model
	case *Anthropic:
		v.Model = model
	}
	return p
}
