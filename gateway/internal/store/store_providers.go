// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
)

func cloneLLMProvider(p *LLMProvider) *LLMProvider {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

func (s *Store) CreateLLMProvider(p LLMProvider) (*LLMProvider, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.TrimSpace(p.Type)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.Model = strings.TrimSpace(p.Model)
	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	if p.Type == "" {
		return nil, errors.New("type is required")
	}
	p.TenantID = NormalizeTenant(p.TenantID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.llmProviders[p.Name]; ok {
		return nil, ErrExists
	}
	cp := p
	s.llmProviders[cp.Name] = &cp
	return cloneLLMProvider(&cp), nil
}

func (s *Store) ListLLMProviders() []*LLMProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*LLMProvider, 0, len(s.llmProviders))
	for _, v := range s.llmProviders {
		out = append(out, cloneLLMProvider(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) GetLLMProvider(name string) (*LLMProvider, error) {
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.llmProviders[name]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneLLMProvider(v), nil
}

func (s *Store) UpdateLLMProvider(p LLMProvider) (*LLMProvider, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.llmProviders[p.Name]
	if !ok {
		return nil, ErrNotFound
	}
	if t := strings.TrimSpace(p.Type); t != "" {
		cur.Type = t
	}
	cur.BaseURL = strings.TrimSpace(p.BaseURL)
	cur.Model = strings.TrimSpace(p.Model)
	cur.Enabled = p.Enabled
	return cloneLLMProvider(cur), nil
}
