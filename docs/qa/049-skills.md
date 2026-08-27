# QA — SPEC 049 Skills loader + use_skill (fail-closed)

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

K6 was MCP `/v1/skills` without a gateway loader. This SPEC adds a one-level `SKILL.md` reader and builtin `use_skill`. **No script exec.**

## What changed

- Env `GOSO_SKILLS_DIR`. Empty/unset → no filesystem walk. Builtin `use_skill` returns `not_configured`. `GET /api/skills` returns `{skills:[]}`.
- When set: scan **one level** of subdirectories for `SKILL.md` (name = folder). Root `SKILL.md` and nested folders are ignored. Body cap **64KiB** (truncated). Extra files (`run.sh`, etc.) are never executed or returned.
- Path jail: reject `..`, absolute names, and anything that escapes the skills dir. If `GOSO_WORKSPACE` is set, the skills dir and files must also sit inside that jail. Symlinks that resolve outside the dir are skipped/rejected.
- Builtin catalog now includes `use_skill` `{name}` (`connector: "builtin"`, no approval). Advertised on `GET /api/agents/{id}/tools` with the other builtins.
- `GET /api/skills` `{skills:[{name, path}]}` names only. Optional `?name=` returns `{name, path, body}`. Missing → 404 `not_found`. Escape → 400 `path escape`. Unconfigured `?name=` → `{error:"not_configured"}`.
- Control-plane Functions: Skills card (StatusLine loading/error, EmptyState). i18n vi+en.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- `gateway/internal/skill`: tempdir one `SKILL.md`; empty env fail-closed; `../` and absolute names rejected; workspace jail; symlink escape skipped; 64KiB cap.
- `gateway/internal/builtin`: catalog length 5 includes `use_skill`; empty env `not_configured`; tempdir invoke returns body.
- httptest `GET /api/skills` empty list / list names / `?name=` body / path escape 400 / missing 404; `POST /api/tools/invoke` builtin `use_skill`.

## Non-goals

MCP `/v1/skills` rewrite, `skill_manage`, BM25 `skill_search`, executing skill scripts, live demo bind/kill, copying goclaw/ZaloCRM.
