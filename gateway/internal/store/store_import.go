// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "strings"

// ImportSource is one source-id mapping. Rollback deletes these rows only.
type ImportSource struct {
	TenantID string
	Kind     string
	SourceID string
	GosoID   string
}

func importSourceKey(tenant, kind, sourceID string) string {
	return strings.TrimSpace(tenant) + "\x00" + strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(sourceID)
}

func importSourceOK(tenant, kind, sourceID, gosoID string) error {
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(kind) == "" || strings.TrimSpace(sourceID) == "" {
		return ErrNotFound
	}
	if strings.TrimSpace(gosoID) == "" && gosoID != "" {
		return ErrNotFound
	}
	return nil
}

// GetImportSource returns the goso id already stored for this triple.
func (s *Store) GetImportSource(tenant, kind, sourceID string) (string, bool, error) {
	if err := importSourceOK(tenant, kind, sourceID, "x"); err != nil {
		return "", false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.importSources[importSourceKey(tenant, kind, sourceID)]
	return id, ok, nil
}

// PutImportSource inserts a mapping. An existing triple is left unchanged.
func (s *Store) PutImportSource(tenant, kind, sourceID, gosoID string) error {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	sourceID = strings.TrimSpace(sourceID)
	gosoID = strings.TrimSpace(gosoID)
	if tenant == "" || kind == "" || sourceID == "" || gosoID == "" {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.importSources == nil {
		s.importSources = map[string]string{}
	}
	key := importSourceKey(tenant, kind, sourceID)
	if _, ok := s.importSources[key]; ok {
		return ErrExists
	}
	s.importSources[key] = gosoID
	return nil
}

// ListImportSources returns mappings for this exact tenant id and kind.
func (s *Store) ListImportSources(tenant, kind string) ([]ImportSource, error) {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	if tenant == "" || kind == "" {
		return nil, ErrNotFound
	}
	prefix := importSourceKey(tenant, kind, "")
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ImportSource
	for key, gosoID := range s.importSources {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		sourceID := strings.TrimPrefix(key, prefix)
		out = append(out, ImportSource{TenantID: tenant, Kind: kind, SourceID: sourceID, GosoID: gosoID})
	}
	return out, nil
}

// DeleteImportSources removes mappings for this exact tenant id and kind.
func (s *Store) DeleteImportSources(tenant, kind string) (int, error) {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	if tenant == "" || kind == "" {
		return 0, ErrNotFound
	}
	prefix := importSourceKey(tenant, kind, "")
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for key := range s.importSources {
		if strings.HasPrefix(key, prefix) {
			delete(s.importSources, key)
			n++
		}
	}
	return n, nil
}

// DeleteImportSource removes one mapping for this tenant, kind, and source id.
func (s *Store) DeleteImportSource(tenant, kind, sourceID string) error {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	sourceID = strings.TrimSpace(sourceID)
	if tenant == "" || kind == "" || sourceID == "" {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := importSourceKey(tenant, kind, sourceID)
	if _, ok := s.importSources[key]; !ok {
		return ErrNotFound
	}
	delete(s.importSources, key)
	return nil
}

// DeleteChannelConfig removes one channel_config row. Missing name is ErrNotFound.
func (s *Store) DeleteChannelConfig(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channelConfig[name]; !ok {
		return ErrNotFound
	}
	delete(s.channelConfig, name)
	return nil
}
