# QA — SPEC 086 sandbox / browser / media when configured (DI-12/13/21)

Date: 2026-08-29. Clean-room Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 087.

Closes matrix **K2 / K5** leftover: tools already advertised as stubs (074). Missing image/Chrome/ffmpeg → **`not_configured`**, never spawn blindly.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Runtime `exec`; isolation Docker sandbox routing | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (runtime `exec` / media; isolation Docker sandbox) |
| No-shell exec (`exec.CommandContext(binary, args...)`, never `sh -c`); timeout; Docker sandbox `--network none` + memory limit | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` (Layer 3 credentialed exec no-shell + timeout; Layer 5 Docker sandbox) |
| SSRF 3-step before URL fetch | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` (Layer 3 SSRF protection) |
| Media generation tools exist; paid vendors remain out of scope here | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (Media Generation) |

goso mapping (self-written): builtin `sandbox` `{cmd}` is `exec.CommandContext("docker", "run", "--rm", "--network=none", "--memory=256m", …)` with a 15s context timeout. `{cmd}` is an argv list (string = one token, array = tokens); never `sh -c`. Requires **both** non-empty `GOSO_SANDBOX_IMAGE` **and** a `docker` binary on PATH. If `GOSO_WORKSPACE` is set, mount `-v abs:abs:ro` (read-only bind; writes do not persist to the host). Else `not_configured`, no spawn. No Docker API SDK.

Builtin `browser` `{url}`: `GOSO_BROWSER_BIN` or `CHROME_PATH` must be an **existing file**. Always `security.CheckURL` first (066). Then spawn `--headless --disable-gpu --no-sandbox` timeout 20s. Stdout truncated at 64KiB. Missing file → `not_configured`, never launch. `GOSO_SSRF=1` blocks `http://127.0.0.1/` and `http://localhost/` without spawning.

Builtin `media`: if `GOSO_FFMPEG` is an existing file, or `ffmpeg` is on PATH **and** `GOSO_MEDIA=1`, run `ffmpeg -version` (health) or `{in,out}` transcode jailed inside `GOSO_WORKSPACE`. Explicit `GOSO_FFMPEG` pointing at a missing path does not fall through to PATH. `image_gen` / `tts` stay `not_configured` without a process-injected test double (keep 074). Paid media APIs remain DI-21 parked. Missing ffmpeg → `not_configured`.

`Configured(name)` is true only when the binary or image is actually present (sandbox: image + docker; browser: existing bin file; media: ffmpeg file). Independent of the Functions UI enabled flag.

## What changed

- `sandbox` / `browser` / `media` leave the 074 always-stub path. Empty env still fail-closed with no process.
- Fake `docker` / `chrome` / `ffmpeg` scripts on PATH (or pointed at by `GOSO_*`) are the unit-test stand-ins. Tests never call `docker pull` and never use a real image.
- Control-plane Functions workspace note (vi+en) documents the env gates. Catalog descriptions and input schemas updated (`cmd` / `url` / `{in,out}`).
- SETUP / `.env.example` / gateway help / DEPLOY overlay rows: opt-in env, not compose Chrome/sandbox services.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 087.

## Proof

- Empty env: `not_configured`, no process (`TestInvoke_SandboxEmptyEnvNoProcess`, `TestInvoke_BrowserEmptyEnvNoProcess`, `TestInvoke_MediaMissingFFmpeg`, `TestInvoke_SandboxNeverSpawns`). `Configured` false (`TestConfigured_SandboxBrowserMediaEmpty`).
- Image without docker / docker without image → `not_configured` (`TestInvoke_SandboxImageWithoutDocker`, `TestInvoke_SandboxDockerWithoutImage`).
- Fake docker on PATH + `GOSO_SANDBOX_IMAGE`: argv is `run --rm --network=none --memory=256m IMAGE echo hello`, never `sh -c` (`TestInvoke_SandboxFakeDockerArgv`). Workspace mount `-v abs:abs:ro` (`TestInvoke_SandboxWorkspaceMountRO`). Timeout (`TestInvoke_SandboxTimeout`). Empty cmd does not spawn (`TestInvoke_SandboxCmdRequired`).
- Fake chrome via `GOSO_BROWSER_BIN` / `CHROME_PATH`: `--headless --disable-gpu --no-sandbox` + url (`TestInvoke_BrowserFakeBinArgv`, `TestInvoke_BrowserChromePathFallback`). Missing file never launches (`TestInvoke_BrowserMissingBinNoSpawn`). Timeout (`TestInvoke_BrowserTimeout`).
- `GOSO_SSRF=1` blocks browser localhost / `127.0.0.1` with public error containing `ssrf` and no spawn (`TestInvoke_BrowserSSRFBlocksLocalhost`).
- `GOSO_FFMPEG` health `-version`; PATH ffmpeg requires `GOSO_MEDIA=1`; `{in,out}` jailed; empty workspace transcode is `not_configured` (`TestInvoke_MediaGOSOFFMPEGHealth`, `TestInvoke_MediaPATHFFmpegRequiresGOSOMEDIA`, `TestInvoke_MediaTranscodeJail`, `TestInvoke_MediaTranscodeEmptyWorkspace`).
- `image_gen` / `tts` still need env + test double (`TestInvoke_MediaFailClosedUnlessDouble`, health test asserts they stay `not_configured` when only ffmpeg is present).
- `GET /api/agents/{id}/tools` still lists `sandbox` with `configured:false` when env is empty (`TestAgentTools_ListAndPatchBuiltin`).

## Non-goals

Docker API vendor SDK. `docker pull` in unit tests. Compose Chrome / sandbox overlays. Paid media vendors (DI-21). HTML-to-Markdown. Merge. SPEC 087.
