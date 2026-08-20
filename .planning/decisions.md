# Decisions — GOSO

| # | Ngày | Quyết định | Lý do | Lựa chọn bị loại |
|---|------|------------|-------|------------------|
| 01 | 2026-08-19 | **Clean-room thương mại** | GoClaw gốc CC BY-NC 4.0 không cho dùng thương mại; phải viết lại 100% | Fork trực tiếp GoClaw |
| 02 | 2026-08-19 | **Lộ trình C→B** | Ra thị trường nhanh (C: Control Plane mới + gateway black-box), vừa thay dần gateway (B) | A (viết hết rồi mới ship) / B thuần (mất 6 tháng mới có MVP) |
| 03 | 2026-08-19 | **Giữ Go cho gateway** | Hiệu năng, single binary, hệ sinh thái GoClaw sẵn | Chuyển hết sang TS/Python (rủi ro hiệu năng) |
| 04 | 2026-08-19 | **Dual DB: Postgres+pgvector (server) / SQLite FTS5 (desktop)** | Tương thích GoClaw, tối ưu từng môi trường | Chỉ dùng một DB cho cả hai |
| 05 | 2026-08-19 | **Giữ bản Desktop Wails v2 + React** | Nhu cầu người dùng cá nhân, phân phối dễ | Chỉ làm server |
| 06 | 2026-08-19 | **MVP 4 channel: Telegram + WebSocket + Zalo Personal + Zalo OA** | Phủ 90% nhu cầu VN, Zalo là khác biệt cạnh tranh | Chỉ Telegram/WebSocket |
| 07 | 2026-08-19 | **Thương hiệu GOSO** | Ngắn, dễ nhớ, không xung đột GoClaw | Giữ tên GoClaw |
| 08 | 2026-08-19 | **Control Plane viết mới bằng TypeScript (Node 20, ESM)** | Tận dụng MCP hiện có, tốc độ phát triển nhanh | Viết Control Plane bằng Go |
| 09 | 2026-08-19 | **MCP giữ nguyên kiến trúc dual transport (stdio + Streamable HTTP)** | Tương thích Claude/Cursor + production | Chỉ stdio |
| 10 | 2026-08-19 | **SPEC 001 = Harness + Gateway skeleton** | Khóa nền tảng trước khi thêm nghiệp vụ | Nhảy thẳng vào channel |

*Ghi mọi quyết định mới vào đây, không để trong đầu ai.*

| 11 | 2026-08-20 | **SPEC 002: net/http + gorilla/websocket + store in-memory** | Không framework, interface store để thay DB sau | Gin/Echo, DB ngay từ đầu |

| 12 | 2026-08-20 | **SPEC 003: net/http LLM + Telegram webhook, không SDK** | Nhẹ, không kéo SDK nặng, dễ mock test | SDK anthropic-go / go-openai |

| 13 | 2026-08-20 | **SPEC 004: 2 Zalo adapter stub webhook, Sender injectable** | Text-only, test mock, không copy ZaloCRM | Polling/QR thật, rich message |

| 14 | 2026-08-20 | **SPEC 005: modernc.org/sqlite (pure Go) cho persist** | Không CGO, cross-build dễ; file data/goso.db | mattn/go-sqlite3 (cần CGO) |
