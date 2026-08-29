// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigured_SandboxBrowserMediaEmpty(t *testing.T) {
	t.Setenv("GOSO_SANDBOX_IMAGE", "")
	t.Setenv("GOSO_BROWSER_BIN", "")
	t.Setenv("CHROME_PATH", "")
	t.Setenv("GOSO_FFMPEG", "")
	t.Setenv("GOSO_MEDIA", "")
	t.Setenv("PATH", t.TempDir())
	if Configured(ToolSandbox) || Configured(ToolBrowser) || Configured(ToolMedia) {
		t.Fatal("empty env must not report configured")
	}
}

func TestInvoke_SandboxEmptyEnvNoProcess(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "echo spawned")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "")
	t.Setenv("GOSO_WORKSPACE", "")
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("empty image must not spawn docker")
	}
	if Configured(ToolSandbox) {
		t.Fatal("configured")
	}
}

func TestInvoke_SandboxImageWithoutDocker(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GOSO_SANDBOX_IMAGE", "goso-sandbox:test")
	if Configured(ToolSandbox) {
		t.Fatal("image without docker must not be configured")
	}
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestInvoke_SandboxDockerWithoutImage(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "")
	if Configured(ToolSandbox) {
		t.Fatal("docker without image must not be configured")
	}
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("must not spawn docker without image")
	}
}

func TestInvoke_SandboxFakeDockerArgv(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "echo ok-out")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "goso-sandbox:test")
	t.Setenv("GOSO_WORKSPACE", "")
	if !Configured(ToolSandbox) {
		t.Fatal("want configured")
	}
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": []any{"echo", "hello"}}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if !strings.Contains(m["stdout"].(string), "ok-out") {
		t.Fatalf("stdout %+v", m)
	}
	got := readArgvLog(t, log)
	wantPrefix := []string{"run", "--rm", "--network=none", "--memory=256m", "goso-sandbox:test", "echo", "hello"}
	if len(got) < len(wantPrefix) {
		t.Fatalf("argv %v", got)
	}
	for i, w := range wantPrefix {
		if got[i] != w {
			t.Fatalf("argv[%d]=%q want %q full=%v", i, got[i], w, got)
		}
	}
	if containsShC(got) {
		t.Fatalf("must not wrap in sh -c: %v", got)
	}
	if strings.Join(got, " ") != strings.Join(wantPrefix, " ") {
		t.Fatalf("argv %v", got)
	}
}

func TestInvoke_SandboxWorkspaceMountRO(t *testing.T) {
	dir := t.TempDir()
	ws := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "img")
	t.Setenv("GOSO_WORKSPACE", ws)
	abs, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		abs = ev
	}
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	got := readArgvLog(t, log)
	vol := abs + ":" + abs + ":ro"
	if !containsPair(got, "-v", vol) {
		t.Fatalf("missing -v %s in %v", vol, got)
	}
	if containsShC(got) {
		t.Fatalf("sh -c %v", got)
	}
}

func TestInvoke_SandboxTimeout(t *testing.T) {
	prev := sandboxTimeout
	sandboxTimeout = 200 * time.Millisecond
	t.Cleanup(func() { sandboxTimeout = prev })
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "exec /bin/sleep 2")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "img")
	t.Setenv("GOSO_WORKSPACE", "")
	start := time.Now()
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "timeout" {
		t.Fatalf("content %+v", m)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout too slow %s", time.Since(start))
	}
}

func TestInvoke_SandboxCmdRequired(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	writeExecLogScript(t, dir, "docker", log, "")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_SANDBOX_IMAGE", "img")
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "cmd is required" {
		t.Fatalf("content %+v", m)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("must not spawn without cmd")
	}
}

func TestInvoke_BrowserEmptyEnvNoProcess(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "chrome.log")
	bin := writeExecLogScript(t, dir, "chrome", log, "echo chrome-out")
	t.Setenv("GOSO_BROWSER_BIN", "")
	t.Setenv("CHROME_PATH", "")
	t.Setenv("GOSO_SSRF", "0")
	if Configured(ToolBrowser) {
		t.Fatal("configured")
	}
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("must not spawn chrome")
	}
	_ = bin
}

func TestInvoke_BrowserMissingBinNoSpawn(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-chrome")
	t.Setenv("GOSO_BROWSER_BIN", missing)
	t.Setenv("CHROME_PATH", "")
	if Configured(ToolBrowser) {
		t.Fatal("missing file must not be configured")
	}
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestInvoke_BrowserFakeBinArgv(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "chrome.log")
	bin := writeExecLogScript(t, dir, "chrome", log, "echo chrome-out")
	t.Setenv("GOSO_BROWSER_BIN", bin)
	t.Setenv("CHROME_PATH", "")
	t.Setenv("GOSO_SSRF", "0")
	if !Configured(ToolBrowser) {
		t.Fatal("want configured")
	}
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com/x"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if !strings.Contains(m["stdout"].(string), "chrome-out") {
		t.Fatalf("stdout %+v", m)
	}
	got := readArgvLog(t, log)
	want := []string{"--headless", "--disable-gpu", "--no-sandbox", "https://example.com/x"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("argv %v", got)
	}
}

func TestInvoke_BrowserChromePathFallback(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "chrome.log")
	bin := writeExecLogScript(t, dir, "google-chrome", log, "echo ok")
	t.Setenv("GOSO_BROWSER_BIN", "")
	t.Setenv("CHROME_PATH", bin)
	t.Setenv("GOSO_SSRF", "0")
	if !Configured(ToolBrowser) {
		t.Fatal("CHROME_PATH must configure")
	}
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestInvoke_BrowserSSRFBlocksLocalhost(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "chrome.log")
	bin := writeExecLogScript(t, dir, "chrome", log, "echo spawned")
	t.Setenv("GOSO_BROWSER_BIN", bin)
	t.Setenv("CHROME_PATH", "")
	t.Setenv("GOSO_SSRF", "1")
	t.Setenv("GOSO_ENV", "development")
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "http://127.0.0.1/"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	msg, _ := m["error"].(string)
	if !strings.Contains(strings.ToLower(msg), "ssrf") {
		t.Fatalf("want ssrf %+v", m)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("SSRF must not spawn chrome")
	}
	res, err = Invoke(context.Background(), ToolBrowser, map[string]any{"url": "http://localhost/"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("localhost %v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("localhost SSRF must not spawn chrome")
	}
}

func TestInvoke_BrowserTimeout(t *testing.T) {
	prev := browserTimeout
	browserTimeout = 200 * time.Millisecond
	t.Cleanup(func() { browserTimeout = prev })
	dir := t.TempDir()
	log := filepath.Join(dir, "chrome.log")
	bin := writeExecLogScript(t, dir, "chrome", log, "exec /bin/sleep 2")
	t.Setenv("GOSO_BROWSER_BIN", bin)
	t.Setenv("GOSO_SSRF", "0")
	res, err := Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "timeout" {
		t.Fatalf("content %+v", m)
	}
}

func TestInvoke_MediaMissingFFmpeg(t *testing.T) {
	t.Setenv("GOSO_FFMPEG", "")
	t.Setenv("GOSO_MEDIA", "")
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { MediaInvoke = nil })
	MediaInvoke = nil
	if Configured(ToolMedia) {
		t.Fatal("configured")
	}
	res, err := Invoke(context.Background(), ToolMedia, map[string]any{}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestInvoke_MediaGOSOFFMPEGHealth(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "ffmpeg.log")
	bin := writeExecLogScript(t, dir, "ffmpeg", log, `
if [ "$1" = "-version" ]; then
  echo "ffmpeg version fake"
  exit 0
fi
`)
	t.Setenv("GOSO_FFMPEG", bin)
	t.Setenv("GOSO_MEDIA", "")
	t.Setenv("PATH", t.TempDir())
	if !Configured(ToolMedia) {
		t.Fatal("GOSO_FFMPEG existing file must configure media")
	}
	if Configured(ToolImageGen) || Configured(ToolTTS) {
		t.Fatal("image_gen/tts stay unconfigured without double")
	}
	res, err := Invoke(context.Background(), ToolMedia, map[string]any{}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if !strings.Contains(m["stdout"].(string), "ffmpeg version fake") {
		t.Fatalf("stdout %+v", m)
	}
	got := readArgvLog(t, log)
	if len(got) != 1 || got[0] != "-version" {
		t.Fatalf("argv %v", got)
	}
	res, err = Invoke(context.Background(), ToolImageGen, map[string]any{"prompt": "x"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("image_gen %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolTTS, map[string]any{"text": "x"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("tts %v %+v", err, res)
	}
}

func TestInvoke_MediaPATHFFmpegRequiresGOSOMEDIA(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "ffmpeg.log")
	writeExecLogScript(t, dir, "ffmpeg", log, `echo "ffmpeg version path"`)
	t.Setenv("GOSO_FFMPEG", "")
	t.Setenv("PATH", dir)
	t.Setenv("GOSO_MEDIA", "")
	if Configured(ToolMedia) {
		t.Fatal("PATH ffmpeg without GOSO_MEDIA=1 must not configure")
	}
	res, err := Invoke(context.Background(), ToolMedia, nil, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("must not spawn ffmpeg")
	}
	t.Setenv("GOSO_MEDIA", "1")
	if !Configured(ToolMedia) {
		t.Fatal("PATH ffmpeg + GOSO_MEDIA=1 must configure")
	}
	res, err = Invoke(context.Background(), ToolMedia, map[string]any{}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("health %v %+v", err, res)
	}
}

func TestInvoke_MediaTranscodeJail(t *testing.T) {
	dir := t.TempDir()
	ws := t.TempDir()
	inFile := filepath.Join(ws, "in.wav")
	if err := os.WriteFile(inFile, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "ffmpeg.log")
	bin := writeExecLogScript(t, dir, "ffmpeg", log, `
last=
for a in "$@"; do last=$a; done
if [ -n "$last" ]; then : > "$last"; fi
`)
	t.Setenv("GOSO_FFMPEG", bin)
	t.Setenv("GOSO_WORKSPACE", ws)
	t.Setenv("GOSO_MEDIA", "")
	res, err := Invoke(context.Background(), ToolMedia, map[string]any{"in": "in.wav", "out": "out.mp3"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	got := readArgvLog(t, log)
	if len(got) != 4 || got[0] != "-y" || got[1] != "-i" {
		t.Fatalf("argv %v", got)
	}
	if !strings.HasSuffix(got[2], "in.wav") || !strings.HasSuffix(got[3], "out.mp3") {
		t.Fatalf("paths %v", got)
	}
	if _, err := os.Stat(filepath.Join(ws, "out.mp3")); err != nil {
		t.Fatalf("out %v", err)
	}

	outside := filepath.Join(t.TempDir(), "escape.mp3")
	res, err = Invoke(context.Background(), ToolMedia, map[string]any{"in": "in.wav", "out": outside}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("escape out %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("escape content %+v", m)
	}
	res, err = Invoke(context.Background(), ToolMedia, map[string]any{"in": "../secret.wav", "out": "out.mp3"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("escape in %v %+v", err, res)
	}
}

func TestInvoke_MediaTranscodeEmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "ffmpeg.log")
	bin := writeExecLogScript(t, dir, "ffmpeg", log, "")
	t.Setenv("GOSO_FFMPEG", bin)
	t.Setenv("GOSO_WORKSPACE", "")
	res, err := Invoke(context.Background(), ToolMedia, map[string]any{"in": "a.wav", "out": "b.mp3"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("must not spawn ffmpeg without workspace jail")
	}
}

func writeExecLogScript(t *testing.T, dir, name, logPath, extra string) string {
	t.Helper()
	body := "#!/bin/sh\nlog=" + shellQuote(logPath) + "\n: > \"$log\"\nfor a in \"$@\"; do\n  printf '%s\\n' \"$a\" >> \"$log\"\ndone\n" + extra + "\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func readArgvLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func containsShC(argv []string) bool {
	for i, a := range argv {
		if a == "sh" || a == "/bin/sh" || a == "bash" {
			if i+1 < len(argv) && argv[i+1] == "-c" {
				return true
			}
		}
		if a == "sh -c" || a == "-c" && i > 0 && (argv[i-1] == "sh" || argv[i-1] == "/bin/sh") {
			return true
		}
	}
	return false
}

func containsPair(argv []string, flag, val string) bool {
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == flag && argv[i+1] == val {
			return true
		}
	}
	return false
}
