// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"strings"
	"time"
	// Embedded zone data, so the tool works on hosts without /usr/share/zoneinfo.
	_ "time/tzdata"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

// ToolDatetime returns the current date and time.
const ToolDatetime = "datetime"

// DefaultTimezone is used when the call names no timezone.
const DefaultTimezone = "Asia/Ho_Chi_Minh"

// Now is the clock of the datetime tool. Tests replace it.
var Now = time.Now

func invokeDatetime(args map[string]any) (*connector.InvokeResult, error) {
	name := DefaultTimezone
	for _, k := range []string{"timezone", "tz"} {
		if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
			name = strings.TrimSpace(v)
			break
		}
	}
	loc, err := time.LoadLocation(name)
	// LoadLocation also accepts "" and "Local"; only real IANA names are wanted.
	if err != nil || name == "Local" {
		return &connector.InvokeResult{
			Tool:      ToolDatetime,
			Connector: ConnectorName,
			Status:    "error",
			Content:   map[string]any{"error": "invalid timezone: use an IANA name such as Asia/Ho_Chi_Minh"},
		}, nil
	}
	now := Now().In(loc)
	return &connector.InvokeResult{
		Tool:      ToolDatetime,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"timezone":   name,
			"iso":        now.Format(time.RFC3339),
			"date":       now.Format("2006-01-02"),
			"time":       now.Format("15:04:05"),
			"weekday":    now.Weekday().String(),
			"utc_offset": now.Format("-07:00"),
			"unix":       now.Unix(),
		},
	}, nil
}
