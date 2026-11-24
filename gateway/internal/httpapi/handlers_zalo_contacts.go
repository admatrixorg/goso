package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// wsZaloContacts answers the picker on step 3 of the channel wizard. The page
// asks for the account's friends and groups so an operator can allow them.
func wsZaloContacts(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		InstanceID  string `json:"instance_id"`
		InstanceID2 string `json:"instanceId"`
		Channel     string `json:"channel"`
	}
	_ = json.Unmarshal(req.Params, &p)
	instance := strings.TrimSpace(firstNonEmpty(p.InstanceID, p.InstanceID2, p.Channel))
	if instance == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "instance_id is required")
	}
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	if cfg, err := st.GetChannelConfig(instance); err != nil || cfg == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "channel instance not found")
	}
	cred, cerr := storedZaloCredential(st, instance)
	if cerr != nil {
		return writeGoClawErr(conn, req.ID, "NOT_CONFIGURED", cerr.Error())
	}
	sess, serr := protocol.NewSessionWithCredential(cred)
	if serr != nil {
		return writeGoClawErr(conn, req.ID, "NOT_CONFIGURED", serr.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// The login handshake is required twice over: it hands out the service hosts
	// the contact routes live on, and it returns the payload key when the stored
	// credential did not carry one.
	if _, lerr := sess.GetLoginInfo(ctx); lerr != nil {
		return writeGoClawErr(conn, req.ID, "UNAUTHORIZED", lerr.Error())
	}
	if strings.TrimSpace(sess.SecretKey) == "" {
		return writeGoClawErr(conn, req.ID, "NOT_CONFIGURED",
			"the account gave no payload key, so signing in again with the QR code is needed")
	}
	contacts, ferr := sess.FetchContacts(ctx)
	if ferr != nil {
		return writeGoClawErr(conn, req.ID, "FAILED", ferr.Error())
	}
	payload := map[string]any{
		"instance_id": instance,
		"friends":     contacts.Friends,
		"groups":      contacts.Groups,
		"total":       len(contacts.Friends) + len(contacts.Groups),
	}
	return writeGoClaw(conn, req.ID, payload)
}
