# Glossary — GOSO (GoClaw-derived, clean-room)

> Nguồn chân lý cho ngôn ngữ miền. Mọi spec, biến, bảng DB phải dùng đúng thuật ngữ này.

| Thuật ngữ | Định nghĩa | Ghi chú |
|-----------|------------|---------|
| **GOSO** | Thương hiệu nền tảng gateway AI mới, clean-room từ ý tưởng GoClaw | Không copy code GoClaw (CC BY-NC 4.0) |
| **Agent** | Thực thể AI tự trị, có `agent_key`, `display_name`, `model`, `workspace` | 1 agent = 1 không gian làm việc |
| **Session** | Phiên hội thoại liên tục giữa user và agent, có `session_key` | Lưu lịch sử message |
| **Message** | Một lượt trao đổi (user/assistant/tool) trong Session | Có `trace_id` |
| **Channel** | Kênh đầu-cuối (Telegram, WebSocket, Zalo Personal, Zalo OA, Discord...) | Mỗi channel có adapter riêng |
| **Provider** | Nhà cung cấp LLM (Anthropic, OpenAI, OpenRouter...) | Cấu hình api_key, base_url, models |
| **Model** | Định danh model LLM (`claude-sonnet-4`, `gpt-4o`...) | Thuộc Provider |
| **Skill** | Bộ kỹ năng/nhắc việc mở rộng cho agent | Có thể gán cho agent hoặc user |
| **Cron** | Công việc định kỳ gửi message tới agent theo cron expression | |
| **Team** | Nhóm agent có thể ủy quyền (delegation) cho nhau | |
| **MCP Server** | Máy chủ Model Context Protocol gắn vào gateway | stdio / sse / streamable-http |
| **Gateway** | Tiến trình Go chạy trung tâm, điều phối channel, session, LLM | Giai đoạn C: dùng GoClaw binary; B: GOSO Gateway viết mới |
| **Control Plane** | Lớp quản trị GOSO (API + Dashboard + Billing) viết mới 100% | Giai đoạn C dựng trước |
| **Pipeline Stage** | Một chặng xử lý trong luồng message (ingress → routing → LLM → egress) | |
| **VaultDoc** | Tài liệu bộ nhớ dài hạn của agent (SOUL.md, IDENTITY.md...) | Lưu trong agent workspace |
| **KG Entity** | Thực thể trong Knowledge Graph trích xuất từ hội thoại | |
| **BatchQueue** | Hàng đợi xử lý theo lô (embed, index...) | |
| **Trace** | Dấu vết thực thi LLM (prompt, token, cost, latency) | Phục vụ billing & debug |
| **Workspace** | Thư mục làm việc cục bộ của agent trên đĩa | |
| **Delegation Link** | Liên kết ủy quyền `agent → agent` | |
| **Transport** | Phương thức truyền MCP (stdio / sse / streamable-http) | |
| **Clean-room** | Quy trình viết lại không sao chép code gốc, chỉ học ý tưởng/hành vi | Bắt buộc pháp lý |
| **C→B** | Lộ trình: (C) Control Plane mới + Gateway GoClaw black-box → (B) thay dần Gateway bằng GOSO Gateway Go | |
| **Wails Desktop** | Bản desktop dùng Wails v2 + React + SQLite FTS5 | Giữ lại ở GOSO |
| **Overlay** | Lớp Docker Compose phủ lên gateway (postgres, redis, jaeger...) | 8 overlay gốc |
| **Zalo Personal** | Kênh Zalo cá nhân (profile) | Ưu tiên MVP |
| **Zalo OA** | Kênh Zalo Official Account | Ưu tiên MVP |

| **Observability** | Khả năng quan sát hệ thống: log, trace, metrics | SPEC 008 |

| **Rate Limit** | Giới hạn số request/phút/IP, trả 429 + Retry-After | SPEC 006 |

| **Auth (Admin Token)** | Bảo vệ /api/* và /ws bằng Bearer token, bypass /healthz | SPEC 006 |

| **Deploy** | Đóng gói Docker + Compose, overlay prod | SPEC 012 |

| **Hardening** | Cứng hóa: secret scan, semgrep, E2E, runbook | SPEC 013 |

*30+ thuật ngữ, mở rộng khi SPEC mới bổ sung.*
