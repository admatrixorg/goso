// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contactsimport

import (
	"encoding/json"
	"fmt"
	"time"
)

// FixtureCount is the synthetic Minh An sample size. It is more than 200.
const FixtureCount = 210

// MinhAnFile returns FixtureCount generated contacts for tenant minh-an.
// The first 200 are telegram users. The last 10 are zalo-oa groups.
// Displays are "Minh An NNNN". Dests are ext-minh-an-NNNN or grp-minh-an-NNNN.
// There are no phone numbers, email addresses, or real people.
func MinhAnFile() File {
	base := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	contacts := make([]Contact, 0, FixtureCount)
	for i := 0; i < FixtureCount; i++ {
		first := base.Add(time.Duration(i) * time.Minute)
		last := first.Add(time.Hour)
		c := Contact{
			TenantID:  "minh-an",
			Display:   fmt.Sprintf("Minh An %04d", i),
			AgentID:   "minh-an-sales",
			SeenCount: 1 + i%4,
			FirstSeen: first.Format(time.RFC3339),
			LastSeen:  last.Format(time.RFC3339),
		}
		if i < 200 {
			c.Channel = "telegram"
			c.Dest = fmt.Sprintf("ext-minh-an-%04d", i)
			c.Kind = "user"
		} else {
			c.Channel = "zalo-oa"
			c.Dest = fmt.Sprintf("grp-minh-an-%04d", i)
			c.Kind = "group"
		}
		contacts = append(contacts, c)
	}
	return File{Contacts: contacts}
}

// FixtureJSON is the canonical bytes of MinhAnFile, with a trailing newline.
func FixtureJSON() []byte {
	b, err := json.MarshalIndent(MinhAnFile(), "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}
