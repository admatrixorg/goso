# SPEC 086 — sandbox / browser / media when configured (DI-12/13/21)

> After 085. Clean-room. Do not kill `:8082` `:8091` `:3000` `:18080`.
> Missing image/Chrome/ffmpeg → **`not_configured`**, never spawn blindly.

Closes **K2 / K5** leftover: tools already advertised as stubs (074).

## GoClaw cite (docs only)

`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` runtime `exec` / media; isolation Docker sandbox.
`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` no-shell exec, timeout.

## goso plan

1. **sandbox** (`exec` group): if `GOSO_SANDBOX_IMAGE` non-empty **and** `docker` binary exists, run `docker run --rm --network=none --memory=256m` with timeout 15s, args `{cmd}` **as argv list not `sh -c`**. Workspace mount only if `GOSO_WORKSPACE` set (`-v abs:abs:ro` or rw documented). Else `not_configured`. No Docker API vendor SDK required — `exec.CommandContext("docker", ...)`.
2. **browser**: if `GOSO_BROWSER_BIN` (or `CHROME_PATH`) points to an **existing file**, spawn with `--headless --disable-gpu --no-sandbox` timeout 20s, args `{url}` after `security.CheckURL` (066 SSRF). Capture stdout truncated 64KiB. Else `not_configured`. Never launch if bin missing.
3. **media / image_gen / tts**: if `GOSO_FFMPEG` (or `ffmpeg` on PATH **and** `GOSO_MEDIA=1`) exists, allow a **documented no-vendor** path: tts/image still `not_configured` without a test double (keep 074). **media** may run `ffmpeg -version` as health or a local file transcode **inside GOSO_WORKSPACE** only (`{in, out}` jailed). Paid APIs remain DI-21 parked. If ffmpeg missing → `not_configured`.
4. `Configured()` true only when the binary/image is actually present.
5. Tests: empty env → not_configured, no process. Fake `docker`/`chrome`/`ffmpeg` scripts on PATH with GOSO_* pointing at them — assert argv, timeout, jail. SSRF blocks browser localhost when `GOSO_SSRF=1`.
6. QA `docs/qa/086-sandbox-browser-media.md`. Do not pull images in unit tests.

QC: typecheck if CP, go test, build, gofmt, vet, agpl, agpl-docs.
Commit `admatrixmdp/spec086-sandbox-browser-media`. Do not merge. Do not start 087.
