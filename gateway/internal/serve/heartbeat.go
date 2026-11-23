package serve

import (
	"context"
	"time"

	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// heartbeatLoop fires due agent heartbeats. It runs in the gateway process
// like the cron loop: every tenant's due heartbeat is claimed and run once.
func heartbeatLoop(ctx context.Context, st store.StoreIface, fire cron.FireFunc) {
	x := store.Extras(st)
	if x == nil || st == nil {
		return
	}
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, h := range x.ListDueHeartbeats(now) {
				go runHeartbeatOnce(st, h, fire)
			}
		}
	}
}

func runHeartbeatOnce(st store.StoreIface, h *store.AgentHeartbeat, fire cron.FireFunc) {
	x := store.Extras(st)
	if h == nil || x == nil || !h.Enabled || h.Prompt == "" {
		return
	}
	sess, err := st.CreateSession(store.Session{TenantID: h.TenantID, AgentID: h.AgentID, Label: "heartbeat:" + h.AgentID})
	if err != nil || sess == nil {
		_ = x.MarkHeartbeatRun(h.TenantID, h.AgentID, "error", "session create failed")
		return
	}
	status, msg := "ok", ""
	if fire != nil {
		if ferr := fire(context.Background(), sess.ID, h.Prompt); ferr != nil {
			status, msg = "error", "run failed"
		}
	}
	_ = x.MarkHeartbeatRun(h.TenantID, h.AgentID, status, msg)
}
