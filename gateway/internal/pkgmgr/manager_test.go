// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pkgmgr

import (
	"strings"
	"testing"
)

func testMgr() *Manager {
	return New(func() []Runtime {
		return []Runtime{
			{Name: "python", Ecosystem: EcoPython, Present: true, Version: "3.12.1", Compatible: true},
			{Name: "node", Ecosystem: EcoNode, Present: true, Version: "22.1.0", Compatible: true},
			{Name: "git", Ecosystem: EcoGitHub, Present: true, Version: "2.45.0", Compatible: true},
			{Name: "go", Present: true, Version: "1.22.5", Compatible: true},
		}
	})
}

func TestAllowPinAndInstall(t *testing.T) {
	m := testMgr()
	if _, err := m.Allow("python", "httpx", "latest"); err != ErrPinInvalid {
		t.Fatalf("latest pin %v", err)
	}
	if _, err := m.Allow("python", "httpx", "^1.0.0"); err != ErrPinInvalid {
		t.Fatalf("range pin %v", err)
	}
	al, err := m.Allow("python", "httpx", "0.27.2")
	if err != nil || al.Pin != "0.27.2" {
		t.Fatalf("allow %v %#v", err, al)
	}
	if _, _, err := m.Install("python", "httpx", "0.27.2", ""); err != ErrConfirmRequired {
		t.Fatalf("no confirm %v", err)
	}
	if _, _, err := m.Install("python", "httpx", "0.27.2", "nope"); err != ErrConfirm {
		t.Fatalf("bad confirm %v", err)
	}
	if _, _, err := m.Install("python", "httpx", "0.1.0", "httpx"); err != ErrPinMismatch {
		t.Fatalf("pin mismatch %v", err)
	}
	if _, _, err := m.Install("python", "requests", "2.32.0", "requests"); err != ErrAllow {
		t.Fatalf("not allowlisted %v", err)
	}
	pkg, job, err := m.Install("python", "httpx", "0.27.2", "python/httpx@0.27.2")
	if err != nil || pkg.Status != StatusInstalled || job.Status != JobSucceeded || job.Progress != 100 {
		t.Fatalf("install %v %#v %#v", err, pkg, job)
	}
	snap := m.Snapshot()
	if len(snap.Packages) != 1 || snap.Packages[0].Status != StatusInstalled {
		t.Fatalf("snap %#v", snap.Packages)
	}
	if len(snap.Jobs) != 1 || strings.Contains(strings.ToLower(strings.Join(snap.Jobs[0].Log, " ")), "token") {
		t.Fatalf("jobs %#v", snap.Jobs)
	}
	if _, _, err := m.Install("python", "httpx", "0.27.2", "httpx"); err != ErrExists {
		t.Fatalf("dup %v", err)
	}
}

func TestPartialRecoverAndUninstall(t *testing.T) {
	m := testMgr()
	if _, err := m.Allow("node", "left-pad", "1.3.0"); err != nil {
		t.Fatal(err)
	}
	m.FailAt = 2
	pkg, job, err := m.Install("node", "left-pad", "1.3.0", "left-pad")
	if err == nil || pkg.Status != StatusPartial || job.Status != JobPartial || job.Progress != 70 {
		t.Fatalf("partial %v %#v %#v", err, pkg, job)
	}
	if _, _, err := m.Install("node", "left-pad", "1.3.0", "left-pad"); err != ErrUseRecover {
		t.Fatalf("retry %v", err)
	}
	pkg, job, err = m.Recover(pkg.ID, pkg.ID)
	if err != nil || pkg.Status != StatusInstalled || job.Action != ActionRecover || job.Status != JobSucceeded {
		t.Fatalf("recover %v %#v %#v", err, pkg, job)
	}
	if _, _, err := m.Uninstall(pkg.ID, ""); err != ErrConfirmRequired {
		t.Fatalf("un no confirm %v", err)
	}
	if _, _, err := m.Uninstall(pkg.ID, "nope"); err != ErrConfirm {
		t.Fatalf("un mismatch %v", err)
	}
	_, job, err = m.Uninstall(pkg.ID, "left-pad")
	if err != nil || job.Status != JobSucceeded {
		t.Fatalf("uninstall %v %#v", err, job)
	}
	if _, err := m.Get(pkg.ID); err != ErrNotFound {
		t.Fatalf("gone %v", err)
	}
}

func TestRuntimeGateAndGitHubWarning(t *testing.T) {
	m := New(func() []Runtime {
		return []Runtime{
			{Name: "python", Ecosystem: EcoPython, Present: false, Compatible: false, Warning: "python is not installed"},
			{Name: "git", Ecosystem: EcoGitHub, Present: true, Version: "2.45.0", Compatible: true},
		}
	})
	if _, err := m.Allow("python", "httpx", "0.27.2"); err != nil {
		t.Fatal(err)
	}
	pkg, job, err := m.Install("python", "httpx", "0.27.2", "httpx")
	if err != ErrRuntime || pkg.Status != StatusFailed || job.Status != JobFailed {
		t.Fatalf("runtime %v %#v %#v", err, pkg, job)
	}
	if pkg.Warning == "" || !strings.Contains(strings.Join(job.Log, "\n"), "compatibility") {
		t.Fatalf("warning %#v %#v", pkg, job)
	}
	if _, err := m.Allow("github", "acme/tools", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	pkg, job, err = m.Install("github", "acme/tools", "v1.2.3", "acme/tools")
	if err != nil || pkg.Status != StatusInstalled {
		t.Fatalf("gh %v %#v", err, pkg)
	}
	if pkg.Warning == "" || !strings.Contains(pkg.Warning, "CLI credential") {
		t.Fatalf("gh warning %#v logs %#v", pkg, job.Log)
	}
}

func TestCLICredentialsNeverReturned(t *testing.T) {
	m := testMgr()
	if _, err := m.SetCLI("gitlab", "aaaa"); err != ErrKind {
		t.Fatalf("kind %v", err)
	}
	got, err := m.SetCLI("github", "ghp_not_a_live_token_fixture")
	if err != nil || !got.Set || got.Kind != "github" {
		t.Fatalf("set %v %#v", err, got)
	}
	snap := m.Snapshot()
	raw := strings.ToLower(strings.Join([]string{
		snap.Credentials[0].Kind,
	}, " "))
	if strings.Contains(raw, "ghp_") || strings.Contains(raw, "token") {
		t.Fatalf("leak %s", raw)
	}
	for _, c := range snap.Credentials {
		if c.Kind == "github" && (!c.Set || c.UpdatedAt == nil) {
			t.Fatalf("meta %#v", c)
		}
		if c.Kind == "npm" && c.Set {
			t.Fatalf("npm set %#v", c)
		}
	}
	if _, err := m.ClearCLI("github", ""); err != ErrConfirmRequired {
		t.Fatalf("clear confirm %v", err)
	}
	cleared, err := m.ClearCLI("github", "github")
	if err != nil || cleared.Set {
		t.Fatalf("clear %v %#v", err, cleared)
	}
}

func TestGETShapeHasNoSecretKeys(t *testing.T) {
	m := testMgr()
	_, _ = m.Allow("system", "ffmpeg", "6.1.0")
	_, _, _ = m.Install("system", "ffmpeg", "6.1.0", "ffmpeg")
	_, _ = m.SetCLI("npm", "npm_fixture_not_live")
	snap := m.Snapshot()
	if len(snap.Credentials) != 3 {
		t.Fatalf("cred count %d", len(snap.Credentials))
	}
	for _, c := range snap.Credentials {
		if c.Kind == "" {
			t.Fatal("empty kind")
		}
	}
	if SecretField("token") != true || SecretField("pin") {
		t.Fatal("secret field helper")
	}
}

func TestUnpinConfirm(t *testing.T) {
	m := testMgr()
	al, err := m.Allow("python", "httpx", "0.27.2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Unpin(al.ID, "nope"); err != ErrConfirm {
		t.Fatalf("unpin mismatch %v", err)
	}
	got, err := m.Unpin(al.ID, "httpx")
	if err != nil || got.ID != al.ID {
		t.Fatalf("unpin %v %#v", err, got)
	}
	if len(m.Snapshot().Allowlist) != 0 {
		t.Fatalf("allow leftover")
	}
}

func TestProbeHostReportsSomething(t *testing.T) {
	rows := ProbeHost()
	if len(rows) != 4 {
		t.Fatalf("len %d", len(rows))
	}
	names := map[string]bool{}
	for _, r := range rows {
		names[r.Name] = true
		if r.Present && r.Version == "" && r.Warning == "" {
			t.Fatalf("present without version %#v", r)
		}
	}
	if !names["python"] || !names["node"] || !names["git"] || !names["go"] {
		t.Fatalf("names %#v", names)
	}
}

func TestSecretShapedNameRejected(t *testing.T) {
	m := testMgr()
	if _, err := m.Allow("python", "sk-live-abcdefgh", "1.0.0"); err != ErrSecret && err != ErrNameInvalid {
		t.Fatalf("secret name %v", err)
	}
}
