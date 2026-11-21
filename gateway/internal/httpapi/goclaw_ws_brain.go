package httpapi

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/brain"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleGoClawBrain(conn *websocket.Conn, req goclawReq, st store.StoreIface, tenant string) error {
	tenant = store.NormalizeTenant(tenant)
	switch req.Method {
	case "brain.pages", "wiki.list":
		pages, err := brain.List(st, tenant)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "ERROR", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"tenant": tenant, "pages": pages})
	case "brain.search", "wiki.search":
		var p struct {
			Query string `json:"query"`
			Q     string `json:"q"`
		}
		_ = json.Unmarshal(req.Params, &p)
		q := p.Query
		if q == "" {
			q = p.Q
		}
		hits, err := brain.Search(st, tenant, q, 20)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "ERROR", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"hits": hits})
	case "brain.get", "wiki.get":
		var p struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(req.Params, &p)
		id := p.ID
		if id == "" {
			id = p.Slug
		}
		page, err := brain.Get(st, tenant, id)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "not found")
		}
		return writeGoClaw(conn, req.ID, page)
	case "brain.graph", "wiki.graph":
		var sql *store.SQLiteStore
		switch s := st.(type) {
		case *store.SQLiteStore:
			sql = s
		case *store.PostgresStore:
			sql = s.SQLiteStore
		}
		if sql == nil {
			return writeGoClaw(conn, req.ID, map[string]any{"edges": []any{}})
		}
		edges, err := sql.ListBrainGraph(tenant, 200)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "ERROR", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"edges": edges})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}
