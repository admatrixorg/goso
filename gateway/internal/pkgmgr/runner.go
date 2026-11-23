package pkgmgr

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExecResult is one bounded command outcome from a package step.
type ExecResult struct {
	OK       bool
	ExitCode int
	Output   string
	Err      string
}

// Runner executes one package command. Tests replace it to stay hermetic.
type Runner func(ctx context.Context, name string, args []string) ExecResult

// DefaultRunner runs the command with a bounded timeout and clips output.
func DefaultRunner(timeout time.Duration) Runner {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return func(ctx context.Context, name string, args []string) ExecResult {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.CommandContext(cctx, name, args...)
		out, err := cmd.CombinedOutput()
		text := string(out)
		if len(text) > 8*1024 {
			text = text[:8*1024]
		}
		res := ExecResult{Output: strings.TrimSpace(text)}
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				res.ExitCode = ee.ExitCode()
			}
			res.Err = err.Error()
			return res
		}
		res.OK = true
		return res
	}
}

// PythonBin returns the interpreter that package installs target.
func PythonBin() string {
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	return "python3"
}

// applyCommand builds the install command for one ecosystem.
func applyCommand(eco, name, version string) (string, []string, bool) {
	switch eco {
	case EcoPython:
		return PythonBin(), []string{"-m", "pip", "install", "--no-input", "--disable-pip-version-check", name + "==" + version}, true
	case EcoNode:
		return "npm", []string{"install", "-g", name + "@" + version}, true
	default:
		return "", nil, false
	}
}

// removeCommand builds the uninstall command for one ecosystem.
func removeCommand(eco, name string) (string, []string, bool) {
	switch eco {
	case EcoPython:
		return PythonBin(), []string{"-m", "pip", "uninstall", "-y", name}, true
	case EcoNode:
		return "npm", []string{"uninstall", "-g", name}, true
	default:
		return "", nil, false
	}
}

// verifyCommand checks the package is importable or resolvable.
func verifyCommand(eco, name string) (string, []string, bool) {
	switch eco {
	case EcoPython:
		return PythonBin(), []string{"-c", "import " + strings.ReplaceAll(name, "-", "_")}, true
	case EcoNode:
		return "npm", []string{"ls", "-g", name}, true
	default:
		return "", nil, false
	}
}

var _ = strconv.Itoa
