// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pkgmgr

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var verRe = regexp.MustCompile(`(\d+)\.(\d+)`)

type binSpec struct {
	Name      string
	Ecosystem string
	Bins      []string
	Args      []string
	MinMajor  int
	MinMinor  int
	Missing   string
	Old       string
}

// ProbeHost inspects PATH for python, node, git, and go. No network, no credentials.
func ProbeHost() []Runtime {
	specs := []binSpec{
		{Name: "python", Ecosystem: EcoPython, Bins: []string{"python3", "python"}, Args: []string{"--version"}, MinMajor: 3, MinMinor: 10, Missing: "python is not installed", Old: "python is older than 3.10"},
		{Name: "node", Ecosystem: EcoNode, Bins: []string{"node"}, Args: []string{"--version"}, MinMajor: 18, MinMinor: 0, Missing: "node is not installed", Old: "node is older than 18"},
		{Name: "git", Ecosystem: EcoGitHub, Bins: []string{"git"}, Args: []string{"--version"}, MinMajor: 2, MinMinor: 0, Missing: "git is not installed", Old: "git is older than 2"},
		{Name: "go", Bins: []string{"go"}, Args: []string{"version"}, MinMajor: 1, MinMinor: 21, Missing: "go is not installed", Old: "go is older than 1.21"},
	}
	out := make([]Runtime, 0, len(specs))
	for _, s := range specs {
		out = append(out, probeBin(s))
	}
	return out
}

func probeBin(s binSpec) Runtime {
	r := Runtime{Name: s.Name, Ecosystem: s.Ecosystem, Warning: s.Missing}
	var path string
	for _, b := range s.Bins {
		p, err := exec.LookPath(b)
		if err == nil {
			path = p
			break
		}
	}
	if path == "" {
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, s.Args...)
	raw, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(raw))
	if err != nil && text == "" {
		r.Present = true
		r.Warning = s.Name + " did not report a version"
		return r
	}
	ver := firstVersion(text)
	r.Present = true
	r.Version = ver
	maj, min := parseMajMin(ver)
	if maj > s.MinMajor || (maj == s.MinMajor && min >= s.MinMinor) {
		r.Compatible = true
		r.Warning = ""
		return r
	}
	r.Warning = s.Old
	return r
}

var fullVerRe = regexp.MustCompile(`\d+(?:\.\d+){1,3}`)

func firstVersion(s string) string {
	if full := fullVerRe.FindString(s); full != "" {
		return full
	}
	m := verRe.FindStringSubmatch(s)
	if len(m) < 3 {
		return ""
	}
	return m[1] + "." + m[2]
}

func parseMajMin(ver string) (int, int) {
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		return 0, 0
	}
	maj, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	return maj, min
}
