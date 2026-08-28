// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
)

func (s *SQLiteStore) CreateLLMProvider(p LLMProvider) (*LLMProvider, error) {
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
	_, err := s.db.Exec(
		`INSERT INTO llm_providers(name, type, base_url, model) VALUES(?,?,?,?)`,
		p.Name, p.Type, p.BaseURL, p.Model,
	)
	if err != nil {
		return nil, ErrExists
	}
	cp := p
	return cloneLLMProvider(&cp), nil
}

func (s *SQLiteStore) ListLLMProviders() []*LLMProvider {
	rows, err := s.db.Query(`SELECT name, type, base_url, model FROM llm_providers ORDER BY name`)
	if err != nil {
		return []*LLMProvider{}
	}
	defer rows.Close()
	out := make([]*LLMProvider, 0)
	for rows.Next() {
		p, err := scanLLMProvider(rows)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *SQLiteStore) GetLLMProvider(name string) (*LLMProvider, error) {
	name = strings.TrimSpace(name)
	row := s.db.QueryRow(`SELECT name, type, base_url, model FROM llm_providers WHERE name=?`, name)
	p, err := scanLLMProvider(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *SQLiteStore) UpdateLLMProvider(p LLMProvider) (*LLMProvider, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	cur, err := s.GetLLMProvider(p.Name)
	if err != nil {
		return nil, err
	}
	if t := strings.TrimSpace(p.Type); t != "" {
		cur.Type = t
	}
	cur.BaseURL = strings.TrimSpace(p.BaseURL)
	cur.Model = strings.TrimSpace(p.Model)
	res, err := s.db.Exec(
		`UPDATE llm_providers SET type=?, base_url=?, model=? WHERE name=?`,
		cur.Type, cur.BaseURL, cur.Model, cur.Name,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return cloneLLMProvider(cur), nil
}

func scanLLMProvider(sc scanner) (*LLMProvider, error) {
	var p LLMProvider
	if err := sc.Scan(&p.Name, &p.Type, &p.BaseURL, &p.Model); err != nil {
		return nil, err
	}
	return cloneLLMProvider(&p), nil
}
