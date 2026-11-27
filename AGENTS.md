# AGENTS.md — goso

Bản gọn, đọc trước mọi phiên. Cấu trúc thư mục và lệnh chạy nhanh ở `README.md`.

## Bản đồ 30 giây

- Repo `admatrixorg/goso`: Gateway AI viết bằng Go (clean-room từ ý tưởng GoClaw), kèm control-plane, desktop, MCP.
- **Bảng việc:** đầu phiên đọc `kanban/README.md` — việc đang làm, chờ review, kẹt hoặc chờ PO, vừa gộp. `python3 kanban/render.py --tom-tat` in tóm tắt vài dòng và báo PR mở chưa có thẻ.
- Việc chuyển dữ liệu GoClaw (Mac mini) sang Goso trên VPS 2 theo dõi ở bảng `admatrixmdp/admatrix-process`; bảng ở đây chỉ giữ một thẻ trỏ về đó, không chép lại.
- Kiểm trước khi mở PR: `make verify`. CI: CircleCI (`.circleci/config.yml`).

## Bảng việc

- `kanban/data.json` là nguồn duy nhất; phần còn lại của `kanban/` là sinh ra.
- Mở / đóng PR hay đổi trạng thái việc → sửa `data.json`, chạy `python3 kanban/render.py`, commit trong cùng PR.
- Commit cuối của PR trước khi gộp chuyển thẻ của chính PR sang Done, để main không trễ một nhịp.
- Kho ý tưởng `.planning/backlog.md`: chỉ grep; thẻ To Do lấy từ đó khi bắt tay làm.

## Luật

- Không push thẳng main: mỗi việc một nhánh, một PR, CI xanh mới gộp.
- Không commit khoá, `.env`, token.
