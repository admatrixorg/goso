// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextimport

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

//go:embed testdata/minh-an.json
var minhAnFixture []byte

const (
	sourceTenant = "11111111-1111-4111-8111-111111111111"
	otherSource  = "22222222-2222-4222-8222-222222222222"
	destTenant   = "minh-an"
	otherDest    = "minh-an-legal"

	salesRule    = "Chiết khấu tối đa của Minh An là 9 phần trăm."
	soulText     = "Cố vấn Minh An nói ngắn, tiếng Việt."
	legalRule    = "Pháp chế Minh An không báo chiết khấu."
	lanNote      = "Lan chỉ nhận báo giá một dòng."
	hoaNote      = "Hoa giữ bản báo giá dài."
	instructions = "Tra loi ngan gon."
	memoryBody   = "Minh An ghi nhớ báo giá ngắn cho đại lý mẫu."
	legalKept    = "Pháp chế giữ nguyên."
	legalUser    = "Lan bên pháp chế giữ nguyên."
	lanNotes     = "Lan giữ điều khoản mẫu của pháp chế."

	salesAgentID   = "11111111-1111-4111-8111-111111111101"
	phapAgentID    = "11111111-1111-4111-8111-111111111102"
	branchAgentID  = "11111111-1111-4111-8111-111111111103"
	missingAgentID = "11111111-1111-4111-8111-111111111104"
	exportRowID    = "99999999-9999-4999-8999-999999999999"
)

func TestReadExportDropsUnknownFields(t *testing.T) {
	ex, err := ReadExport(bytes.NewReader(minhAnFixture))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "EMBEDMARKERvector") {
		t.Fatalf("embedding kept: %s", raw)
	}
	if strings.Contains(string(raw), exportRowID) {
		t.Fatalf("goclaw id kept: %s", raw)
	}
	if len(ex.Agents) != 6 || len(ex.Users) != 6 {
		t.Fatalf("rows agents=%d users=%d", len(ex.Agents), len(ex.Users))
	}
	if ex.Agents[0].AgentID != salesAgentID || ex.Agents[0].FileName != "AGENTS.md" || ex.Users[0].UserID != "lan" {
		t.Fatalf("columns %+v %+v", ex.Agents[0], ex.Users[0])
	}
}

func TestExportTooLarge(t *testing.T) {
	_, err := readExport(bytes.NewReader([]byte(`{"agent_context_files":[]}`)), 8)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err %v", err)
	}
}

func TestMinhAnFixture(t *testing.T) {
	ex := mustFixture(t)
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			puts := &putCount{StoreIface: st}
			seed := seedMinhAn(t, puts)
			puts.n = 0
			log := &bytes.Buffer{}

			dry, err := Import(puts, ex, minhAnOpt(t, false, printfLog{log}))
			if err != nil {
				t.Fatal(err)
			}
			if !dry.DryRun || dry.Inserted != 6 || dry.Updated != 0 || dry.Skipped != 0 || dry.Rejected != 6 {
				t.Fatalf("dry-run %+v", dry)
			}
			if puts.n != 0 {
				t.Fatalf("dry-run puts %d", puts.n)
			}
			assertAbsent(t, st, seed)
			assertLogHidesBodies(t, log.String())
			if strings.Contains(log.String(), "EMBEDMARKERvector") {
				t.Fatalf("log kept embedding: %s", log.String())
			}

			log.Reset()
			opt := minhAnOpt(t, true, printfLog{log})
			got, err := Import(puts, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			if got.DryRun || got.Inserted != 6 || got.Updated != 0 || got.Skipped != 0 || got.Rejected != 6 {
				t.Fatalf("apply %+v", got)
			}
			if puts.n != 6 {
				t.Fatalf("apply puts %d", puts.n)
			}
			assertLogHidesBodies(t, log.String())
			assertImported(t, st, seed)
			assertReasons(t, got)

			before := snapshot(t, st, seed)
			puts.n = 0
			again, err := Import(puts, ex, minhAnOpt(t, true, nil))
			if err != nil {
				t.Fatal(err)
			}
			if again.Inserted != 0 || again.Updated != 0 || again.Skipped != 6 || again.Rejected != 6 {
				t.Fatalf("rerun %+v", again)
			}
			if puts.n != 0 {
				t.Fatalf("rerun puts %d", puts.n)
			}
			after := snapshot(t, st, seed)
			if before != after {
				t.Fatalf("rerun changed rows\nbefore %s\nafter  %s", before, after)
			}
			assertImported(t, st, seed)

			assertPrompt(t, st, seed)
			mem, err := st.GetMemory(seed.mem.ID)
			if err != nil || mem.Body != memoryBody || mem.TenantID != destTenant {
				t.Fatalf("memory changed: %+v %v", mem, err)
			}
			sales, err := st.GetAgent(seed.sales.ID)
			if err != nil || sales.Instructions != instructions || sales.AgentKey != "tu-van-sales" {
				t.Fatalf("agent changed: %+v %v", sales, err)
			}
		})
	}
}

func TestDuplicateLastWinsIsIdempotent(t *testing.T) {
	ex := &Export{Agents: []AgentFile{
		{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "ban mot"},
		{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "ban hai"},
	}}
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			puts := &putCount{StoreIface: st}
			sales, err := puts.CreateAgent(store.Agent{
				AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := Import(puts, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if first.Inserted != 1 || first.Updated != 0 || first.Skipped != 1 || first.Rejected != 0 {
				t.Fatalf("first %+v", first)
			}
			if puts.n != 1 {
				t.Fatalf("puts %d", puts.n)
			}
			got, err := st.GetContextFile(destTenant, store.ContextScopeAgent, sales.ID, "AGENTS.md")
			if err != nil || got.Body != "ban hai" {
				t.Fatalf("stored %+v %v", got, err)
			}
			listed, err := st.ListContextFiles(destTenant, store.ContextScopeAgent, sales.ID)
			if err != nil || len(listed) != 1 {
				t.Fatalf("list %+v %v", listed, err)
			}
			stamp := got.ID + "|" + got.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + got.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			puts.n = 0
			second, err := Import(puts, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if second.Inserted != 0 || second.Updated != 0 || second.Skipped != 2 || puts.n != 0 {
				t.Fatalf("rerun %+v puts %d", second, puts.n)
			}
			again, err := st.GetContextFile(destTenant, store.ContextScopeAgent, sales.ID, "AGENTS.md")
			if err != nil {
				t.Fatal(err)
			}
			gotStamp := again.ID + "|" + again.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + again.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			if again.Body != "ban hai" || gotStamp != stamp {
				t.Fatalf("rerun wrote id/time/body %s %q", gotStamp, again.Body)
			}

			changed := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "ban ba", UpdatedAt: "2026-05-03T06:07:08Z"},
			}}
			puts.n = 0
			third, err := Import(puts, changed, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if third.Updated != 1 || third.Inserted != 0 || puts.n != 1 {
				t.Fatalf("update %+v puts %d", third, puts.n)
			}
			updated, err := st.GetContextFile(destTenant, store.ContextScopeAgent, sales.ID, "AGENTS.md")
			wantUpdated := time.Date(2026, 5, 3, 6, 7, 8, 0, time.UTC)
			if err != nil || updated.ID != got.ID || updated.Body != "ban ba" || !updated.UpdatedAt.Equal(wantUpdated) || !updated.CreatedAt.Equal(got.CreatedAt) {
				t.Fatalf("replaced row %+v %v", updated, err)
			}
			listed, err = st.ListContextFiles(destTenant, store.ContextScopeAgent, sales.ID)
			if err != nil || len(listed) != 1 {
				t.Fatalf("still one row %+v %v", listed, err)
			}
		})
	}
}

func TestOwnerCapAndTooLarge(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			ag, err := st.CreateAgent(store.Agent{
				AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			var files []AgentFile
			for i := 1; i <= 17; i++ {
				files = append(files, AgentFile{
					TenantID: sourceTenant,
					AgentID:  "tu-van-sales",
					FileName: fmt.Sprintf("n%02d.md", i),
					Content:  fmt.Sprintf("ghi chu %02d", i),
				})
			}
			files = append(files, AgentFile{
				TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "big.md",
				Content: strings.Repeat("x", store.MaxContextFileBytes+1),
			})
			rep, err := Import(st, &Export{Agents: files}, opt(t, Options{
				SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Inserted != 16 || rep.Rejected != 2 {
				t.Fatalf("cap %+v", rep)
			}
			listed, err := st.ListContextFiles(destTenant, store.ContextScopeAgent, ag.ID)
			if err != nil || len(listed) != 16 {
				t.Fatalf("stored %d %v", len(listed), err)
			}
			if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "n17.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("17th stored: %v", err)
			}
			if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "big.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("oversize stored: %v", err)
			}
			puts := &putCount{StoreIface: st}
			again, err := Import(puts, &Export{Agents: files}, opt(t, Options{
				SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if again.Inserted != 0 || again.Updated != 0 || again.Skipped != 16 || again.Rejected != 2 || puts.n != 0 {
				t.Fatalf("rerun %+v puts %d", again, puts.n)
			}

			wide := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentID: "tu-van-bytes", FileName: "a.md", Content: strings.Repeat("a", store.MaxContextFileBytes)},
				{TenantID: sourceTenant, AgentID: "tu-van-bytes", FileName: "b.md", Content: strings.Repeat("b", store.MaxContextFileBytes)},
				{TenantID: sourceTenant, AgentID: "tu-van-bytes", FileName: "c.md", Content: "c"},
			}}
			// The 16 short files already fill the file-count cap, so replace
			// that owner with a fresh agent for the byte cap.
			other, err := st.CreateAgent(store.Agent{
				AgentKey: "tu-van-bytes", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			byteRep, err := Import(st, wide, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if byteRep.Inserted != 2 || byteRep.Rejected != 1 {
				t.Fatalf("bytes %+v", byteRep)
			}
			got, err := st.ListContextFiles(destTenant, store.ContextScopeAgent, other.ID)
			if err != nil || len(got) != 2 {
				t.Fatalf("byte files %+v %v", got, err)
			}
			if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, other.ID, "c.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("byte cap wrote c.md")
			}
		})
	}
}

func TestInvalidRowsWriteNothing(t *testing.T) {
	ex := &Export{
		Agents: []AgentFile{
			{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "../AGENTS.md", Content: "no"},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "a/b.md", Content: "no"},
			{TenantID: sourceTenant, AgentID: "", FileName: "AGENTS.md", Content: "no"},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "   "},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "no", CreatedAt: "not-a-time"},
		},
		Users: []UserFile{
			{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan nguyen", FileName: "USER.md", Content: "no"},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan@minh-an.test", FileName: "USER.md", Content: "no"},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan", FileName: "USER.md", Content: ""},
			{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: strings.Repeat("a", 65), FileName: "USER.md", Content: "no"},
		},
	}
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			puts := &putCount{StoreIface: st}
			if _, err := puts.CreateAgent(store.Agent{
				AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			}); err != nil {
				t.Fatal(err)
			}
			rep, err := Import(puts, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Inserted != 0 || rep.Updated != 0 || rep.Rejected != 9 || puts.n != 0 {
				t.Fatalf("invalid %+v puts %d", rep, puts.n)
			}
			for _, row := range rep.Rejections {
				if row.Reason != reasonInvalid {
					t.Fatalf("reason %+v", row)
				}
			}
		})
	}
}

func TestUserFileConflictDoesNotOverwrite(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			if _, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.CreateAgent(store.Agent{AgentKey: "phap-che", DisplayName: "Pháp chế Minh An", TenantID: destTenant}); err != nil {
				t.Fatal(err)
			}
			kept, err := st.PutContextFile(store.ContextFile{
				TenantID: destTenant, Scope: store.ContextScopeUser, UserKey: "lan",
				Name: "USER.md", Body: "giu nguyen",
			})
			if err != nil {
				t.Fatal(err)
			}
			ex := &Export{Users: []UserFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan", FileName: "USER.md", Content: "ban mot"},
				{TenantID: sourceTenant, AgentID: "phap-che", UserID: "lan", FileName: "USER.md", Content: "ban hai"},
			}}
			puts := &putCount{StoreIface: st}
			rep, err := Import(puts, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Inserted != 0 || rep.Updated != 0 || rep.Rejected != 2 || puts.n != 0 {
				t.Fatalf("conflict %+v puts %d", rep, puts.n)
			}
			for _, row := range rep.Rejections {
				if row.Reason != reasonScopeConflict {
					t.Fatalf("reason %+v", row)
				}
			}
			got, err := st.GetContextFile(destTenant, store.ContextScopeUser, "lan", "USER.md")
			if err != nil || got.Body != "giu nguyen" || got.ID != kept.ID {
				t.Fatalf("stored %+v %v", got, err)
			}
			same := &Export{Users: []UserFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan", FileName: "NOTES.md", Content: "cung mot"},
				{TenantID: sourceTenant, AgentID: "phap-che", UserID: "lan", FileName: "NOTES.md", Content: "cung mot"},
			}}
			puts.n = 0
			once, err := Import(puts, same, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if once.Inserted != 1 || once.Skipped != 1 || puts.n != 1 {
				t.Fatalf("same body %+v puts %d", once, puts.n)
			}
			puts.n = 0
			twice, err := Import(puts, same, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if twice.Inserted != 0 || twice.Updated != 0 || twice.Skipped != 2 || puts.n != 0 {
				t.Fatalf("rerun %+v puts %d", twice, puts.n)
			}
		})
	}
}

func TestJSONLUsesTheSameColumns(t *testing.T) {
	raw := strings.Join([]string{
		`{"tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"tu-van-sales","file_name":"AGENTS.md","content":"tu jsonl","created_at":"2026-03-01T04:05:06Z","updated_at":"2026-04-02T05:06:07Z"}`,
		`{"tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"tu-van-sales","user_id":"lan","file_name":"USER.md","content":"lan jsonl","created_at":"2026-03-01T04:05:06Z","updated_at":"2026-04-02T05:06:07Z"}`,
	}, "\n")
	ex, err := ReadExport(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Agents) != 1 || len(ex.Users) != 1 || ex.Users[0].UserID != "lan" || ex.Agents[0].FileName != "AGENTS.md" {
		t.Fatalf("jsonl %+v", ex)
	}
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant}); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})); err != nil {
		t.Fatal(err)
	}
	ag, err := st.ListContextFiles(destTenant, store.ContextScopeAgent, mustAgent(t, st, "tu-van-sales"))
	if err != nil || len(ag) != 1 || ag[0].Body != "tu jsonl" || !ag[0].CreatedAt.Equal(time.Date(2026, 3, 1, 4, 5, 6, 0, time.UTC)) {
		t.Fatalf("agent %+v %v", ag, err)
	}
	lan, err := st.GetContextFile(destTenant, store.ContextScopeUser, cust("lan"), "USER.md")
	if err != nil || lan.Body != "lan jsonl" {
		t.Fatalf("user %+v %v", lan, err)
	}
}

func mustAgent(t *testing.T, st store.StoreIface, key string) string {
	t.Helper()
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key {
			return a.ID
		}
	}
	t.Fatal("missing agent")
	return ""
}

func TestDenylistStopsBeforeWrite(t *testing.T) {
	st := store.New()
	puts := &putCount{StoreIface: st}
	base := Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}
	missing := filepath.Join(t.TempDir(), "missing-deny.txt")
	_, err := Import(puts, &Export{}, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true, DenyFile: missing})
	var fileErr *FileError
	if !errors.As(err, &fileErr) || fileErr.Kind != "missing" || puts.n != 0 {
		t.Fatalf("missing %v puts %d", err, puts.n)
	}
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte(" \n# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Import(puts, &Export{}, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true, DenyFile: empty})
	if !errors.As(err, &fileErr) || fileErr.Kind != "empty" || puts.n != 0 {
		t.Fatalf("empty %v puts %d", err, puts.n)
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(bad, []byte("not a tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Import(puts, &Export{}, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true, DenyFile: bad})
	if !errors.As(err, &fileErr) || fileErr.Kind != "malformed" || puts.n != 0 {
		t.Fatalf("malformed %v puts %d", err, puts.n)
	}
	listed := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(listed, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(puts, &Export{}, Options{SourceTenant: sourceTenant, DestTenant: "blocked-tenant", Apply: true, DenyFile: listed})
	var denied *DenyError
	if !errors.As(err, &denied) || denied.Count != 1 || puts.n != 0 || rep == nil || rep.DeniedTenants != 1 {
		t.Fatalf("dest denied %v %+v puts %d", err, rep, puts.n)
	}
	if strings.Contains(err.Error(), "blocked-tenant") {
		t.Fatal("error names the tenant")
	}
	rep, err = Import(puts, &Export{}, Options{SourceTenant: "blocked-tenant", DestTenant: destTenant, Apply: true, DenyFile: listed})
	if !errors.As(err, &denied) || denied.Count != 1 || puts.n != 0 {
		t.Fatalf("source denied %v %+v puts %d", err, rep, puts.n)
	}
	_ = base
}

func TestDeniedExportRowIsOmitted(t *testing.T) {
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
	}); err != nil {
		t.Fatal(err)
	}
	deny := filepath.Join(t.TempDir(), "deny.txt")
	if err := os.WriteFile(deny, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := &Export{
		Agents: []AgentFile{
			{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "kept"},
			{TenantID: "blocked-tenant", AgentID: "tu-van-sales", FileName: "SECRET.md", Content: "nope"},
		},
		Users: []UserFile{
			{TenantID: "blocked-tenant", AgentID: "tu-van-sales", UserID: "lan", FileName: "USER.md", Content: "nope"},
		},
	}
	log := &bytes.Buffer{}
	rep, err := Import(st, ex, Options{
		SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true,
		DenyFile: deny, Log: printfLog{log},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DeniedTenants != 1 || rep.Inserted != 1 || rep.Updated != 0 || rep.Rejected != 0 {
		t.Fatalf("report %+v", rep)
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "blocked-tenant") || strings.Contains(log.String(), "blocked-tenant") {
		t.Fatalf("denied tenant leaked report %s log %s", raw, log.String())
	}
	got, err := st.GetContextFile(destTenant, store.ContextScopeAgent, mustAgent(t, st, "tu-van-sales"), "AGENTS.md")
	if err != nil || got.Body != "kept" {
		t.Fatalf("kept %+v %v", got, err)
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, got.AgentID, "SECRET.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("denied row stored: %v", err)
	}
}

func TestEditedImportIsNotOverwritten(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			ag, err := st.CreateAgent(store.Agent{
				AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			first := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "imported"},
			}}
			if _, err := Import(st, first, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})); err != nil {
				t.Fatal(err)
			}
			if _, err := st.PutContextFile(store.ContextFile{
				TenantID: destTenant, Scope: store.ContextScopeAgent, AgentID: ag.ID,
				Name: "AGENTS.md", Body: "hand edit",
			}); err != nil {
				t.Fatal(err)
			}
			again := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "new export"},
			}}
			rep, err := Import(st, again, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true}))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Updated != 0 || rep.Inserted != 0 || rep.Skipped != 1 {
				t.Fatalf("rerun %+v", rep)
			}
			if len(rep.Agents) != 1 || rep.Agents[0].Reasons[reasonNativeExists] != 1 {
				t.Fatalf("reasons %+v", rep.Agents)
			}
			got, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "AGENTS.md")
			if err != nil || got.Body != "hand edit" {
				t.Fatalf("edit lost %+v %v", got, err)
			}
		})
	}
}

func TestNativeFileSurvivesImportAndRollback(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			ag, err := st.CreateAgent(store.Agent{
				AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			native, err := st.PutContextFile(store.ContextFile{
				TenantID: destTenant, Scope: store.ContextScopeAgent, AgentID: ag.ID,
				Name: "AGENTS.md", Body: "real file",
			})
			if err != nil {
				t.Fatal(err)
			}
			ex := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "AGENTS.md", Content: "imported text"},
				{TenantID: sourceTenant, AgentID: "tu-van-sales", FileName: "SOUL.md", Content: "soul text"},
			}}
			deny := testDeny(t)
			rep, err := Import(st, ex, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true, DenyFile: deny})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Inserted != 1 || rep.Skipped != 1 || rep.Updated != 0 {
				t.Fatalf("import %+v", rep)
			}
			if rep.Agents[0].Reasons[reasonNativeExists] != 1 {
				t.Fatalf("reasons %+v", rep.Agents)
			}
			got, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "AGENTS.md")
			if err != nil || got.Body != "real file" || got.ID != native.ID {
				t.Fatalf("native changed %+v %v", got, err)
			}
			if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "SOUL.md"); err != nil {
				t.Fatal(err)
			}
			first, err := Rollback(st, destTenant, deny, true)
			if err != nil {
				t.Fatal(err)
			}
			if first.Deleted != 1 || first.Kept != 0 {
				t.Fatalf("rollback %+v", first)
			}
			got, err = st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "AGENTS.md")
			if err != nil || got.Body != "real file" || got.ID != native.ID {
				t.Fatalf("native after rollback %+v %v", got, err)
			}
			if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "SOUL.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("imported file remained: %v", err)
			}
			second, err := Rollback(st, destTenant, deny, true)
			if err != nil {
				t.Fatal(err)
			}
			if second.Deleted != 0 || second.Kept != 0 {
				t.Fatalf("second rollback %+v", second)
			}
			got, err = st.GetContextFile(destTenant, store.ContextScopeAgent, ag.ID, "AGENTS.md")
			if err != nil || got.Body != "real file" {
				t.Fatalf("native after second rollback %+v %v", got, err)
			}
		})
	}
}

func TestCustomerKeyMap(t *testing.T) {
	links, err := ReadCustomerMap(strings.NewReader(`[{"channel":"zalo","sender_id":"s-lan","user_id":"lan"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if customerKey(links, "lan") != "zalo:s-lan" || customerKey(links, "hoa") != "goclaw:hoa" {
		t.Fatalf("keys %q %q", customerKey(links, "lan"), customerKey(links, "hoa"))
	}
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: destTenant}); err != nil {
		t.Fatal(err)
	}
	ex := &Export{Users: []UserFile{
		{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "lan", FileName: "USER.md", Content: "mot"},
		{TenantID: sourceTenant, AgentID: "tu-van-sales", UserID: "hoa", FileName: "USER.md", Content: "hai"},
	}}
	if _, err := Import(st, ex, opt(t, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true, Customers: links})); err != nil {
		t.Fatal(err)
	}
	lan, err := st.GetContextFile(destTenant, store.ContextScopeUser, "zalo:s-lan", "USER.md")
	if err != nil || lan.Body != "mot" {
		t.Fatalf("mapped %+v %v", lan, err)
	}
	hoa, err := st.GetContextFile(destTenant, store.ContextScopeUser, "goclaw:hoa", "USER.md")
	if err != nil || hoa.Body != "hai" {
		t.Fatalf("fallback %+v %v", hoa, err)
	}
}

func TestBadOptionsWriteNothing(t *testing.T) {
	st := store.New()
	puts := &putCount{StoreIface: st}
	_, err := Import(puts, &Export{}, opt(t, Options{DestTenant: destTenant, Apply: true}))
	var cfg *ConfigError
	if !errors.As(err, &cfg) || puts.n != 0 {
		t.Fatalf("err %v puts %d", err, puts.n)
	}
	_, err = Import(puts, &Export{}, opt(t, Options{SourceTenant: sourceTenant, DestTenant: "not a tenant", Apply: true}))
	if !errors.As(err, &cfg) || puts.n != 0 {
		t.Fatalf("dest err %v puts %d", err, puts.n)
	}
}

type seed struct {
	sales  *store.Agent
	phap   *store.Agent
	branch *store.Agent
	legal  *store.Agent
	mem    *store.Memory
	sess   *store.Session
}

func seedMinhAn(t *testing.T, st store.StoreIface) seed {
	t.Helper()
	sales, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An",
		TenantID: destTenant, Instructions: instructions,
	})
	if err != nil {
		t.Fatal(err)
	}
	phap, err := st.CreateAgent(store.Agent{
		AgentKey: "phap-che", DisplayName: "Pháp chế Minh An",
		TenantID: destTenant, Instructions: instructions,
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := st.CreateAgent(store.Agent{
		AgentKey: "chi-nhanh", DisplayName: "Chi nhánh Minh An", TenantID: otherDest,
	})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-legal", DisplayName: "Pháp chế Minh An",
		TenantID: otherDest, Instructions: instructions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: otherDest, Scope: store.ContextScopeAgent, AgentID: legal.ID,
		Name: "AGENTS.md", Body: legalKept,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: otherDest, Scope: store.ContextScopeUser, UserKey: "lan",
		Name: "USER.md", Body: legalUser,
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: destTenant})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := st.PutMemory(store.Memory{
		SessionID: sess.ID, TenantID: destTenant, Kind: store.KindDurable, Body: memoryBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	return seed{sales: sales, phap: phap, branch: branch, legal: legal, mem: mem, sess: sess}
}

func assertAbsent(t *testing.T, st store.StoreIface, seed seed) {
	t.Helper()
	for _, owner := range []struct{ scope, id, name string }{
		{store.ContextScopeAgent, seed.sales.ID, "AGENTS.md"},
		{store.ContextScopeAgent, seed.sales.ID, "SOUL.md"},
		{store.ContextScopeAgent, seed.phap.ID, "AGENTS.md"},
		{store.ContextScopeUser, cust("lan"), "USER.md"},
		{store.ContextScopeUser, cust("lan"), "NOTES.md"},
		{store.ContextScopeUser, cust("hoa"), "USER.md"},
	} {
		if _, err := st.GetContextFile(destTenant, owner.scope, owner.id, owner.name); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("dry-run wrote %s %s: %v", owner.scope, owner.name, err)
		}
	}
}

func assertImported(t *testing.T, st store.StoreIface, seed seed) {
	t.Helper()
	sales, err := st.ListContextFiles(destTenant, store.ContextScopeAgent, seed.sales.ID)
	if err != nil || len(sales) != 2 || sales[0].Name != "AGENTS.md" || sales[0].Body != salesRule || sales[1].Name != "SOUL.md" || sales[1].Body != soulText {
		t.Fatalf("sales files %+v %v", sales, err)
	}
	for _, f := range sales {
		if f.TenantID != destTenant || f.AgentID != seed.sales.ID || f.UserKey != "" {
			t.Fatalf("sales scope %+v", f)
		}
	}
	phap, err := st.GetContextFile(destTenant, store.ContextScopeAgent, seed.phap.ID, "AGENTS.md")
	if err != nil || phap.Body != legalRule || phap.AgentID != seed.phap.ID {
		t.Fatalf("phap %+v %v", phap, err)
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, seed.sales.ID, "NOTES.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("sales received an extra file")
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, seed.phap.ID, "SOUL.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("phap received the sales file")
	}
	lan, err := st.GetContextFile(destTenant, store.ContextScopeUser, cust("lan"), "USER.md")
	if err != nil || lan.Body != lanNote || lan.UserKey != cust("lan") || lan.AgentID != "" || lan.TenantID != destTenant {
		t.Fatalf("lan %+v %v", lan, err)
	}
	if lan.ID == exportRowID {
		t.Fatal("stored the goclaw uuid")
	}
	created := time.Date(2026, 3, 1, 4, 5, 6, 0, time.UTC)
	updated := time.Date(2026, 4, 2, 5, 6, 7, 0, time.UTC)
	salesFile, err := st.GetContextFile(destTenant, store.ContextScopeAgent, seed.sales.ID, "AGENTS.md")
	if err != nil || !salesFile.CreatedAt.Equal(created) || !salesFile.UpdatedAt.Equal(updated) {
		t.Fatalf("clock %+v %v", salesFile, err)
	}
	notes, err := st.GetContextFile(destTenant, store.ContextScopeUser, cust("lan"), "NOTES.md")
	if err != nil || notes.Body != lanNotes || notes.UserKey != cust("lan") || notes.AgentID != "" {
		t.Fatalf("lan notes %+v %v", notes, err)
	}
	hoa, err := st.GetContextFile(destTenant, store.ContextScopeUser, cust("hoa"), "USER.md")
	if err != nil || hoa.Body != hoaNote || hoa.UserKey != cust("hoa") {
		t.Fatalf("hoa %+v %v", hoa, err)
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeUser, cust("lan"), "AGENTS.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("user received an agent file")
	}
	branch, err := st.ListContextFiles(otherDest, store.ContextScopeAgent, seed.branch.ID)
	if err != nil || len(branch) != 0 {
		t.Fatalf("other tenant agent received files %+v %v", branch, err)
	}
	kept, err := st.GetContextFile(otherDest, store.ContextScopeAgent, seed.legal.ID, "AGENTS.md")
	if err != nil || kept.Body != legalKept {
		t.Fatalf("legal file changed %+v %v", kept, err)
	}
	keptUser, err := st.GetContextFile(otherDest, store.ContextScopeUser, "lan", "USER.md")
	if err != nil || keptUser.Body != legalUser {
		t.Fatalf("legal user changed %+v %v", keptUser, err)
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeAgent, seed.branch.ID, "AGENTS.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("branch agent id was used in the destination tenant")
	}
	for _, f := range append(append([]*store.ContextFile{}, sales...), phap, lan, hoa) {
		if strings.Contains(f.Body, "FOREIGN") || strings.Contains(f.Body, "OTHERAGENT") || strings.Contains(f.Body, "MISSING") || strings.Contains(f.Body, "EMBED") {
			t.Fatalf("marker stored in %+v", f)
		}
	}
}

func assertReasons(t *testing.T, rep *Report) {
	t.Helper()
	want := map[string]int{
		reasonAgentOtherTenant: 2,
		reasonAgentMissing:     2,
		reasonTenantMismatch:   2,
	}
	got := map[string]int{}
	for _, row := range rep.Rejections {
		got[row.Reason]++
		if strings.Contains(row.Name, "MARKER") || strings.Contains(row.Owner, "MARKER") || row.Owner == "lan" || row.Owner == "hoa" {
			t.Fatalf("rejection carries a body or user id %+v", row)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("reasons %+v", got)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("reason %s = %d", k, got[k])
		}
	}
	var agents, users TenantSection
	for _, s := range rep.Agents {
		if s.SourceTenant == sourceTenant {
			agents = s
		}
	}
	for _, s := range rep.Users {
		if s.SourceTenant == sourceTenant {
			users = s
		}
	}
	if agents.Input != 5 || agents.Inserted != 3 || agents.Rejected != 2 || !agents.balanced() {
		t.Fatalf("agent section %+v", agents)
	}
	if users.Input != 5 || users.Inserted != 3 || users.Rejected != 2 || !users.balanced() {
		t.Fatalf("user section %+v", users)
	}
}

func assertPrompt(t *testing.T, st store.StoreIface, seed seed) {
	t.Helper()
	want, err := contextfile.Prompt(st, destTenant, seed.sales.ID, cust("lan"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(want, "AGENTS.md:\n"+salesRule) || !strings.Contains(want, "SOUL.md:\n"+soulText) || !strings.Contains(want, "USER.md:\n"+lanNote) {
		t.Fatalf("prompt block %q", want)
	}
	if strings.Contains(want, hoaNote) || strings.Contains(want, legalRule) || strings.Contains(want, legalKept) {
		t.Fatalf("prompt block leaked %q", want)
	}

	t.Run("flag off", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "")
		seen := &scripted{}
		res, err := pipeline.NewRunner(st, nil, seen, nil).Run(context.Background(), seed.sess.ID, "Chiết khấu tối đa là bao nhiêu?", pipeline.ModeFull)
		if err != nil {
			t.Fatal(err)
		}
		sys := systemText(seen.msgs)
		if strings.Contains(sys, salesRule) || strings.Contains(sys, lanNote) || strings.Contains(sys, want) {
			t.Fatalf("flag off loaded context files: %q", sys)
		}
		if !strings.Contains(sys, instructions) {
			t.Fatalf("instructions missing: %q", sys)
		}
		if res.Reply != "không rõ" {
			t.Fatalf("reply %q", res.Reply)
		}
	})

	t.Run("flag on loads the imported files", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "1")
		seen := &scripted{}
		ctx := contextfile.WithUser(context.Background(), cust("lan"))
		res, err := pipeline.NewRunner(st, nil, seen, nil).Run(ctx, seed.sess.ID, "Chiết khấu tối đa là bao nhiêu?", pipeline.ModeFull)
		if err != nil {
			t.Fatal(err)
		}
		sys := systemText(seen.msgs)
		if !strings.Contains(sys, want) {
			t.Fatalf("assembly did not load the imported prompt block\nwant %q\ngot %q", want, sys)
		}
		if strings.Contains(sys, hoaNote) || strings.Contains(sys, legalRule) || strings.Contains(sys, "FOREIGN") || strings.Contains(sys, legalKept) {
			t.Fatalf("assembly leaked %q", sys)
		}
		if !strings.Contains(sys, instructions) || !strings.Contains(sys, "Cố vấn Minh An") {
			t.Fatalf("existing prompt dropped: %q", sys)
		}
		if res.Reply != salesRule+" "+lanNote {
			t.Fatalf("reply %q", res.Reply)
		}
	})
}

func assertLogHidesBodies(t *testing.T, log string) {
	t.Helper()
	for _, marker := range []string{salesRule, soulText, legalRule, lanNote, lanNotes, hoaNote, "FOREIGNAGENTMARKER", "FOREIGNUSERMARKER", "OTHERAGENTMARKER", "OTHERUSERMARKER", "MISSINGAGENTMARKER", "MISSINGUSERMARKER", `owner="lan"`, `owner="hoa"`} {
		if strings.Contains(log, marker) {
			t.Fatalf("log contains %q: %s", marker, log)
		}
	}
}

func snapshot(t *testing.T, st store.StoreIface, seed seed) string {
	t.Helper()
	var b strings.Builder
	write := func(scope, owner, name string) {
		t.Helper()
		f, err := st.GetContextFile(destTenant, scope, owner, name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s\n", f.ID, f.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), f.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), f.Name, f.Body)
	}
	write(store.ContextScopeAgent, seed.sales.ID, "AGENTS.md")
	write(store.ContextScopeAgent, seed.sales.ID, "SOUL.md")
	write(store.ContextScopeAgent, seed.phap.ID, "AGENTS.md")
	write(store.ContextScopeUser, cust("lan"), "USER.md")
	write(store.ContextScopeUser, cust("lan"), "NOTES.md")
	write(store.ContextScopeUser, cust("hoa"), "USER.md")
	kept, err := st.GetContextFile(otherDest, store.ContextScopeAgent, seed.legal.ID, "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&b, "legal|%s|%s\n", kept.ID, kept.Body)
	return b.String()
}

func minhAnOpt(t *testing.T, apply bool, log Logger) Options {
	return opt(t, Options{
		SourceTenant: sourceTenant,
		DestTenant:   destTenant,
		Apply:        apply,
		Log:          log,
		AgentMap: map[string]string{
			salesAgentID:  "tu-van-sales",
			phapAgentID:   "phap-che",
			branchAgentID: "chi-nhanh",
		},
	})
}

func opt(t *testing.T, o Options) Options {
	t.Helper()
	if o.DenyFile == "" {
		o.DenyFile = testDeny(t)
	}
	return o
}

func testDeny(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "deny.txt")
	if err := os.WriteFile(p, []byte("not-this-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func cust(id string) string { return "goclaw:" + id }

func mustFixture(t *testing.T) *Export {
	t.Helper()
	ex, err := ReadExport(bytes.NewReader(minhAnFixture))
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

type scripted struct {
	msgs []llm.Message
}

func (s *scripted) Name() string { return "scripted" }

func (s *scripted) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	s.msgs = append([]llm.Message(nil), msgs...)
	sys := systemText(msgs)
	if strings.Contains(sys, salesRule) && strings.Contains(sys, lanNote) {
		return salesRule + " " + lanNote, nil
	}
	return "không rõ", nil
}

func systemText(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type putCount struct {
	store.StoreIface
	n int
}

func (p *putCount) PutContextFile(in store.ContextFile) (*store.ContextFile, error) {
	p.n++
	return p.StoreIface.PutContextFile(in)
}

type printfLog struct{ b *bytes.Buffer }

func (p printfLog) Printf(format string, args ...any) {
	fmt.Fprintf(p.b, format+"\n", args...)
}

func backends(t *testing.T) map[string]func(t *testing.T) store.StoreIface {
	t.Helper()
	out := map[string]func(t *testing.T) store.StoreIface{
		"memory": func(t *testing.T) store.StoreIface { return store.New() },
		"sqlite": func(t *testing.T) store.StoreIface {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "ctx.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	}
	if postgresDSN(t) != "" {
		out["postgres"] = func(t *testing.T) store.StoreIface {
			return openPostgres(t)
		}
	}
	return out
}

func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" && (os.Getenv("CI") == "true" || os.Getenv("CI") == "1") {
		t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
	}
	return dsn
}

var schemaSeq atomic.Int64

func openPostgres(t *testing.T) store.StoreIface {
	t.Helper()
	dsn := postgresDSN(t)
	schema := fmt.Sprintf("ctximport_%d", schemaSeq.Add(1))
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	pg, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	return pg
}
