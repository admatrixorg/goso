# SPEC 014 — AI-Native Core Agent: Connector Architecture (ZaloCRM làm connector đầu tiên)

> LOCKED: 2026-08-20 — goso = core AI-native, ZaloCRM = connector riêng (MCP/HTTP, EventStore tracking→suggestion, sidecar zca-js không lây AGPL)
> CHỜ LOCK cũ đã đóng theo user 'approved' 2026-08-20 (không BUILD — chỉ xong bước plan).
> Nhánh: `plan/014-zalocrm-connector` trên `admatrixorg/goso` (tách từ `main` @ e11755c).

## Goal

GOSO trở thành **core agent AI-native** điều khiển các hệ thống nghiệp vụ bên ngoài (CRM, POS, ERP…) thông qua lớp **Connector** chuẩn hóa. ZaloCRM là connector đầu tiên, chạy **riêng** (không merge code vào goso) — goso chỉ học **hành vi/hợp đồng** để viết client adapter sạch. Sau SPEC này, việc thêm một CRM/POS/ERP mới = viết thêm 1 connector theo hợp đồng, không sửa core.

## Bối cảnh (vì sao chọn kiến trúc này)

- User muốn goso là **bộ não** (agent), ZaloCRM là **thân** (kết nối Zalo + dữ liệu CRM); tương tự với POS/ERP sau này.
- Upstream `locphamnguyen/ZaloCRM` là **AGPL-3.0** → copy code vào goso sẽ lây AGPL toàn bộ goso (xung đột thương mại).
- Bản địa phương `zalocrm-mcp/` đã chứng minh mô hình **sidecar MCP làm ranh giới kiểm soát** (approval, policy proof, không lộ UID).

## Nguyên tắc AI-native (bắt buộc với mọi feature từ SPEC này)

> **Tracking → Data → Decision suggestion.** Mọi việc thiết kế từ đây phục vụ **AI Agent + human hoạt động**: agent thực thi, người giám sát và quyết định.

1. **Không feature nào không sinh event**: mỗi lần gọi tool, mỗi kết quả, mỗi quyết định duyệt/từ chối/sửa của người đều ghi thành **Event** có `trace_id`, timestamp, connector, kind. Ghi ngay tại lớp Invoke — không vá sau.
2. **Không event nào không vào data**: Event lưu vào EventStore truy vấn được (`GET /api/events`) — đây là nguyên liệu cho AI phân tích và cho human giám sát trên Control Plane.
3. **Không data nào không dẫn tới gợi ý**: dữ liệu tích lũy phải được phân tích định kỳ/kích hoạt để sinh **Suggestion** cho người (chi tiết ở SPEC 015 — Decision Loop). SPEC 014 chịu trách nhiệm phần tracking + data; SPEC 015 dùng data đó.
4. **Human-in-the-loop là dữ liệu học**: quyết định approve/reject/sửa nội dung của người được ghi kind `human_feedback`, làm ngữ cảnh cho gợi ý và hành vi sau này.

## Kiến trúc đích

```
Channel (Zalo/Telegram/WS) → Session → Agent Runtime
                                         │
                                         ├─ LLM Provider (routing, fallback)
                                         ├─ Tool Layer (function calling)
                                         │    └─ Connector Registry
                                         │         ├─ ZaloCRM connector (MCP/HTTP)
                                         │         ├─ CRM-x connector (tương lai)
                                         │         ├─ POS-y connector (tương lai)
                                         │         └─ ERP-z connector (tương lai)
                                         ├─ EventStore (tracking → data) ──► SPEC 015:
                                         │                                    phân tích → Suggestion → human
                                         └─ Memory (VaultDoc, KG)
```

## User stories

- **US-01** Agent trong goso có thể gọi tool chuẩn (ví dụ `contact_search`, `message_preview`, `appointment_list`) của ZaloCRM thông qua connector; tool chạy qua MCP (stdio/streamable-http) hoặc HTTP REST; kết quả quay lại Session làm tool message.
- **US-02** Operator đăng ký connector qua Control Plane hoặc API (`POST /api/connectors` với name, transport, endpoint, credential ref); agent được gán connector qua `agent_connector` link; `GET /api/connectors` liệt kê trạng thái (connected/disabled).
- **US-03** Mọi tool **đột biến** (gửi tin nhắn, tạo đơn hàng, đổi giá…) phải qua **Approval Gate**: tool trả về `pending_approval` + policy proof, không tự thực thi; chỉ route duyệt của owner mới cho phép thực thi (theo mẫu `zalocrm-mcp`).
- **US-04** Connector chuẩn khai báo **Tool Manifest** (JSON Schema cho mỗi tool) — goso tự nạp manifest thành tool cho LLM (không hardcode từng tool).
- **US-05** Connector hỏng/offline → tool call trả lỗi chuẩn `connector_unavailable`, agent báo lại user thay vì treo; timeout + retry cấu hình được.
- **US-06 (tracking → data)** Mọi hành vi của vòng đời tool (attempt, success, error, `connector_unavailable`, phê duyệt/từ chối/sửa nội dung của người) đều sinh **Event** vào EventStore và xem được trên Control Plane (bảng Event). Dữ liệu này là đầu vào cho phân tích và gợi ý ở giai đoạn sau — không có feature nào không sinh event.

## Acceptance criteria

- [ ] AC-01 `gateway/internal/connector` — package mới: `Connector` interface (ListTools, Invoke, Health), `Registry` (đăng ký/lookup theo name), test đơn vị không cần mạng.
- [ ] AC-02 MCP client transport (streamable-http + stdio) gọi được MCP server thật trong test (fake MCP server in-process); tool manifest → MCP tool mapping.
- [ ] AC-03 HTTP connector transport (REST endpoint + Bearer) với manifest JSON nạp từ URL hoặc inline config.
- [ ] AC-04 Approval Gate: tool manifest đánh dấu `requires_approval: true` → Tool Layer không invoke trực tiếp, trả `pending_approval` + `approval_id`; endpoint `POST /api/approvals/:id/decision` để duyệt/từ chối.
- [ ] AC-05 API connector CRUD: `POST/GET /api/connectors`, gán vào agent `POST /api/agents/:id/connectors`, Control Plane hiển thị danh sách + trạng thái health.
- [ ] AC-06 Agent runtime: LLM nhận tool list từ connector của agent; tool result lưu Message role `tool` trong Session; trace ghi connector + latency.
- [ ] AC-07 `make verify` xanh; e2e: fake ZaloCRM-style connector → user chat "tìm khách A" → agent gọi tool → trả kết quả; tool nhạy cảm → pending_approval.
- [ ] AC-08 Không copy code từ `Zalo CRM/`, `zalocrm-mcp/`, hay upstream AGPL; chỉ tham khảo hành vi/hợp đồng; mọi file mới có header GOSO.
- [ ] AC-09 EventStore: mọi attempt/success/error/approval-decision sinh Event (`trace_id`, `connector`, `tool`, `kind`, `ts`, tóm tắt args/result — không lộ credential); lưu ring/persist, `GET /api/events?kind=&connector=&limit=`; Control Plane có trang Events xem được theo thời gian thực cơ bản.

## Non-goals

- Không merge/fork code ZaloCRM vào goso (giữ AGPL riêng biệt).
- Không viết lại kết nối Zalo thật trong goso (SPEC 004 giữ stub; vận tải Zalo thuộc ZaloCRM).
- Không xây CRM/POS/ERP cụ thể nào ngoài ZaloCRM-connector — hợp đồng phải đủ tổng quát cho tương lai.
- Không OAuth provider thứ ba (Facebook/Google) — để SPEC riêng.
- Không billing per-tool — SPEC 010 đã có khung.
- Không UI chat trực tiếp Zalo trong goso — Control Plane quản trị connector, chat vẫn qua channel.
- Không sinh Suggestion ngay trong 014 — phần phân tích → gợi ý ở SPEC 015 (Decision Loop). SPEC 014 chỉ cam kết: data được tracking đầy đủ để 015 dùng.

## Rủi ro & ghi chú

| Rủi ro | Giảm thiểu |
|--------|------------|
| Hợp đồng tool không đủ tổng quát → sửa core khi thêm ERP | AC-04 manifest-driven; test với ≥2 manifest mẫu (CRM + giả lập POS) |
| Approval Gate trở thành nút thắt UX | Timeout duyệt + fallback message rõ; preview nhanh như `zalocrm-mcp` |
| Connector AGPL gọi goso = "tác phẩm phái sinh"? | Tương tác qua mạng/API chuẩn, không liên kết tĩnh — ranh giới dịch vụ (ADR 18); tham vấn pháp lý nếu cần |
| ZaloCRM API thay đổi | Versioned manifest; connector khai báo `schema_version` |

## Câu hỏi mở [NEEDS CLARIFICATION]

1. Transport chính cho ZaloCRM connector: **MCP streamable-http** (tái dụng `zalocrm-mcp` đã có) hay **REST thuần**? Khuyến nghị: MCP trước (có sẵn sidecar), REST làm fallback.
2. Approval duyệt ở đâu: Control Plane goso hay giữ nguyên route owner của ZaloCRM? Khuyến nghị: **giữ route owner ZaloCRM** (AC-04 chỉ lưu pending + relay), tránh trùng quyền.
3. Nhánh này có cần upstream tracking tới `locphamnguyen/ZaloCRM` không? Khuyến nghị: **không** — chỉ tham khảo, không merge.
