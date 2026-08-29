// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	maxSpawnOut      = 64 << 10
	sandboxMemLimit  = "256m"
	sandboxNetNone   = "none"
	sandboxMountMode = "ro" // host bind is read-only; writes do not persist
)

var (
	sandboxTimeout = 15 * time.Second
	browserTimeout = 20 * time.Second
	mediaTimeout   = 15 * time.Second
)

func sandboxConfigured() bool {
	return sandboxImage() != "" && dockerBin() != ""
}

func sandboxImage() string {
	return strings.TrimSpace(os.Getenv("GOSO_SANDBOX_IMAGE"))
}

func dockerBin() string {
	return existingLookPath("docker")
}

func browserBin() string {
	for _, key := range []string{"GOSO_BROWSER_BIN", "CHROME_PATH"} {
		if p := existingFile(os.Getenv(key)); p != "" {
			return p
		}
	}
	return ""
}

func ffmpegBin() string {
	if raw := strings.TrimSpace(os.Getenv("GOSO_FFMPEG")); raw != "" {
		return existingFile(raw)
	}
	if !envFlagOn(os.Getenv("GOSO_MEDIA")) {
		return ""
	}
	return existingLookPath("ffmpeg")
}

func existingFile(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return ""
	}
	return p
}

func existingLookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return existingFile(p)
}

func invokeSandbox(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	image := sandboxImage()
	bin := dockerBin()
	if image == "" || bin == "" {
		return notConfigured(ToolSandbox), nil
	}
	argv := cmdArgv(args)
	if len(argv) == 0 {
		return toolErr(ToolSandbox, "error", "cmd is required"), nil
	}
	dockerArgs := []string{"run", "--rm", "--network=" + sandboxNetNone, "--memory=" + sandboxMemLimit}
	if workspaceConfigured() {
		root, err := workspaceAbs()
		if err != nil {
			return mapFSErr(ToolSandbox, err), nil
		}
		dockerArgs = append(dockerArgs, "-v", root+":"+root+":"+sandboxMountMode)
	}
	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, argv...)
	return runCaptured(ctx, ToolSandbox, bin, dockerArgs, sandboxTimeout)
}

func invokeBrowser(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	bin := browserBin()
	if bin == "" {
		return notConfigured(ToolBrowser), nil
	}
	raw := urlArg(args)
	if raw == "" {
		return toolErr(ToolBrowser, "error", "url is required"), nil
	}
	if err := security.CheckURL(raw); err != nil {
		return toolErr(ToolBrowser, "error", err.Error()), nil
	}
	argv := []string{"--headless", "--disable-gpu", "--no-sandbox", raw}
	return runCaptured(ctx, ToolBrowser, bin, argv, browserTimeout)
}

func runFFmpeg(ctx context.Context, bin string, args map[string]any) (*connector.InvokeResult, error) {
	in, _ := stringArg(args, "in", "input")
	in = strings.TrimSpace(in)
	out, _ := stringArg(args, "out", "output")
	out = strings.TrimSpace(out)
	if in == "" && out == "" {
		return runCaptured(ctx, ToolMedia, bin, []string{"-version"}, mediaTimeout)
	}
	if in == "" || out == "" {
		return toolErr(ToolMedia, "error", "in and out are required"), nil
	}
	if !workspaceConfigured() {
		return notConfigured(ToolMedia), nil
	}
	inAbs, _, err := jailPath(in)
	if err != nil {
		return mapFSErr(ToolMedia, err), nil
	}
	outAbs, _, err := jailPath(out)
	if err != nil {
		return mapFSErr(ToolMedia, err), nil
	}
	return runCaptured(ctx, ToolMedia, bin, []string{"-y", "-i", inAbs, outAbs}, mediaTimeout)
}

func runCaptured(ctx context.Context, tool, bin string, argv []string, timeout time.Duration) (*connector.InvokeResult, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{n: maxSpawnOut, buf: &stdout}
	cmd.Stderr = &limitWriter{n: maxSpawnOut, buf: &stderr}
	err := cmd.Run()
	out := stdout.String()
	errOut := stderr.String()
	exit := 0
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	content := map[string]any{
		"stdout":    out,
		"exit_code": exit,
	}
	if errOut != "" {
		content["stderr"] = errOut
	}
	if err == nil {
		return &connector.InvokeResult{
			Tool:      tool,
			Connector: ConnectorName,
			Status:    "ok",
			Content:   content,
		}, nil
	}
	if ctx.Err() != nil {
		content["error"] = "timeout"
		return &connector.InvokeResult{
			Tool:      tool,
			Connector: ConnectorName,
			Status:    "error",
			Content:   content,
		}, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		content["error"] = "exit"
		return &connector.InvokeResult{
			Tool:      tool,
			Connector: ConnectorName,
			Status:    "error",
			Content:   content,
		}, nil
	}
	content["error"] = "exec failed"
	return &connector.InvokeResult{
		Tool:      tool,
		Connector: ConnectorName,
		Status:    "error",
		Content:   content,
	}, nil
}

func cmdArgv(args map[string]any) []string {
	if args == nil {
		return nil
	}
	var out []string
	if v, ok := args["cmd"]; ok {
		out = append(out, anyArgv(v)...)
	}
	if v, ok := args["args"]; ok {
		out = append(out, anyArgv(v)...)
	}
	return out
}

func anyArgv(v any) []string {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

type limitWriter struct {
	n   int
	buf *bytes.Buffer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf == nil {
		return len(p), nil
	}
	remain := w.n - w.buf.Len()
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		return len(p), nil
	}
	return w.buf.Write(p)
}

var _ io.Writer = (*limitWriter)(nil)
