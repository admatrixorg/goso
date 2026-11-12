# SPEC 171 — Docling converter

Evidence for the ingest converter. No control-plane tab change, so prototype screenshots do not apply. No customer data and no secret values. Env vars are named, not filled in. Fixtures are synthetic Minh An files. MQG-249.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `GOSO_VAULT_DOCLING` | off | Uploaded jobs stay `uploaded`. The sidecar is not called |
| `GOSO_VAULT_DOCLING_URL` | empty | Internal sidecar base URL. Empty or a refused connection fails the job as `not_configured` |
| `GOSO_VAULT_DOCLING_PAGE_TIMEOUT` | 60s | Per-page limit. A Go duration or a number of seconds. Not measured on VPS2 |
| `GOSO_VAULT_DOCLING_PAGE_ATTEMPTS` | 2 | Tries before `timeout`. Values above 10 are capped |
| `GOSO_VAULT_DOCLING_PAGE_BUDGET` | 32 | Page slots the gateway waits for one sidecar call. Cap 500. Not measured on VPS2 |

`GOSO_VAULT_UPLOAD` is unchanged and still defaults off. The converter flag does not exempt any HTTP body from `LimitAPI`.

Compose profile `docling` is off unless requested. Dev ceilings in `compose.yml`, not VPS2 measurements: 1 CPU, 2 GiB RAM, no extra swap. The sidecar publishes no host port and joins only the internal network `docling_internal`. Its vault mount is read-only and limited to the `vault` subpath. The process user is uid 10001.

Compose sets `GOSO_VAULT_DIR` to `/data/vault` so the gateway and the sidecar share one vault. A host-run gateway is unchanged: it still uses `GOSO_VAULT_DIR` or `data/vault`.

## Forget (SPEC 164)

This PR does not delete a conversion when a person is forgotten. SPEC 164 still has to delete `raw/`, the ingest job, `sources/`, and derived wiki pages. Turning `GOSO_VAULT_DOCLING` off leaves converted files in place and does not drop columns.

## Tests

Postgres for the local `go test` was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here. SQLite cases run in the same command.

From the repo root:

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package reported `ok` or `[no test files]`.

[go-test.txt](go-test.txt)

Verbose cases, including memory, SQLite, and Postgres where the store is involved:

[go-test-cases.txt](go-test-cases.txt)

| Criterion | Test |
|---|---|
| PDF, docx, xlsx, pptx, and a scan image become markdown that contains the Minh An phrase. OCR jobs record `tesseract` and `vie+eng`. The phrase is not stored on the job row. Raw bytes are unchanged | `TestConvert_MinhAnFixturesAndText` |
| txt and md skip the sidecar, become NFC and LF, and land in `sources/inbox/` through `PutTenantAt` | `TestConvert_MinhAnFixturesAndText` |
| Whitespace or digits with no letter is `empty_conversion` and raw remains. A one-letter file (`Ơ`) is kept | `TestConvert_EmptyKeepsRawAndShortFileSurvives` |
| Markdown over 1 MiB is split on headings, each part is at most 1 MiB, UTF-8 is not cut, and joining the parts matches | `TestConvert_LargeMarkdownSplitsOnHeadings` |
| Flag off does not call the sidecar. The job stays `uploaded` | `TestConvert_FlagOffDoesNotCallSidecar` |
| Sidecar down or URL empty is `not_configured`. Raw stays. `GET /healthz` stays 200 | `TestConvert_SidecarMissingFailsClosed`, `TestVaultIngest_SidecarDownDoesNotFailHealth` |
| Another tenant's `GET` is 404 and does not contain `source_paths` | `TestVaultIngest_ConvertedFieldsStayInTenant` |
| A hung page ends as `timeout` after 2 attempts and the job does not stay `converting` | `TestConvert_HungPageTimesOut`, sidecar `tests/test_pages.py` |
| EasyOCR is rejected. `ocr_failed`, `unsupported`, and `too_large` are stored as those classes | `TestConvert_WrongEngineAndClasses` |
| A conversion that would pass the disk quota does not write `sources/` | `TestConvert_QuotaBlocksSources` |
| A failed job can be retried without a second raw file. A converted file dedupes | `TestConvert_RetryAfterFailureDoesNotRewriteRaw` |
| A restart fails leftover `converting` jobs as `internal` | `TestConvert_ReapDoesNotLeaveConverting` |
| The HTTP client sends tenant, raw path, mime, timeout, and attempts. It does not send the file bytes or the local path | `TestConvert_HTTPClientOmitsFileBytes` |
| A sidecar body over the response cap is `too_large` | `TestConvert_HTTPBodyCapIsTooLarge` |
| Dockerfile runtime command has no `wget`/`curl`, sets `HF_HUB_OFFLINE`, runs as uid 10001, and the page worker selects Tesseract `vie`+`eng`. Compose has the CPU, RAM, internal network, and read-only vault mount | `TestDoclingSidecarPolicy` |
| Missing models return `not_configured` and the converter source does not call a downloader | sidecar `tests/test_offline.py` |
| The sidecar resolves only `t/<tenant>/raw/<file>` and refuses escape and symlinks | sidecar `tests/test_jail.py` |
| New columns are additive on SQLite and Postgres, including an old table that predates them | `TestIngestJobTableAdditive`, `TestIngestJobConvertColumnsAddedToOldTable` |
| Rename failure plus rollback failure returns both errors | `TestPutTenantAt_RollbackKeepsRenameError` |

`TestConvert_MinhAnFixturesAndText` and `TestIngestJobTableAdditive` passed on memory, SQLite, and Postgres. The Minh An phrase in those Go tests is produced by the fake converter. It is not a Tesseract run.

Sidecar unit tests, no network:

```bash
PYTHONPATH=sidecar/docling python3 -m unittest discover -s sidecar/docling/tests -v
```

[python-test.txt](python-test.txt)

`go test` does not start Docling and does not download models.

## Cổng trước khi bật cờ

This is the gate before turning `GOSO_VAULT_DOCLING` on on any machine. It is not a gate for this PR. None of the seven proofs below has been done yet.

1. A successful `docker build`, with the image digest.
2. `docker run --network none` as uid 10001 converting Minh An fixtures: one text PDF, one docx, and one scanned image with diacritics. The diacritic phrase must appear in the markdown.
3. The runtime log, with no download and no outbound HTTP lines.
4. `pip list` inside the image showing no `easyocr`, and an explicit statement of whether `rapidocr` or `onnxruntime` is present.
5. `id` output showing uid 10001.
6. The real license files from inside the image: `/usr/share/doc/tesseract-ocr*/copyright` and the model licenses under `/opt/docling/models`, with `licenses.md` updated to match those files.
7. Image size and peak RAM from `docker stats` while converting the fixtures.

## What this PR did not run

- The Docling image was not built and was not started. A network-blocked container did not convert a fixture. The Dockerfile bakes models in the build stage, the runtime command is only `python -m goso_docling`, and `HF_HUB_OFFLINE=1` is set. The server returns `not_configured` when artifacts or `vie`/`eng` tessdata are missing. That is the code path. It is not an observation of a built image.
- Tesseract `vie` quality on a real Vietnamese scan was not measured. The PNG fixture is a synthetic signature. The phrase in its markdown comes from the fake converter.
- CPU, RAM, and disk headroom on VPS2 were not measured.
- Upstream Docling images were not checked for a runtime model download. This PR only describes the Goso Dockerfile.
- Office files are one timed conversion in the sidecar. Per-page `page_range` is applied to PDF. A slide or sheet is not split out as its own page.
- License files inside a built image were not opened. The table in [licenses.md](licenses.md) is the intended bill of materials. Prod stays off until that table is checked against a built image, anh Dat approves the flag, and the SPEC 170 disk quota is set.

## qa:tabs

[qa-tabs.log](qa-tabs.log)

`qa:tabs` ran on the tree of `1a47f84538075bcf10075573018c33fd70a5f574` and exited 0. The log records that SHA as `tested-head`. The commit that adds this sentence does not change the gateway. Chrome: the binary at `/usr/local/bin/google-chrome`. `QA_SKIP_TABS` was not set. `QA_SHOTS` was not set.

No vault tab markup changed, so there are no prototype screenshots.

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080`. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_VAULT_UPLOAD`, `GOSO_VAULT_DOCLING`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

## Rollback

Turn `GOSO_VAULT_DOCLING` off. Converted files stay. Raw files stay. The new columns stay.
