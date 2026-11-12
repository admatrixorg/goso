// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memorybench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// BaselineCommit is the main revision this floor was measured on.
const BaselineCommit = "9545d08"

// BenchDir is bench/memory-vi at the repo root. The source file lives in
// gateway/memorybench, so two parents up is the module root.
func BenchDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("memorybench: caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "bench", "memory-vi")
}

// BaselinePath is the checked-in floor.
func BaselinePath() string {
	return filepath.Join(BenchDir(), "baseline-9545d08.json")
}

// BaselineFile is the checked-in result of main 9545d08.
type BaselineFile struct {
	Spec    int                       `json:"spec"`
	Commit  string                    `json:"commit"`
	Floor   string                    `json:"floor"`
	Engines map[string]BaselineEngine `json:"engines"`
}

// BaselineEngine is one store's floor.
type BaselineEngine struct {
	Passed int                      `json:"passed"`
	Total  int                      `json:"total"`
	Score  string                   `json:"score"`
	Groups map[string]BaselineGroup `json:"groups"`
}

// BaselineGroup is one group's floor, including each case.
type BaselineGroup struct {
	Name   string          `json:"name"`
	Passed int             `json:"passed"`
	Total  int             `json:"total"`
	Score  string          `json:"score"`
	Cases  map[string]bool `json:"cases"`
}

const floorText = "Floor of cross-session recall at main 9545d08. A later run fails when a case marked true here fails, or when any case that must return nothing admits an item. A higher score does not fail and does not require editing this file."

// LoadBaseline reads the checked-in floor.
func LoadBaseline(path string) (BaselineFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BaselineFile{}, err
	}
	var file BaselineFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return BaselineFile{}, err
	}
	if file.Spec != 153 || file.Commit != BaselineCommit {
		return BaselineFile{}, fmt.Errorf("baseline identity spec=%d commit=%s", file.Spec, file.Commit)
	}
	return file, nil
}

// NewBaseline builds the floor document from a sqlite and a postgres run.
func NewBaseline(sqlite, postgres EngineScore) BaselineFile {
	return BaselineFile{
		Spec:   153,
		Commit: BaselineCommit,
		Floor:  floorText,
		Engines: map[string]BaselineEngine{
			"sqlite":   toEngine(sqlite),
			"postgres": toEngine(postgres),
		},
	}
}

func toEngine(score EngineScore) BaselineEngine {
	groups := make(map[string]BaselineGroup, len(score.Groups))
	for id, g := range score.Groups {
		cases := make(map[string]bool, len(g.Cases))
		for k, v := range g.Cases {
			cases[k] = v
		}
		groups[id] = BaselineGroup{
			Name: g.Name, Passed: g.Passed, Total: g.Total, Score: g.Score(), Cases: cases,
		}
	}
	return BaselineEngine{
		Passed: score.Passed, Total: score.Total, Score: score.Score(), Groups: groups,
	}
}

// WriteBaseline writes the floor with a trailing newline. Map keys are sorted
// by encoding/json, so the file is stable.
func WriteBaseline(path string, file BaselineFile) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

// Regressions lists cases and groups that scored below the floor.
// A case that was false and is now true is not a regression.
func Regressions(base BaselineEngine, got EngineScore) []string {
	var out []string
	if base.Total != got.Total || base.Score != fmt.Sprintf("%d/%d", base.Passed, base.Total) {
		out = append(out, fmt.Sprintf("suite total baseline %s file-count %d/%d run %s", base.Score, base.Passed, base.Total, got.Score()))
	}
	if len(base.Groups) != len(got.Groups) {
		out = append(out, "group set changed")
	}
	for _, g := range Groups {
		bg, ok := base.Groups[g.ID]
		if !ok {
			out = append(out, "missing baseline group "+g.ID)
			continue
		}
		gg := got.Groups[g.ID]
		if bg.Name != g.Name || bg.Total != gg.Total || bg.Score != fmt.Sprintf("%d/%d", bg.Passed, bg.Total) {
			out = append(out, fmt.Sprintf("%s baseline shape %s (%d) run %s", g.Name, bg.Score, bg.Total, gg.Score()))
		}
		if gg.Passed < bg.Passed {
			out = append(out, fmt.Sprintf("%s dropped %s -> %s", g.Name, bg.Score, gg.Score()))
		}
		if len(bg.Cases) != gg.Total {
			out = append(out, fmt.Sprintf("%s case count baseline %d run %d", g.Name, len(bg.Cases), gg.Total))
		}
		for id, passed := range bg.Cases {
			now, ok := gg.Cases[id]
			if !ok {
				out = append(out, "missing case "+id)
				continue
			}
			if passed && !now {
				out = append(out, fmt.Sprintf("%s passed at %s and now fails", id, BaselineCommit))
			}
		}
		for id := range gg.Cases {
			if _, ok := bg.Cases[id]; !ok {
				out = append(out, "case not in baseline "+id)
			}
		}
	}
	return out
}

// SamePass reports whether two runs scored the same cases.
func SamePass(a, b EngineScore) bool {
	if a.Passed != b.Passed || a.Total != b.Total || len(a.Results) != len(b.Results) {
		return false
	}
	ab, bb := a.ByID(), b.ByID()
	if len(ab) != len(bb) {
		return false
	}
	for id, ar := range ab {
		br, ok := bb[id]
		if !ok || ar.Pass != br.Pass || ar.Injected != br.Injected {
			return false
		}
	}
	return true
}

// CanonicalBytes renders a baseline the way WriteBaseline does, for tests.
func CanonicalBytes(file BaselineFile) ([]byte, error) {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// BytesEqual compares two JSON documents after re-encoding is not required.
func BytesEqual(a, b []byte) bool { return bytes.Equal(a, b) }
