package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// wsZaloQRStart runs the Zalo Personal QR login and streams the code to the
// caller as events: zalo.personal.qr.code carries the PNG, and
// zalo.personal.qr.done reports the outcome. Once the login finishes the cookie
// set is stored for the instance, so the channel is authenticated.
func wsZaloQRStart(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		InstanceID  string `json:"instance_id"`
		InstanceID2 string `json:"instanceId"`
		Channel     string `json:"channel"`
	}
	_ = json.Unmarshal(req.Params, &p)
	instance := firstNonEmpty(p.InstanceID, p.InstanceID2, p.Channel)
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "instance_id is required")
	}
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	if cfg, err := st.GetChannelConfig(instance); err != nil || cfg == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "channel instance not found")
	}
	if err := writeGoClaw(conn, req.ID, map[string]any{"ok": true, "instance_id": instance, "status": "starting"}); err != nil {
		return err
	}
	go runZaloQR(conn, st, tenant, instance)
	return nil
}

func runZaloQR(conn *websocket.Conn, st store.StoreIface, tenant, instance string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	qr := protocol.NewQRSession()
	code, err := qr.StartQR(ctx)
	if err != nil {
		_ = writeGoClawEvent(conn, "zalo.personal.qr.done", map[string]any{
			"instance_id": instance, "success": false, "error": err.Error(),
		})
		return
	}
	if err := writeGoClawEvent(conn, "zalo.personal.qr.code", map[string]any{
		"instance_id": instance,
		"png_b64":     protocol.PNGBase64(code.ImagePNG),
		"code":        code.Code,
	}); err != nil {
		return
	}
	if err := qr.WaitForScan(ctx); err != nil {
		_ = writeGoClawEvent(conn, "zalo.personal.qr.done", map[string]any{
			"instance_id": instance, "success": false, "error": err.Error(),
		})
		return
	}
	if err := qr.WaitForConfirm(ctx); err != nil {
		_ = writeGoClawEvent(conn, "zalo.personal.qr.done", map[string]any{
			"instance_id": instance, "success": false, "error": err.Error(),
		})
		return
	}
	cred, cerr := qr.Finish(ctx)
	if cerr != nil || cred == nil || len(cred.Cookies) == 0 {
		msg := "login finished but no cookies were captured"
		if cerr != nil {
			msg = cerr.Error()
		}
		_ = writeGoClawEvent(conn, "zalo.personal.qr.done", map[string]any{
			"instance_id": instance, "success": false, "error": msg,
		})
		return
	}
	encoded, _ := json.Marshal(cred)
	_ = secrets.Put(st, zaloCookieSecretName(instance), encoded)
	_ = writeGoClawEvent(conn, "zalo.personal.qr.done", map[string]any{
		"instance_id": instance, "success": true, "cookies": len(cred.Cookies),
	})
}
