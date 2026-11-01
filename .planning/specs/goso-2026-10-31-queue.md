# SPEC queue — GOSO spec v2 (2026-11-01)

Read with `GOSO_SPEC_2026-10-31.md`. A v2 section marked **THAY THẾ** replaces the old one. This file follows **v2 §11**, not the 31/10 phase list. SPEC 133 (chat agent / model / @) stays the slice already on this branch. The old 134–148 sequence in this file is retired; do not implement it in parallel.

UI reference is `Goso_Prototype_2026-11-01.html` (version 24). Sample data is **Minh An (demo)** only: household-goods distribution, teams `kinh-doanh` / `phap-che`, agents Cố vấn pháp lý · Cố vấn chiến lược · Cố vấn tài chính · Trợ lý giám đốc. Do not put MQ fund, ARR, or “Chủ tịch” examples in new code or copy.

Ideas only from OpenKnowledge. No GPL-3.0 code. No Claude CLI / Codex OAuth on a commercial path. API keys only. Do not edit the live Mac mini deploy or secrets. SPEC 132 (`admatrixmdp/spec132-memory-revive`, six commits, not merged) is a dependency of SPEC 134 only. Do not merge it into this branch unless that SPEC starts.

## Design rule

Surfaces that the prototype already shows follow that prototype: one job, Vietnamese copy, no mock counts on a page whose API does not exist. Shell tokens stay on the sidebar and top bar. Chat stays a 740px column, one rounded composer, quiet Agent / Model controls.

## Already on this branch (not a new SPEC)

Sidebar matches v2 §1, including aliases `#projects` → `#chat` and `#agent-team` / `#agent-link` → Agent Setting. Composer is text, +, Agent, Model, send/stop. Conversation search (Ctrl+K on Trò chuyện) and rename / archive / delete / restore are wired to the session API. Project menu items stay disabled until SPEC 136.

Folded out of the sidebar, still reachable:

| Old item | Where it lives now |
|---|---|
| Nhóm (`#/teams`) | Link on Agent Setting |
| Bản đồ nhiệt (`#/heatmap`) | Link on Tổng quan |
| Marketing (`#/marketing`) | Link on Liên hệ |
| Webhook (`#/webhooks`) | Rewrites to Hooks, under the lifecycle-hook empty state. Webhooks are not labeled Hooks |
| Log | Renamed Nhật ký, group GIÁM SÁT. Same logs API |
| Sao lưu | Was only inside Cấu hình. Now also `#/backup`, same backup API |

Decision, Decision Flow, and Cuộc họp use the prototype layout with an empty state. They do not invent Minh An meeting rows or decision counts.

## Queue (v2 §11)

| SPEC | v2 | Scope | GoClaw already has | Goso must build | Acceptance (from v2) | Depends on |
|---|---|---|---|---|---|---|
| 134 | 1 | Memory backend: append/patch one section at a time; keep every write as a version; Dreaming consolidates **per scope**; episodic prompt in the user’s language, typed entities (person / org / doc / topic), drop empty sessions; add scope `project`. | Episodic notes, durable notes, message FTS (`gateway/internal/store` memory kinds). In-process pipeline. No fact store, no per-section history, no project scope. SPEC 132 (unmerged) adds tenant fail-closed, cross-session recall, episodic prune, KG supersede, prior-session admission. | Section writer that does not overwrite the whole file; version rows; scope on facts and episodes; Vietnamese extraction prompt; empty-session skip. UI for the seven memory tabs stays a later slice of this SPEC, after the store is honest. | A lower-authority write cannot erase a higher one; the previous version remains and can be compared. An empty “OK” session is marked skip, not stored as a 31K episode. Entity labels are typed, not stop-words. A project-scoped fact is not returned outside that project. Shadow mode does not change the chat prompt until a later gate. Tests cover the store. Sample rows use Minh An (chiết khấu 8% → 10% toàn công ty, 9% dự án miền Tây). | SPEC 132 if it is still unmerged. Stack 134 on that branch; do not copy it into main first. |
| 135 | 2 | Connector + automatic source lookup + citations. OAuth per person. Three ingest modes: live lookup, sync, one-shot import. First sources: Google Drive, Lark, Notion. Chat looks up Vault + enabled connectors and shows citation chips. No composer toggles. | Generic connector records (name, transport, endpoint, enabled) and a connector page. No Drive/Lark/Notion OAuth, no per-user grant, no “tra lúc chat”, no citation on a reply. | Connector kinds, OAuth, scope picker, the three modes, conflict flag, and a retrieval step the chat pipeline can call. Citation chip: source name · document · section, deep link. Turning a connector off removes it from the next lookup. | “Đại lý X còn nợ bao nhiêu?” returns one “Đã tra N nguồn” line, a citation row, and does not attach an unrelated doc. “chỉ tìm trên Drive” uses one source. A pasted URL is summarized as “Đã đọc trang” without asking. A missing OAuth shows not connected, never a fake document count. | 134 for fact reconciliation when a looked-up number disagrees with memory. The connector record itself can start before 134; citations that update facts cannot. |
| 136 | 3 | Project inside Trò chuyện, plus the rest of the conversation menu and search. | Sessions have label, agent, model override, and (from this branch) `archived`. Search, rename, archive, restore, delete already call that API. There is no `project` table and no project-scoped memory. | Project list above Gần đây, project screen (instructions, files or connector folder, members, meetings, project knowledge, close), `project_id` on the session, memory precedence user > project > team > agent > tenant. Enable “Thêm vào project” / “Bỏ khỏi project”. No project picker in the composer. | A chat inside “Mở kênh đại lý miền Tây” uses that project’s 9% discount, not the company 10%. Closing the project archives its facts and queues a lesson on Phê duyệt. Search covers recent, every project, and archived. Delete still confirms and does not delete extracted memory. | 134 (project scope). 135 for “trỏ thư mục Drive/Lark”. Menu items that only need the session API are already done. |
| 137 | 4 | One Phê duyệt queue (shell, browser send, Decision R2/R3, knowledge promotion) and the remaining HỆ THỐNG gaps against the live gateway. | Approvals page and API for tool/shell-style approvals. Tenants, providers, API keys, packages, config, import/export, backup, and Nhật ký are real pages. Account menu links to Tổ chức and API Keys. Hooks have no registry. Decision is an empty page. | Four approval kinds in one list. Provider notes for personal-subscription OAuth (do not enable them for customers). Optional user/agent counts on Tổ chức if the API grows them. Daily backup schedule only when the gateway can store it. Do not invent the 23-provider catalog. | A shell approval and a “nâng tri thức” row are distinguishable and can be approved or refused. Refusing does not count as an answer edit. Nhật ký still tails the real log. A provider that is not configured stays off, with no sample spend. | 134–136 for the knowledge-promotion kind and Decision R2/R3. Shell approvals can be grouped earlier. |
| 138 | 5 | Cuộc họp. Meet + uploaded audio first. Meeting bot only after a Vietnamese transcript trial. | Nothing. The live Cuộc họp page is an empty layout: upload refuses because there is no speech-to-text service, and it does not create a sample meeting. Connectors have no `meet` kind. | `meetings/<date>-<name>.md`, idempotency key `source + source_meeting_id`, summary / decisions / tasks / entities, reconciliation labels, review-before-save, consent checkbox that is required, viewer set = attendees + project members. | Re-sync overwrites the same file, not a second one. A decision that disagrees with the project fact is labeled, not silently applied. Upload without the consent check is rejected. No fake “46 phút” row. Chat can answer “họp giao ban tuần 44 chốt gì?” only after a real meeting is stored. | 134, 135 (Meet/Zoom connectors), 136 if a meeting is attached to a project. |
| 139 | 6 | Entity profile as its own page (option B), OKF export/import, issued-document lifecycle. | Knowledge graph page and vault files exist. No `#entity/<slug>` page, no OKF package type, no draft → issued → superseded lifecycle. | **Do not** build `people/` `companies/` `concepts/` folders. Route `#entity/<slug>` (example `#entity/dai-ly-can-tho-1`) with **no sidebar item**. Entry points: Knowledge Graph, Cuộc họp › Người và tổ chức (“Xem hồ sơ”), a chat citation whose source is a profile, entity names in replies, a pasted share link. Page: title + type (Người / Tổ chức / Khái niệm) · Tổng hợp · Dòng thời gian · Liên quan. **Implementation note:** Tổng hợp and Dòng thời gian are views over Memory Facts, not a separate store. Start with Tổ chức. Gate auto-stubs (a name seen in ≥2 sources, or confirmed by a person) until episodic extraction stops emitting junk tokens such as “The” / “When”. New names that pass the gate get an empty profile. A contradiction is a dispute label. Each row’s visibility follows the source scope (v2 §4.4). Wikilink: keep `[[wikilink]]` in Vault; OKF export rewrites to `[name](path.md)`; OKF import leaves standard links as they are. | “Đại lý Cần Thơ 1 hiện thế nào?” returns a 3-line summary and 3 sourced timeline points. An issued policy for September is not used for an October order. | 134 (facts + extraction quality), 135, 138. |

## Not in this queue

User-Chrome extension and 8-character pairing, SME billing, and the Lark routing workbook. Plain notes and worldbuilding stay out (v2 §9.3). Claude CLI and Codex subscription OAuth stay off.

## Definition of done

Each SPEC names the page and API it changes, keeps vi and en, and states the empty and error cases. Gateway tests for store or HTTP changes. `npm test` and `npm run verify` in `control-plane/` for UI changes. Before a UI PR is called mergeable, `npm run qa:tabs` is green against a running gateway and control plane (`docs/qa/qa-tabs.md`). No secret in a response body. One spec item per PR after SPEC 132 and the current PR #3.

## Decisions closed on 2026-11-01 (v2 §8, §9.1, §12)

| Decision | What was closed |
|---|---|
| Entity vault is option B | Own page `#entity/<slug>`, no sidebar item. Knowledge Graph is one entry point among others. Views over Memory Facts, not a new store. Tổ chức first. Auto-stubs gated until extraction is fixed. |
| Wikilinks | Keep `[[wikilink]]` inside Goso/Vault. Convert to `[name](path.md)` only in an OKF export. Import does not convert back. |
| Merge order | Merge SPEC 132 (PR #1) and PR #3 first, then SPEC 134. One PR per spec item after that. |

## PR acceptance (v2 §13, 2026-11-01)

A PR can be merged only when all three hold. Missing one means no merge. The checklist is `.github/PULL_REQUEST_TEMPLATE.md`.

1. The PR description has side-by-side screenshots of the HTML prototype and real Goso for every tab that PR changes.
2. `npm run qa:tabs` is green: no “chưa dựng” screen, no console error, no failed request the page depends on, and the chat flows work (send, agent/model, @mention, search, rename / archive / restore).
3. No real MQ Capital or customer data in code, examples, or screenshots. Minh An (demo) only.

“Chưa có dữ liệu”, “Chưa có cuộc họp”, and “Chưa có hook” are honest empty states. They are not the “chưa dựng” marker. Decision, Decision Flow, and Cuộc họp on PR #3 are those empty states. If a reviewer treats any unfinished surface as “chưa dựng”, take those three items out of PR #3 and ship them with their own SPECs.

## Still pending (v2 §14)

1. **GoClaw license, top priority.** This tree has `mcp/LICENSE` only. A written answer from the GoClaw team is required before commercial sale. Dat has drafted an Appendix A question letter. No license change until that answer exists.
2. Vietnamese transcription trial and recording-consent legal check. Blocks the Cuộc họp phase (SPEC 138) only.
3. Whether GoClaw 3.14 already has project-scoped memory. This queue assumes it does not; SPEC 134 adds it. Confirm before building a second scope model.
4. Official Lark logo. Do not draw a stand-in and call it Lark.

## What to start next

Merge **SPEC 132 (PR #1)** and **PR #3** once each is green under the three rules above. Then **SPEC 134**, one PR, stacked on 132. Dirty memory makes Connector citations, Project precedence, meetings, and entity profiles lie. Do not start 135’s fact updates before 134.
