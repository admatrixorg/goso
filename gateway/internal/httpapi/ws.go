// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// RegisterWS registers the WebSocket echo endpoint on the given mux.
func RegisterWS(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		// session_id is optional for echo; validate if required later.
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Echo with prefix.
			reply := "echo: " + string(msg)
			if err := conn.WriteMessage(mt, []byte(reply)); err != nil {
				return
			}
		}
	})
}
