// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package health

import (
	"fmt"
	"runtime"

	"github.com/mqglobal/goso/gateway/internal/config"
)

// Check represents a single doctor check.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

// Report is the doctor output.
type Report struct {
	Checks []Check `json:"checks"`
	OK     bool    `json:"ok"`
}

// Run executes all health checks.
func Run(cfg config.Config) Report {
	checks := []Check{
		checkGoVersion(),
		checkPort(cfg.Port),
		checkEnv(cfg.Env),
	}
	ok := true
	for _, c := range checks {
		if c.Status == "fail" {
			ok = false
		}
	}
	return Report{Checks: checks, OK: ok}
}

func checkGoVersion() Check {
	v := runtime.Version()
	return Check{Name: "go_version", Status: "ok", Detail: v}
}

func checkPort(port int) Check {
	if port < 1 || port > 65535 {
		return Check{Name: "port", Status: "fail", Detail: fmt.Sprintf("invalid port %d", port)}
	}
	return Check{Name: "port", Status: "ok", Detail: fmt.Sprintf("port %d", port)}
}

func checkEnv(env string) Check {
	if env == "" {
		return Check{Name: "env", Status: "warn", Detail: "GOSO_ENV not set, defaulting to development"}
	}
	return Check{Name: "env", Status: "ok", Detail: env}
}
