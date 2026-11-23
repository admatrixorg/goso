package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerPairedDeviceRoutes serves the device registry the pairing page reads.
func registerPairedDeviceRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/pairing/devices", handlePairedDeviceList(st))
	mux.HandleFunc("DELETE /v1/pairing/devices/{id}", handlePairedDeviceDelete(st))
}

func handlePairedDeviceList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.PairedDevices(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"devices": []any{}, "total": 0})
			return
		}
		rows := x.ListPairedDevices(requestTenant(r))
		writeJSON(w, http.StatusOK, map[string]any{"devices": rows, "total": len(rows)})
	}
}

func handlePairedDeviceDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.PairedDevices(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "device registry unavailable")
			return
		}
		if err := x.DeletePairedDevice(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// pairDeviceFromBody records the device that claimed a code. The body is the
// same one the exchange already reads, so an older client that sends only a code
// keeps working and simply leaves no device row.
func pairDeviceFromBody(st store.StoreIface, tenant string, raw []byte, role string) (*store.PairedDevice, error) {
	x := store.PairedDevices(st)
	if x == nil || len(raw) == 0 {
		return nil, nil
	}
	var body struct {
		SenderID  string `json:"sender_id"`
		SenderID2 string `json:"senderId"`
		Channel   string `json:"channel"`
		ChatID    string `json:"chat_id"`
		ChatID2   string `json:"chatId"`
		PairedBy  string `json:"paired_by"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, nil
	}
	sender := firstNonEmpty(body.SenderID, body.SenderID2)
	if strings.TrimSpace(sender) == "" {
		return nil, nil
	}
	return x.InsertPairedDevice(store.PairedDevice{
		TenantID: tenant, SenderID: sender, Channel: strings.TrimSpace(body.Channel),
		ChatID:   firstNonEmpty(body.ChatID, body.ChatID2),
		PairedBy: firstNonEmpty(body.PairedBy, role, "operator"),
	})
}

var _ = auth.ErrPairingExpired
