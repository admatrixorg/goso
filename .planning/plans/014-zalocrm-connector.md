# PLAN 014 — AI-Native Core Agent: Connector Architecture

> SPEC: `specs/014-zalocrm-connector.md` (LOCKED 2026-08-20; BUILD 2026-08-22 on `admatrixmdp/spec014`)
> Nhánh: `admatrixmdp/spec014` (base `main` @ e11755c)

## Mô hình miền (DDD — vẽ trước schema/code)

| Thực thể | Thuộc tính chính | Bất biến |
|----------|------------------|----------|
| **Connector** | name, transport (`mcp-http`/`mcp-stdio`/`http`), endpoint, credential_ref, schema_version, enabled | name duy nhất; disabled ⇒ mọi Invoke trả `connector_unavailable` |
| **ToolManifest** | tool name, JSON Schema input, `requires_approval`, mô tả | Schema phải parse được trước khi connector được coi là healthy |
| **ApprovalRequest** | approval_id, connector, tool, args, policy_proof, status (`pending/approved/rejected/expired`), expires_at | Tool có `requires_approval` không bao giờ được Invoke khi chưa approved |
| **AgentConnectorLink** | agent_id, connector_name | 1 cặp agent+connector duy nhất |

Luồng: `Session → Tool Layer → Registry.lookup(connector) → [requires_approval? → ApprovalRequest(pending)] : Transport.Invoke → Message(role=tool)`.

## Bảng task

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Interface `Connector` + `Registry` (không I/O) | `gateway/internal/connector/connector.go`, `registry.go`, `*_test.go` | `go test ./internal/connector -run Registry` |
| T02 | Tool Manifest parse + validate JSON Schema | `gateway/internal/connector/manifest.go`, `manifest_test.go` | `go test ./internal/connector -run Manifest` |
| T03 | MCP transport (streamable-http + stdio) với fake MCP server in-process | `gateway/internal/connector/mcp.go`, `mcp_test.go` | `go test ./internal/connector -run MCP` |
| T04 | HTTP transport (REST + Bearer) | `gateway/internal/connector/http.go`, `http_test.go` | `go test ./internal/connector -run HTTPTransport` |
| T05 | Approval Gate + `POST /api/approvals/:id/decision` | `gateway/internal/approval/gate.go`, httpapi handler, tests | `go test ./internal/approval` |
| T06 | Connector CRUD API + agent link | `gateway/internal/httpapi/handlers.go`, store, tests | `go test ./internal/httpapi -run Connector` |
| T07 | Wire Tool Layer vào Agent runtime (tool list từ connector, Message role=tool, trace) | `gateway/internal/agent/*`, tests | `go test ./internal/agent -run Tools` |
| T08 | Health + timeout/retry + lỗi chuẩn `connector_unavailable` | `gateway/internal/connector/health.go`, tests | `go test ./internal/connector -run Health` |
| T09 | Control Plane: trang Connectors (danh sách + health + gán agent) | `control-plane/src/pages/Connectors.tsx`, `src/api/client.ts` | `npm run typecheck` + build |
| T10 | E2E: fake ZaloCRM-style connector (manifest CRM + giả lập POS) | `scripts/e2e-connector.sh`, fixture manifest | `./scripts/e2e-connector.sh` |
| T11 | EventStore: mọi invoke/approval sinh Event, `GET /api/events`, trang Events trên Control Plane (AC-09) | `gateway/internal/eventstore/*`, httpapi, `control-plane/src/pages/Events.tsx` | `go test ./internal/eventstore` + typecheck |
| T12 | QA AC-01…AC-09 + verify tổng | `make verify` + checklist | bảng QA trong file này |

## Rationale

- **MCP trước, REST fallback**: `zalocrm-mcp` đã là MCP sidecar hoàn chỉnh với approval/policy-proof — tái dụng ngay hợp đồng thay vì viết REST mới. (Trả lời câu hỏi mở #1 của SPEC, khuyến nghị áp dụng; user có thể bác khi LOCK.)
- **Approval Gate nằm ở goso nhưng thực thi ở owner ZaloCRM**: goso chỉ giữ `pending` + relay quyết định (AC-04), không tranh quyền duyệt với route owner (câu hỏi mở #2).
- **Manifest-driven, không hardcode tool**: yêu cầu AI-native — thêm POS/ERP chỉ cần nộp manifest mới; test T10 dùng 2 manifest để chứng minh tính tổng quát.
- **Fake connector in-process cho test**: không phụ thuộc dịch vụ ZaloCRM thật khi CI chạy; tuân thủ DDD — business-rule test không cần server ngoài.
- **Không theo dõi upstream AGPL** (câu hỏi mở #3): tránh mọi rủi ro "kéo code AGPL vào nhánh" kể cả vô tình qua merge.
- **EventStore — xương sống AI-native**: T11 đảm bảo **không feature nào không sinh event** (nguyên tắc SPEC 014). Mọi Invoke/approval-decision đều ghi Event (kind: `attempt/success/error/human_feedback/...`, trace_id, tóm tắt — không lộ credential) làm nguyên liệu cho SPEC 015 sinh Suggestion; tách riêng T11 để AC-09 có thể song song T06–T10 nhưng verify sau cùng ở T12.

## Song song hóa

- T01–T02 là nền — làm trước. T03/T04 song song (2 người/2 worktree). T05 song song T03/T04 (khác package). T06/T07/T08 sau T01–T04. T09 chỉ cần T06. T10 sau T01–T04. T11 (EventStore) bắt đầu ngay sau T01 (không phụ thuộc transport), gom Event từ mọi layer — hoàn thiện cùng lúc với T05–T10; T12 cuối.

## Trạng thái

- [x] T01 — interface + registry
- [x] T02 — manifest
- [x] T03 — MCP transport
- [x] T04 — HTTP transport
- [x] T05 — approval gate
- [x] T06 — CRUD API
- [x] T07 — tool layer runtime
- [x] T08 — health/retry
- [x] T09 — control plane UI
- [x] T10 — e2e fake connector
- [x] T11 — event store
- [x] T12 — QA

## QA (điền khi BUILD xong)

| AC | Kết quả | Bằng chứng |
|----|---------|------------|
| AC-01 | ✅ | `go test ./gateway/internal/connector -run Registry` — `Connector` + `Registry`; disabled → `connector_unavailable` |
| AC-02 | ✅ | `go test ./gateway/internal/connector -run MCP` — fake MCP HTTP + stdio in-process |
| AC-03 | ✅ | `go test ./gateway/internal/connector -run HTTPTransport` — REST + Bearer, manifest URL/inline |
| AC-04 | ✅ | `go test ./gateway/internal/approval` + HTTP `POST /api/approvals/{id}/decision`; no Invoke until approved; relay only |
| AC-05 | ✅ | `POST/GET /api/connectors`, `POST /api/agents/{id}/connectors`; Control Plane `Connectors.tsx`; SPEC 001–007 routes intact |
| AC-06 | ✅ | `go test ./gateway/internal/agent -run Tools` — tool list, `role=tool`, trace connector+latency |
| AC-07 | ✅ | `make verify`; `./scripts/e2e-connector.sh` (CRM `contact_search` + POS; `message_send` → pending_approval) |
| AC-08 | ✅ | `grep -r minhhaiphan gateway` empty; `zalocrm` string-only; GOSO copyright header on new Go files |
| AC-09 | ✅ | `go test ./gateway/internal/eventstore`; `GET /api/events`; `control-plane/src/pages/Events.tsx` |
