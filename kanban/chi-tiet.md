<!-- Sinh bởi kanban/render.py từ kanban/data.json. Đừng sửa tay. -->
# Kanban Board: goso

- **Repo:** [admatrixorg/goso](https://github.com/admatrixorg/goso) · **Cập nhật:** `2026-11-27 14:06 UTC+7` · **`main`:** [`50a470a`](https://github.com/admatrixorg/goso/commit/50a470a3)
- **Tổng:** `51` PR gộp trong 7 ngày, `4` PR đang mở
- **Điểm nghẽn số 1:** 4 PR đứng ở review từ đầu tháng 11, chạm WIP 4; #38 xung đột với main.
- **Nguồn dữ liệu:** [`data.json`](data.json) · **Bảng web:** [`web/index.html`](web/index.html)

## 1. 📋 To Do (`0`)
- _Không có thẻ._

## 2. 🚧 Doing (`1/4`)
- **[CHUYEN-GOSO](https://github.com/admatrixmdp/admatrix-process)** Chuyển dữ liệu GoClaw (Mac mini) sang Goso trên VPS 2 (bảng riêng ở admatrix-process) — *Owner: Lead* — ➡️ Xem tiến độ từng bước trên bảng admatrix-process; không chép thẻ sang đây
  - Issue #26 (tổng), #30 (kiểm VPS 2), #32 (nghiệm thu 9router, chờ anh Dat)

## 3. 🔎 In Review (`4/4`)
- **[#38](https://github.com/admatrixorg/goso/pull/38)** docs: Memory v3 specs 153–169 (MQG-229) (SPEC MQG-229, `cursor/memory-v3-specs-153-169-ff9f`, `CONFLICTING`, xung đột với main, đứng từ 12/11) — *Owner: admatrixmdp* — ➡️ Rebase lên main rồi gộp (chỉ tài liệu) ⚠️
- **[#24](https://github.com/admatrixorg/goso/pull/24)** feat(bridge): score knowledge chunks with Jev before injection (`cursor/jev-knowledge-filter-f553`, đứng từ 12/11) — *Owner: admatrixmdp* — ➡️ Chạy test bridge rồi gộp hoặc đóng ⚠️
- **[#23](https://github.com/admatrixorg/goso/pull/23)** fix(replycheck): split SHOP FACTS from CUSTOMER WORDS (FM bugs: order-code false hold, customer voucher miss) (`fix/replycheck-customer-words`, đứng từ 02/11) — *Owner: admatrixmdp* — ➡️ Chạy test replycheck rồi gộp hoặc đóng ⚠️
- **[#10](https://github.com/admatrixorg/goso/pull/10)** Trò chuyện: Project section above recent conversations (`cursor/chat-project-sidebar-a600`, đứng từ 12/11) — *Owner: admatrixmdp* — ➡️ Xem lại giao diện rồi gộp hoặc đóng ⚠️

## 4. ⛔ Blocked / Chờ duyệt (`0`)
- _Không có thẻ._

## 5. ✅ Done (`14`)
**Gộp trong 7 ngày (`14`):**
- **[#140](https://github.com/admatrixorg/goso/pull/140)** docs: bảng việc kanban và AGENTS.md gọn trỏ agent đọc bảng (`docs/bang-viec-kanban`, gộp 27/11) — *Owner: Lead*
- **[#139](https://github.com/admatrixorg/goso/pull/139)** feat(auth): cho khoá API cấp gateway chỉ đọc xem người dùng của một tenant (`feat/system-key-read-users`, gộp 2026-11-26) — *Owner: admatrixmdp*
- **[#137](https://github.com/admatrixorg/goso/pull/137)** fix(channels): list only the request tenant channels (`fix/channel-list-tenant-scope`, gộp 2026-11-26) — *Owner: admatrixmdp*
- **[#138](https://github.com/admatrixorg/goso/pull/138)** ci: serialize test packages when a shared Postgres is configured (`ci/serialize-pg-tests`, gộp 2026-11-26) — *Owner: admatrixmdp*
- **[#136](https://github.com/admatrixorg/goso/pull/136)** fix(channel): polite Vietnamese voice on customer chats (`fix/zalo-content-voice`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#135](https://github.com/admatrixorg/goso/pull/135)** fix(zalo): keep the personal websocket alive and reply to the sender (`fix/zalo-personal-keepalive`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#134](https://github.com/admatrixorg/goso/pull/134)** fix(zalo): run the personal listener and answer inbound DMs (`fix/zalo-personal-reply`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#133](https://github.com/admatrixorg/goso/pull/133)** ci: make CircleCI the only pipeline (`ci/disable-github-actions`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#132](https://github.com/admatrixorg/goso/pull/132)** ci: run CircleCI on every push so the connected project actually builds (`ci/circleci-trigger`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#130](https://github.com/admatrixorg/goso/pull/130)** ci: add a CircleCI pipeline that mirrors the GitHub workflow (`ci/circleci`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#131](https://github.com/admatrixorg/goso/pull/131)** fix(channels): stop 404s on the channel detail panels that are not ported (`fix/channel-detail-empty-panels`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#129](https://github.com/admatrixorg/goso/pull/129)** fix(zalo): sign requests so a stored cookie set is accepted (`fix/zalo-signed-params`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[#128](https://github.com/admatrixorg/goso/pull/128)** fix(channels): show a freshly created channel on the first page (`fix/channel-list-order`, gộp 2026-11-24) — *Owner: admatrixmdp*
- **[GOP-KHAC](https://github.com/admatrixorg/goso/pulls?q=is%3Apr+is%3Amerged+merged%3A%3E%3D2026-11-20)** Thêm 38 PR gộp khác trong 7 ngày — *Owner: nhiều người*

## Bảng kiểm tra nhanh

| ID | Cột | Owner | Stream | Trạng thái | Mergeable | Head | Điểm nghẽn | Bước tiếp theo |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| [CHUYEN-GOSO](https://github.com/admatrixmdp/admatrix-process) | Doing | Lead | Vận hành / CI | bảng riêng ở admatrix-process |  |  |  | Xem tiến độ từng bước trên bảng admatrix-process; không chép thẻ sang đây |
| [#38](https://github.com/admatrixorg/goso/pull/38) | In Review | admatrixmdp | Tài liệu / spec | xung đột với main, đứng từ 12/11 | CONFLICTING | `cursor/memory-v3-specs-153-169-ff9f` | Có | Rebase lên main rồi gộp (chỉ tài liệu) |
| [#24](https://github.com/admatrixorg/goso/pull/24) | In Review | admatrixmdp | Tính năng | đứng từ 12/11 |  | `cursor/jev-knowledge-filter-f553` | Có | Chạy test bridge rồi gộp hoặc đóng |
| [#23](https://github.com/admatrixorg/goso/pull/23) | In Review | admatrixmdp | Sửa lỗi | đứng từ 02/11 |  | `fix/replycheck-customer-words` | Có | Chạy test replycheck rồi gộp hoặc đóng |
| [#10](https://github.com/admatrixorg/goso/pull/10) | In Review | admatrixmdp | Tính năng | đứng từ 12/11 |  | `cursor/chat-project-sidebar-a600` | Có | Xem lại giao diện rồi gộp hoặc đóng |
| [#140](https://github.com/admatrixorg/goso/pull/140) | Done | Lead | Tài liệu / spec | gộp 27/11 |  | `docs/bang-viec-kanban` |  |  |
| [#139](https://github.com/admatrixorg/goso/pull/139) | Done | admatrixmdp | Tính năng | gộp 2026-11-26 |  | `feat/system-key-read-users` |  |  |
| [#137](https://github.com/admatrixorg/goso/pull/137) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-26 |  | `fix/channel-list-tenant-scope` |  |  |
| [#138](https://github.com/admatrixorg/goso/pull/138) | Done | admatrixmdp | Vận hành / CI | gộp 2026-11-26 |  | `ci/serialize-pg-tests` |  |  |
| [#136](https://github.com/admatrixorg/goso/pull/136) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/zalo-content-voice` |  |  |
| [#135](https://github.com/admatrixorg/goso/pull/135) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/zalo-personal-keepalive` |  |  |
| [#134](https://github.com/admatrixorg/goso/pull/134) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/zalo-personal-reply` |  |  |
| [#133](https://github.com/admatrixorg/goso/pull/133) | Done | admatrixmdp | Vận hành / CI | gộp 2026-11-24 |  | `ci/disable-github-actions` |  |  |
| [#132](https://github.com/admatrixorg/goso/pull/132) | Done | admatrixmdp | Vận hành / CI | gộp 2026-11-24 |  | `ci/circleci-trigger` |  |  |
| [#130](https://github.com/admatrixorg/goso/pull/130) | Done | admatrixmdp | Vận hành / CI | gộp 2026-11-24 |  | `ci/circleci` |  |  |
| [#131](https://github.com/admatrixorg/goso/pull/131) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/channel-detail-empty-panels` |  |  |
| [#129](https://github.com/admatrixorg/goso/pull/129) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/zalo-signed-params` |  |  |
| [#128](https://github.com/admatrixorg/goso/pull/128) | Done | admatrixmdp | Sửa lỗi | gộp 2026-11-24 |  | `fix/channel-list-order` |  |  |
| [GOP-KHAC](https://github.com/admatrixorg/goso/pulls?q=is%3Apr+is%3Amerged+merged%3A%3E%3D2026-11-20) | Done | nhiều người |  |  |  |  |  |  |
