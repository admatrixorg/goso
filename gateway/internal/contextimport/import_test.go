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
	if len(ex.Agents) != 6 || len(ex.Users) != 3 {
		t.Fatalf("rows agents=%d users=%d", len(ex.Agents), len(ex.Users))
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

			dry, err := Import(puts, ex, Options{
				SourceTenant: sourceTenant,
				DestTenant:   destTenant,
				Log:          printfLog{log},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !dry.DryRun || dry.Inserted != 5 || dry.Updated != 0 || dry.Skipped != 0 || dry.Rejected != 4 {
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
			got, err := Import(puts, ex, Options{
				SourceTenant: sourceTenant,
				DestTenant:   destTenant,
				Apply:        true,
				Log:          printfLog{log},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.DryRun || got.Inserted != 5 || got.Updated != 0 || got.Skipped != 0 || got.Rejected != 4 {
				t.Fatalf("apply %+v", got)
			}
			if puts.n != 5 {
				t.Fatalf("apply puts %d", puts.n)
			}
			assertLogHidesBodies(t, log.String())
			assertImported(t, st, seed)
			assertReasons(t, got)

			before := snapshot(t, st, seed)
			puts.n = 0
			again, err := Import(puts, ex, Options{
				SourceTenant: sourceTenant,
				DestTenant:   destTenant,
				Apply:        true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if again.Inserted != 0 || again.Updated != 0 || again.Skipped != 5 || again.Rejected != 4 {
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
		{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "AGENTS.md", Body: "ban mot"},
		{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "AGENTS.md", Body: "ban hai"},
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
			first, err := Import(puts, ex, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})
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
			second, err := Import(puts, ex, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})
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
				{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "AGENTS.md", Body: "ban ba"},
			}}
			puts.n = 0
			third, err := Import(puts, changed, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})
			if err != nil {
				t.Fatal(err)
			}
			if third.Updated != 1 || third.Inserted != 0 || puts.n != 1 {
				t.Fatalf("update %+v puts %d", third, puts.n)
			}
			updated, err := st.GetContextFile(destTenant, store.ContextScopeAgent, sales.ID, "AGENTS.md")
			if err != nil || updated.ID != got.ID || updated.Body != "ban ba" {
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
					AgentKey: "tu-van-sales",
					Name:     fmt.Sprintf("n%02d.md", i),
					Body:     fmt.Sprintf("ghi chu %02d", i),
				})
			}
			files = append(files, AgentFile{
				TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "big.md",
				Body: strings.Repeat("x", store.MaxContextFileBytes+1),
			})
			rep, err := Import(st, &Export{Agents: files}, Options{
				SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true,
			})
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
			again, err := Import(puts, &Export{Agents: files}, Options{
				SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if again.Inserted != 0 || again.Updated != 0 || again.Skipped != 16 || again.Rejected != 2 || puts.n != 0 {
				t.Fatalf("rerun %+v puts %d", again, puts.n)
			}

			wide := &Export{Agents: []AgentFile{
				{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "a.md", Body: strings.Repeat("a", store.MaxContextFileBytes)},
				{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "b.md", Body: strings.Repeat("b", store.MaxContextFileBytes)},
				{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "c.md", Body: "c"},
			}}
			// The 16 short files already fill the file-count cap, so replace
			// that owner with a fresh agent for the byte cap.
			other, err := st.CreateAgent(store.Agent{
				AgentKey: "tu-van-bytes", DisplayName: "Cố vấn Minh An", TenantID: destTenant,
			})
			if err != nil {
				t.Fatal(err)
			}
			for i := range wide.Agents {
				wide.Agents[i].AgentKey = "tu-van-bytes"
			}
			byteRep, err := Import(st, wide, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})
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
			{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "../AGENTS.md", Body: "no"},
			{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "a/b.md", Body: "no"},
			{TenantID: sourceTenant, AgentKey: "", Name: "AGENTS.md", Body: "no"},
			{TenantID: sourceTenant, AgentKey: "tu-van-sales", Name: "AGENTS.md", Body: "   "},
		},
		Users: []UserFile{
			{TenantID: sourceTenant, UserKey: "lan nguyen", Name: "USER.md", Body: "no"},
			{TenantID: sourceTenant, UserKey: "lan@minh-an.test", Name: "USER.md", Body: "no"},
			{TenantID: sourceTenant, UserKey: "lan", Name: "USER.md", Body: ""},
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
			rep, err := Import(puts, ex, Options{SourceTenant: sourceTenant, DestTenant: destTenant, Apply: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Inserted != 0 || rep.Updated != 0 || rep.Rejected != 7 || puts.n != 0 {
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

func TestBadOptionsWriteNothing(t *testing.T) {
	st := store.New()
	puts := &putCount{StoreIface: st}
	_, err := Import(puts, &Export{}, Options{DestTenant: destTenant, Apply: true})
	var cfg *ConfigError
	if !errors.As(err, &cfg) || puts.n != 0 {
		t.Fatalf("err %v puts %d", err, puts.n)
	}
	_, err = Import(puts, &Export{}, Options{SourceTenant: sourceTenant, DestTenant: "not a tenant", Apply: true})
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
		{store.ContextScopeUser, "lan", "USER.md"},
		{store.ContextScopeUser, "hoa", "USER.md"},
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
	lan, err := st.GetContextFile(destTenant, store.ContextScopeUser, "lan", "USER.md")
	if err != nil || lan.Body != lanNote || lan.UserKey != "lan" || lan.AgentID != "" || lan.TenantID != destTenant {
		t.Fatalf("lan %+v %v", lan, err)
	}
	hoa, err := st.GetContextFile(destTenant, store.ContextScopeUser, "hoa", "USER.md")
	if err != nil || hoa.Body != hoaNote || hoa.UserKey != "hoa" {
		t.Fatalf("hoa %+v %v", hoa, err)
	}
	if _, err := st.GetContextFile(destTenant, store.ContextScopeUser, "lan", "AGENTS.md"); !errors.Is(err, store.ErrNotFound) {
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
		reasonAgentOtherTenant: 1,
		reasonAgentMissing:     1,
		reasonTenantMismatch:   2,
	}
	got := map[string]int{}
	for _, row := range rep.Rejections {
		got[row.Reason]++
		if strings.Contains(row.Name, "MARKER") || strings.Contains(row.Owner, "MARKER") {
			t.Fatalf("rejection carries a body marker %+v", row)
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
	if users.Input != 2 || users.Inserted != 2 || users.Rejected != 0 || !users.balanced() {
		t.Fatalf("user section %+v", users)
	}
}

func assertPrompt(t *testing.T, st store.StoreIface, seed seed) {
	t.Helper()
	want, err := contextfile.Prompt(st, destTenant, seed.sales.ID, "lan")
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
		ctx := contextfile.WithUser(context.Background(), "lan")
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
	for _, marker := range []string{salesRule, soulText, legalRule, lanNote, hoaNote, "FOREIGNAGENTMARKER", "FOREIGNUSERMARKER", "OTHERAGENTMARKER", "MISSINGAGENTMARKER"} {
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
	write(store.ContextScopeUser, "lan", "USER.md")
	write(store.ContextScopeUser, "hoa", "USER.md")
	kept, err := st.GetContextFile(otherDest, store.ContextScopeAgent, seed.legal.ID, "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&b, "legal|%s|%s\n", kept.ID, kept.Body)
	return b.String()
}

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
