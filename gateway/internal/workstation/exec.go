package workstation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// ExecRequest is one command to run on a workstation.
type ExecRequest struct {
	Workstation *store.Workstation
	Command     string
	Args        []string
	CWD         string
	Env         map[string]string
	Timeout     time.Duration
}

// ExecResult is the bounded outcome, truncated so a runaway command cannot
// flood the response.
type ExecResult struct {
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
	Backend    string `json:"backend"`
}

// ErrDenied reports a command that no enabled pattern allows.
var ErrDenied = errors.New("command is not allowed on this workstation")

// ExecOutputCap bounds each stream in the response.
const ExecOutputCap = 64 * 1024

// AllowedCommand reports whether any enabled pattern allows this command.
// A workstation with no patterns allows nothing, which keeps an imported
// row from silently becoming a shell.
func AllowedCommand(patterns []*store.WorkstationPermission, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	first := command
	if i := strings.IndexAny(first, " \t"); i >= 0 {
		first = first[:i]
	}
	for _, p := range patterns {
		if p == nil || !p.Enabled {
			continue
		}
		want := strings.TrimSpace(p.Pattern)
		if want == "" {
			continue
		}
		if want == "*" || want == command || want == first {
			return true
		}
		if strings.HasSuffix(want, "*") && strings.HasPrefix(command, strings.TrimSuffix(want, "*")) {
			return true
		}
	}
	return false
}

type wsMeta struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	IdentityRef string `json:"identity_ref"`
	Container   string `json:"container"`
}

func readMeta(w *store.Workstation) wsMeta {
	var m wsMeta
	if w != nil && len(w.Metadata) > 0 {
		_ = json.Unmarshal(w.Metadata, &m)
	}
	return m
}

func clipStream(b []byte) (string, bool) {
	if len(b) <= ExecOutputCap {
		return string(b), false
	}
	return string(b[:ExecOutputCap]), true
}

// Run executes one command on the workstation backend.
func Run(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	if req.Workstation == nil {
		return nil, errors.New("workstation is required")
	}
	if strings.TrimSpace(req.Command) == "" {
		return nil, errors.New("command is required")
	}
	if req.Timeout <= 0 || req.Timeout > 5*time.Minute {
		req.Timeout = 60 * time.Second
	}
	switch strings.ToLower(strings.TrimSpace(req.Workstation.BackendType)) {
	case "local":
		return runLocal(ctx, req)
	case "docker":
		return runDocker(ctx, req)
	case "ssh", "":
		return runSSH(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported backend %q", req.Workstation.BackendType)
	}
}

func commandLine(req ExecRequest) []string {
	parts := []string{req.Command}
	parts = append(parts, req.Args...)
	return parts
}

func runLocal(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	args := commandLine(req)
	cmd := exec.CommandContext(cctx, args[0], args[1:]...)
	if strings.TrimSpace(req.CWD) != "" {
		cmd.Dir = req.CWD
	}
	if len(req.Env) > 0 {
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)
	res := &ExecResult{OK: err == nil, DurationMS: dur.Milliseconds(), Backend: "local"}
	res.Stdout, res.Truncated = clipStream(out.Bytes())
	res.Stderr, _ = clipStream(errb.Bytes())
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.Stderr = err.Error()
		}
	}
	return res, nil
}

func runDocker(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	meta := readMeta(req.Workstation)
	container := strings.TrimSpace(meta.Container)
	if container == "" {
		return nil, errors.New("docker backend needs container in metadata")
	}
	args := []string{"exec", container}
	args = append(args, commandLine(req)...)
	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)
	res := &ExecResult{OK: err == nil, DurationMS: dur.Milliseconds(), Backend: "docker"}
	res.Stdout, res.Truncated = clipStream(out.Bytes())
	res.Stderr, _ = clipStream(errb.Bytes())
	if err != nil {
		res.Stderr = strings.TrimSpace(res.Stderr + " " + err.Error())
	}
	return res, nil
}

func runSSH(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	meta := readMeta(req.Workstation)
	host := strings.TrimSpace(meta.Host)
	if host == "" {
		return nil, errors.New("ssh backend needs host in metadata")
	}
	port := meta.Port
	if port <= 0 {
		port = 22
	}
	user := strings.TrimSpace(meta.User)
	if user == "" {
		user = "root"
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            sshAuth(meta.IdentityRef),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         req.Timeout,
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: req.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	c, chans, creqs, cerr := ssh.NewClientConn(conn, addr, cfg)
	if cerr != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", cerr)
	}
	client := ssh.NewClient(c, chans, creqs)
	defer client.Close()
	sess, serr := client.NewSession()
	if serr != nil {
		return nil, fmt.Errorf("ssh session: %w", serr)
	}
	defer sess.Close()
	var out, errb bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &errb
	line := strings.Join(commandLine(req), " ")
	if strings.TrimSpace(req.CWD) != "" {
		line = "cd " + shellQuote(req.CWD) + " && " + line
	}
	for k, v := range req.Env {
		line = k + "=" + shellQuote(v) + " " + line
	}
	start := time.Now()
	err = sess.Run("sh -lc " + shellQuote(line))
	dur := time.Since(start)
	res := &ExecResult{OK: err == nil, DurationMS: dur.Milliseconds(), Backend: "ssh"}
	res.Stdout, res.Truncated = clipStream(out.Bytes())
	res.Stderr, _ = clipStream(errb.Bytes())
	if err != nil {
		if ee, ok := err.(*ssh.ExitError); ok {
			res.ExitCode = ee.ExitStatus()
		} else {
			res.Stderr = strings.TrimSpace(res.Stderr + " " + err.Error())
		}
	}
	return res, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// sshAuth resolves an identity reference to a key file path or agent socket.
func sshAuth(ref string) []ssh.AuthMethod {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "env:") {
		ref = strings.TrimPrefix(ref, "env:")
	}
	if data, err := osReadFile(ref); err == nil {
		if signer, serr := ssh.ParsePrivateKey(data); serr == nil {
			return []ssh.AuthMethod{ssh.PublicKeys(signer)}
		}
	}
	return nil
}

var osReadFile = os.ReadFile
