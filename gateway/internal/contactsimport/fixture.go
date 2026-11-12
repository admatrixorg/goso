// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contactsimport

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	// FixtureDirect is the number of person rows with an empty thread_id.
	FixtureDirect = 200
	// FixtureThreads is the number of thread rows, including one that reuses
	// the first direct sender_id and one group.
	FixtureThreads = 10
	// FixtureMerged is the one already-merged row. It is not inserted.
	FixtureMerged = 1
)

// FixtureRows is every sample row. FixtureLive is the rows that are written.
const (
	FixtureRows = FixtureDirect + FixtureThreads + FixtureMerged
	FixtureLive = FixtureDirect + FixtureThreads
)

// SharedSender is the sender_id that also has a thread row.
const SharedSender = "ext-minh-an-0000"

// SharedThread is that row's thread_id.
const SharedThread = "thread-minh-an-0000"

// MergedSender is the sender_id of the already-merged sample row.
const MergedSender = "ext-minh-an-merged"

// MinhAnFile returns the synthetic Minh An channel_contacts sample.
// Displays are "Minh An ...". Sender ids are ext-minh-an-NNNN, one shared
// thread, and one merged row. There are no phone numbers or real people.
func MinhAnFile() File {
	base := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	contacts := make([]Contact, 0, FixtureRows)
	for i := 0; i < FixtureDirect; i++ {
		first := base.Add(time.Duration(i) * time.Minute)
		contacts = append(contacts, person(fmt.Sprintf("ext-minh-an-%04d", i), "", fmt.Sprintf("Minh An %04d", i), "user", "user", "", first))
	}
	// Eight thread rows with their own sender, the shared sender's thread, and one group thread.
	for i := 0; i < FixtureThreads-2; i++ {
		n := FixtureDirect + i
		first := base.Add(time.Duration(n) * time.Minute)
		sender := fmt.Sprintf("ext-minh-an-%04d", n)
		thread := fmt.Sprintf("thread-minh-an-%04d", n)
		contacts = append(contacts, person(sender, thread, fmt.Sprintf("Minh An Thread %04d", n), "user", "user", "", first))
	}
	sharedAt := base.Add(time.Duration(FixtureDirect+FixtureThreads) * time.Minute)
	contacts = append(contacts, person(SharedSender, SharedThread, "Minh An Thread 0000", "user", "user", "", sharedAt))
	groupAt := sharedAt.Add(time.Minute)
	group := person("grp-minh-an-0001", "thread-minh-an-g1", "Minh An Group 0001", "group", "group", "group", groupAt)
	group.ChannelType = "zalo-oa"
	contacts = append(contacts, group)
	mergedAt := groupAt.Add(time.Minute)
	merged := person(MergedSender, "", "Minh An Merged", "user", "user", "", mergedAt)
	merged.ID = "c0ffee0a-1b2c-4d3e-8f90-a1b2c3d4e5f6"
	merged.MergedID = "a1b2c3d4-e5f6-4789-a012-b3c4d5e6f708"
	contacts = append(contacts, merged)
	return File{Contacts: contacts}
}

func person(sender, thread, display, contactType, peerKind, threadType string, first time.Time) Contact {
	return Contact{
		TenantID:    "minh-an",
		ChannelType: "telegram",
		SenderID:    sender,
		DisplayName: display,
		PeerKind:    peerKind,
		FirstSeenAt: first.Format(time.RFC3339),
		LastSeenAt:  first.Add(time.Hour).Format(time.RFC3339),
		ContactType: contactType,
		ThreadID:    thread,
		ThreadType:  threadType,
	}
}

// FixtureJSON is the canonical bytes of MinhAnFile, with a trailing newline.
func FixtureJSON() []byte {
	b, err := json.MarshalIndent(MinhAnFile(), "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}
