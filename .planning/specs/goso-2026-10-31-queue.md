# SPEC queue 133+ — GOSO combined spec 2026-10-31

Source: `docs/qa/goso-spec-2026-10-31-gap.md`. Order follows §13 (phase 1 first), then the dependency each later phase has on the previous one. SPEC 132 is not merged; only SPEC 137 stacks on `admatrixmdp/spec132-memory-revive` while that remains true.

Ideas only from OpenKnowledge. No GPL-3.0 code. No Claude CLI / Codex OAuth on a commercial path. API keys only. Demo-only data must be labeled mock. Do not edit live Mac mini deploy config or secrets.

## Design rule (prototype 31/10)

SPECs 134+ that touch the control plane follow the interactive prototype “Giao diện Goclaw/Goso”, not a denser operator form.

- Tokens, scoped until an app-wide pass: `--bg #fff`, `--panel #fafbfc`, `--line #e8eaef`, `--fg #1d2433`, `--muted #636b7d`, `--faint #9aa1b0`, one accent `#2563eb` (dark: `#0f1218` / `#131720` / `#232936` / `#e6e9ef` / `#5b8cff`). Type is Inter at 13–14px, JetBrains Mono only for ids and code. Chat already scopes these on `.z-chat-split`. Adopting them on every page is a later SPEC, not a drive-by.
- One job per surface. No role badges, no “this API is missing” footers, no session ids in the title, no prompt-mode control on the chat screen.
- Chat: empty state is only “Chào buổi …” and a few suggestion chips. Thread max 740px. One rounded composer, fixed to the bottom. Agent and model are quiet text buttons inside it. Caption is a 12px muted `agent · model` line. “Chuyển sang X” is a short hairline, not a banner. Reply actions appear on hover.
- Vietnamese copy, with diacritics. Long names truncate. Focus is a 2px accent outline, not a heavy ring.

| SPEC | Phase | Scope | Acceptance |
|---|---|---|---|
| 133 | 1 | Chat: session agent picker, model picker default "Theo agent", @ for one message, reply caption `agent · model`, switch line. See `.planning/specs/133-chat-agent-model.md`. | Implemented on this branch. |
| 134 | 1 | Paste, drop, and Ctrl+U on the chat composer. Chips in the input (image thumb or name+type, remove on hover). Ctrl+V of images and long text. No Docling. Files the browser cannot read are not invented. | A dropped file is visible as a chip before send; send still works with text; oversize (>200 MB) is refused with Vietnamese copy; control-plane tests cover chip helpers; no secret in the chip label. |
| 135 | 1 | Vault wiki layers under `{scope}/wiki/{raw,sources,topics,_meta}` plus frontmatter `status` (`draft\|stable\|deprecated`) and `superseded_by`. Keep today's list/graph. | Creating a doc writes the layer path; list filter can show a layer; superseded doc stays readable; no OpenKnowledge import. |
| 136 | 1 | Docling ingest job: save original, markdown, heading split at 1 MB of markdown, source page, index/log update. If Docling is not configured, the doc stays `đang xử lý` / not configured — never a fake OCR body. | Upload PDF/markdown shows the path that will be written and a duplicate SHA-256 warning; a missing Docling URL does not invent text. |
| 137 | 1 | Memory Facts shadow store: subject, attribute, value, valid-from/to, asserted-by, confidence, source, status. Reconciliation create/extend/supersede/dispute/skip with authority admin > user > agent > extractor. Default shadow (score + log, no prompt injection). **Stack on SPEC 132 if it is still unmerged.** | A lower-authority fact cannot overwrite; the old row remains; shadow mode does not change chat; Go tests cover the five outcomes. |
| 138 | 1 | Memory screen, 7 tabs: Kiến trúc, Memory Facts, Episodic, Tài liệu, Truy hồi, Dreaming, Quản trị. Recall sliders warn when weights ≠ 100. Facts tab reads SPEC 137 and shows the shadow badge. | Tabs render empty/error without pretending embeddings are on; slider warning is unit-tested. |
| 139 | shell | Left nav: one **Agent Setting** with tabs Agent / Agent Team / Agent Link. Rename group labels to CHÍNH / KHẢ NĂNG, "Máy chủ MCP" → MCP, cron → Schedule Task. Keep `#/agents` and `#/teams` as aliases. Supersedes the Agents-vs-Teams split in SPEC 119 only. | `liveSide` has one core entry; both old hashes open the right tab; vi+en; hash tests. |
| 140 | 3 (start) | Decision in Khả năng: routing matrix R0–R3 × confidence, question bank, model chain placeholder (no Jev/Clef network calls unless a public hostname is configured), log. Default shadow. Hard rules cannot be disabled (R3 never auto, R2 at least logs, <70% auto only on R0). | A dry-run message writes a log row and does not branch the pipeline; refusing an action is not counted as "người sửa đáp án". |
| 141 | 3 | Decision Flow: three sample graphs (inbound message, pre-tool, before reply) on the existing graph viewer pattern. Run-test highlights the path. Edits stay in the question bank from 140. | Selecting a sample and running a message highlights nodes; a dashed loop is visible on the reply flow; no drag-create of new nodes. |
| 142 | 2 | Goso MCP read tools: `tra_cuu`, `tra_cuu_fact`, `boi_canh_phien`, `doc_tep`, `search`, `links`. Scope by caller. Labels công khai / nội bộ / mật / tối mật; mật and above never leave the platform. Access log: who, what, when, which platform. | A mật doc is absent from a tool result tagged external; the log row exists; no OpenKnowledge source. |
| 143 | 2 | Vault "Sức khoẻ" tab: orphans, broken links, missing citations, expired claims. Runnable by hand. Monthly Schedule Task hook can be a registered no-op until cron can call it. | A fixture vault with one orphan and one broken wikilink shows both; empty vault is an empty state, not a fake issue. |
| 144 | 3 | `ghi_nho` (through SPEC 137 reconciliation) and `ghi_quyet_dinh`. Decision may leave shadow only when the log has ≥300 rows and human answer-edits <10%. The gate is real counts, not a constant. | Below the gate, execute mode is refused; at the gate it is allowed; a refusal is not an answer-edit. |
| 145 | later | "Hiểu media" settings: provider chain for image/document/audio, per agent "theo model của agent" or the shared chain. Default display Gemini 2.5 Flash → GPT-4o as labels only. API keys, write-only. No Claude CLI / Codex. OCR for Vault stays in SPEC 136, not chat. | Saving a chain does not call a vendor; unset chain says ảnh đi theo model chính. |
| 146 | 1 remainder | Chat polish on top of 133: stop button aborts the stream, empty greeting, "Trò chuyện thường", retry with another model, one-line activity ("Đã đọc n tệp") that expands on click. Image fallback to a vision model only if SPEC 145 configured a chain; otherwise say the model cannot read the image. | Abort stops the local stream; greeting is Vietnamese and uses the signed-in label when present; retry sends one new turn with the picked model and does not change the session default. |
| 147 | 4 | AI Workers page, 3 tabs by direction, plus a multi-model room. Rows without a real ACP/API connection are badges **mô phỏng**. No personal-subscription providers. | The page cannot show a green "connected" for Claude CLI or Codex OAuth. |
| 148 | §1 / §12 | Top bar: timezone, account menu (org list, org, API keys link, about, logout). Optional follow-up: one monitoring timeline with filters LLM / tool / admin. Do not hide Nodes/MCP until 139 has shipped and an operator asks. | Language and theme still work; logout clears the local token only; timeline is a filter over existing trace/event/activity APIs, not a new store. |

## Not scheduled

User-Chrome extension and 8-character pairing (§2.5), entity vault, meeting ingest, Muse/Dots connectors, SME billing, and the LarkBase routing workbook. Those stay in §14 / phase 4 until the phase 1 brain answers fund-policy questions from versioned facts.

## Definition of done

Each SPEC names the page and API it changes, keeps vi+en, states empty/error, and does not echo secrets. UI copy stays consistent with the current Vietnamese strings. Gateway tests for store/HTTP changes. `npm test` and `npm run verify` in `control-plane/` for UI changes.
