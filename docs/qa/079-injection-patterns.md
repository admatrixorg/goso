# QA — SPEC 079 Injection pattern completeness

Date: 2026-08-29. Clean-room Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not paste goclaw Go. Do not copy goclaw regex tables. Do not merge. Do not start SPEC 080.

Closes matrix **S2, S7** (041 shipped four of six documented phrases). Production default block already **066**.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go. Do not copy regex tables from the cite.

| Behavior | Cite |
|----------|------|
| Input layer scans more prompt-injection phrases than a four-substring list; production fail-closed injection action | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` |

goso mapping (self-written): `security.ScanInjection` is case-insensitive `strings.Contains` over **six** goso-owned documented substrings (not a regex port). `GOSO_INJECTION=log|block` is unchanged. Production (`GOSO_ENV=production`) still defaults to **block** when unset (066). Demo / unset still defaults to **log**. Block → **400** `injection blocked` on `POST /api/chat`.

## Six exact documented substrings

Keep the four from 041, then two goso-owned phrases covering remaining classes:

| Class | Exact substring (lowercase as stored) |
|-------|----------------------------------------|
| instruction-override | `ignore previous instructions` |
| prompt-exfil | `exfiltrate system prompt` |
| destructive-sql | `drop table` |
| credential-dump | `dump credentials` |
| role-override | `you are now` |
| delimiter-escape | `end of system` |

Match is case-insensitive contains on the same `ScanInjection` path. Benign text (`hello, book a meeting`) does not match.

## What changed

- `gateway/internal/security/injection.go`: `injectionPatterns` length 6.
- Tests: `TestScanInjection_SixPatterns` (renamed from `FourPatterns`); each of the six matches; benign no match; `TestInspectChat_ProductionDefaultBlock` and demo/default log unchanged. `GOSO_INJECTION` gates not weakened.
- This QA lists the six exact strings. No goclaw regex.

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 080.

## Proof

- Six matches + benign: `TestScanInjection_SixPatterns`.
- Demo / unset default **log**: `TestInspectChat_LogAndBlock`.
- Production default **block** (066): `TestInspectChat_ProductionDefaultBlock`.
- HTTP log allows / block 400: `TestChat_InjectionLogAllows`, `TestChat_InjectionBlock400`.
- SSE injection stays JSON 400: `TestChatSSE_QuotaAndInjectionStayJSON`.

## Non-goals

Copying goclaw regex or Go. Weakening `GOSO_INJECTION` production/demo defaults. Merge. SPEC 080. Binding/killing demo ports. Product secrets in git.
