package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/workstation"
)

// registerWorkstationExecRoutes serves the command runner behind the workstation
// detail page. Every run needs an enabled permission pattern.
func registerWorkstationExecRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("POST /v1/workstations/{id}/exec", handleWorkstationExec(st))
}

func workstationExecPayload(w http.ResponseWriter, r *http.Request) (command string, args []string, cwd string, env map[string]string, timeout time.Duration, ok bool) {
	var body struct {
		Command    string            `json:"command"`
		Args       []string          `json:"args"`
		CWD        string            `json:"cwd"`
		Env        map[string]string `json:"env"`
		TimeoutSec int               `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return "", nil, "", nil, 0, false
	}
	command = strings.TrimSpace(body.Command)
	if command == "" {
		writeErr(w, http.StatusBadRequest, "command is required")
		return "", nil, "", nil, 0, false
	}
	timeout = 60 * time.Second
	if body.TimeoutSec > 0 && body.TimeoutSec <= 300 {
		timeout = time.Duration(body.TimeoutSec) * time.Second
	}
	return command, body.Args, strings.TrimSpace(body.CWD), body.Env, timeout, true
}

func handleWorkstationExec(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Workstations(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "workstation store unavailable")
			return
		}
		tenant := requestTenant(r)
		ws, err := x.GetWorkstation(tenant, r.PathValue("id"))
		if err != nil || ws == nil {
			writeErr(w, http.StatusNotFound, "workstation not found")
			return
		}
		command, args, cwd, env, timeout, ok := workstationExecPayload(w, r)
		if !ok {
			return
		}
		patterns := x.ListWorkstationPermissions(tenant, ws.ID)
		if !workstation.AllowedCommand(patterns, command) {
			writeErr(w, http.StatusForbidden, "command is not allowed on this workstation")
			return
		}
		if cwd == "" {
			cwd = ws.DefaultCWD
		}
		res, rerr := workstation.Run(r.Context(), workstation.ExecRequest{
			Workstation: ws, Command: command, Args: args, CWD: cwd, Env: env, Timeout: timeout,
		})
		if rerr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok": false, "error": rerr.Error(), "workstation": ws.Key,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": res.OK, "exit_code": res.ExitCode, "stdout": res.Stdout, "stderr": res.Stderr,
			"duration_ms": res.DurationMS, "truncated": res.Truncated, "backend": res.Backend,
			"workstation": ws.Key, "command": command,
		})
	}
}

// handleWorkstationProbe runs a fixed echo so the Test button reports reachability.
func handleWorkstationProbe(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Workstations(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "workstation store unavailable")
			return
		}
		tenant := requestTenant(r)
		ws, err := x.GetWorkstation(tenant, r.PathValue("id"))
		if err != nil || ws == nil {
			writeErr(w, http.StatusNotFound, "workstation not found")
			return
		}
		res, rerr := workstation.Run(r.Context(), workstation.ExecRequest{
			Workstation: ws, Command: "echo", Args: []string{"goso-probe"}, Timeout: 15 * time.Second,
		})
		if rerr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "connected": false, "error": rerr.Error(),
				"backend": ws.BackendType, "workstation": ws.Key,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": res.OK, "connected": res.OK, "output": strings.TrimSpace(res.Stdout),
			"duration_ms": res.DurationMS, "backend": res.Backend, "workstation": ws.Key,
		})
	}
}

var _ = strconv.Itoa
