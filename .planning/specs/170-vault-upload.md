# SPEC 170 — Upload tài liệu vào vault

Trạng thái: nháp đã sửa theo CTO, chưa giao code. Một PR vào `main`. Không merge từ file này.

Mẫu dữ liệu: Minh An. Không đọc dữ liệu khách, không chép GoClaw/Dewee. Không đổi memory, không đổi hành vi importer 152 ngoài việc **bỏ qua `raw/`**. Không đổi `PUT /api/vault/docs` khi cờ tắt.

Nền (9545d08, đã đối chiếu lại):

- `security/limit.go:11–24`: `LimitAPI` bọc mọi `/api` và `/v1` bằng `MaxBytesReader(MaxAPIBody)` = 1 MiB. Upload vault lớn hơn 1 MiB sẽ chết nếu không miễn trừ đúng route.
- Thứ tự middleware (`serve.go:199–215`): Observe → Auth (`Require`, khi có token hoặc không dev-mode) → rate limit → tenant → `LimitAPI` → mux. Auth *đã* chạy trước `LimitAPI` khi được bật. Khi `GOSO_DEV_MODE=1` và không có admin/view token, `Require` bị bỏ — route upload **vẫn phải tự buộc auth**.
- Route vault chỉ có `/api/vault/*`, không alias `/v1` (`handlers.go:238–246`).
- `PutTenant(title, body, tenantID)` không nhận path (`vault.go:201`). Handler bỏ qua `path` sau khi kiểm escape (`handlers_vault.go:81–96`).
- `collectTenant` nhặt `.md`/`.txt` trong mọi thư mục con (`scope.go:338–375`). Sync xoá hàng registry khi file không còn trong tập đó (`SyncTenant` `scope.go:503–520`). Inventory cũng dùng `collectTenant`.

## Mục tiêu

Có một cửa upload **vào vault**: kiểm auth và tenant trước khi đọc body, stream file ra đĩa tạm, kiểm magic bytes, lưu bất biến trong `raw/`, SHA-256 của **bytes gốc**, chống trùng trong tenant, hạn mức đĩa từng tenant, tạo việc với trạng thái. Cây thư mục của cả bộ được tạo từ PR này. Thêm `PutTenantAt` để 171–174 chỉ ghi/dời qua một cửa.

## Phạm vi

### Route và LimitAPI

- Thêm đúng một route: `POST /api/vault/upload` (multipart, field `file`). Không dùng `POST /api/storage/upload`. Không đăng ký alias `/v1/vault/upload` (đồng bộ với các route vault hiện có). Nếu một PR sau thêm alias, phải miễn trừ cả hai path trong `LimitAPI` — không làm trong PR này.
- `LimitAPI` miễn trừ **chính xác** `POST` + path `/api/vault/upload` **chỉ khi** cờ `GOSO_VAULT_UPLOAD` bật. Khi cờ tắt, route này **vẫn chịu trần 1 MiB** (`MaxAPIBody`) và handler trả 404; không ghi vào `raw/`.
- Khi cờ bật: không bọc `MaxAPIBody` trên đúng route đó. Sau khi auth và tenant đã đạt, bọc `http.MaxBytesReader` theo `GOSO_VAULT_INGEST_MAX_BYTES` (mặc định 20 MiB). Mọi route `/api` và `/v1` khác vẫn trần 1 MiB, kể cả khi cờ bật.
- Auth và tenant **trước** khi đọc body: không `ParseMultipartForm`, không copy ra file tạm, không hash, nếu chưa qua admin/write và chưa resolve tenant. Request chưa đăng nhập, kể cả body 20 MiB, trả 401/403 và **không đọc body** (không slurp vào RAM, không ghi temp). Trong dev-mode không token, route này vẫn từ chối nếu thiếu admin/write — không được “mở cửa” vì `Require` bị bỏ.
- Trước 158/164: chỉ **admin token** và khóa có scope `write` (hoặc `admin`) được upload. Viewer, view token, khóa `read` trả 403; thân lỗi không chứa tên file, hash, path. `X-Goso-Tenant` giữ quy tắc 149: khóa API không đổi tenant bằng header.

### Ghi đĩa và chống giả loại file

- Stream từ `MaxBytesReader` thẳng ra file tạm trong thư mục đã kiểm của tenant (`O_EXCL`, không theo symlink). **Không** đọc toàn bộ file vào RAM. Hash SHA-256 vừa ghi vừa tính.
- Không tin phần mở rộng hay `Content-Type` của client. Kiểm magic bytes (ít nhất: PDF `%PDF`, ZIP/Office `PK` rồi kiểm nội dung zip cho docx/xlsx/pptx, PNG/JPEG/GIF/WebP, và UTF-8 text cho txt/md). Không khớp → 400 `type not allowed`, xoá temp, không vào `raw/`.
- Loại chấp nhận sau khi kiểm magic: pdf, docx, xlsx, pptx, txt, md, png, jpg, jpeg, gif, webp. `.doc` / `.odt` / audio / video: 400 rõ (chưa làm).
- Vượt `GOSO_VAULT_INGEST_MAX_BYTES`: 413, xoá temp, không ghi `raw/`.

### Job, raw, cây thư mục

- Bảng việc ingest (SQLite và Postgres, **chỉ thêm bảng**): id, tenant_id, sha256_original, original_filename, mime_detected (từ magic, không phải client), bytes, raw_path, status, error_class, created_at, updated_at, existing_job_id (khi trùng).
- Trạng thái dành sẵn: `uploaded`, `converting`, `converted`, `naming`, `classifying`, `inbox`, `filed`, `wiki_updating`, `ready`, `failed`, `duplicate`. PR này chỉ ghi `uploaded`, `duplicate`, `failed`.
- `GET /api/vault/ingest/{id}` chỉ cùng tenant; khác tenant 404, không lộ.
- Tạo cây `raw/`, `sources/` (`inbox/`, `projects/`, `areas/`, `resources/`, `archive/`), `wiki/` (`topics/`). Có thể tạo `wiki/schema.md`, `index.md`, `log.md` bản trống do **code** viết, chưa có logic 174.
- `raw_path` logic: `raw/<sha256>.<ext>` trong thư mục tenant. Tên người dùng chỉ lưu trên job (và sau 173 trong frontmatter).
- Trùng: cùng tenant + cùng SHA-256 gốc → không ghi `raw/` lần hai, 200 `{duplicate: true, ...}`. Khác tenant không so, không lộ.
- Raw bất biến. Không xoá raw trong spec này.

### PutTenantAt

Thêm `PutTenantAt(tenant, path, body)` trên `vault.Service`:

- `path` là logical path trong thư mục tenant, đã kiểm escape, không symlink, không được trỏ vào `raw/`.
- Ghi file và hàng `vault_docs` **trong một bước**, cùng trình tự an toàn như `PutTenant` hiện tại (temp `O_EXCL` trong dir đã kiểm, commit registry, rename, refresh links; lỗi DB thì không publish file; hash thân khớp registry).
- `PutTenant` (title → slug) giữ nguyên cho form JSON.
- **171–174 chỉ được tạo, ghi đè, hoặc dời file vault (sources/wiki) qua `PutTenantAt`** (dời = ghi path mới + gỡ path cũ trong cùng bước logic, không để orphan). Không `os.WriteFile` thẳng, không sửa registry trước/sau lệch nhau.
- File trong `raw/` không đi qua `PutTenantAt` và **không** tạo hàng `vault_docs` (nhị phân, job mới là nguồn sự thật cho raw).

### Loại raw/ khỏi sync, inventory, importer

- `collectTenant` bỏ qua mọi path có tiền tố `raw/` (trước filter ext), kể cả `.md`/`.txt` nếu lỡ nằm dưới `raw/`. Ghi `skipped` sao cho `SyncTenant` **không** lấy đó làm căn cứ xoá hàng khác, và **không** xoá hay upsert từ `raw/`.
- Inventory không liệt kê file dưới `raw/` (cũng không lộ hash/tên gốc qua inventory).
- Importer 152: bỏ qua path nguồn hoặc đích dưới `raw/`; không ghi vào `raw/`; không lấy file `raw/` làm body. Hành vi khác của 152 giữ nguyên.
- Test đủ ba chỗ (sync, inventory, importer).

### Hạn mức đĩa từng tenant

Đây là điều kiện bật, không phải việc để sau.

- Có hạn mức tổng đĩa **từng tenant** (raw + sources + wiki + mọi file dưới `t/<tenant>/`): cấu hình `GOSO_VAULT_INGEST_QUOTA_BYTES` (số dương, byte). Anh Dat đặt số này khi duyệt bật prod.
- **Thiếu hạn mức (unset, 0, hoặc âm) thì không được bật trên prod.** Nếu cờ `GOSO_VAULT_UPLOAD` bật mà hạn mức không hợp lệ: upload fail-closed (không ghi), health/cấu hình báo `quota_required`. Không im lặng coi như vô hạn.
- Vượt hạn mức (file mới làm tổng vượt trần): 413 hoặc 409 `quota exceeded`, xoá temp, không vào `raw/`, không tạo job thành công.
- Dev/test phải gán một hạn mức tường minh trong fixture (ví dụ vài MiB), không dựa vào “không đặt là được”.

## Ngoài phạm vi

- Chuyển Markdown, OCR (171). Đặt tên (173). Phân loại (172). Wiki (174). UI (175).
- Upload trong chat (SPEC 133).
- Đổi `storage.MaxFileBytes` / `security.MaxAPIBody` cho các route khác.
- Coi đường dẫn là quyền. Forget (nợ 164).

## Dữ liệu / API

```
POST /api/vault/upload
Content-Type: multipart/form-data
field: file

201 Created
{ "job": { "id", "tenant_id", "status": "uploaded", "sha256", "bytes", "original_filename", "raw_path" } }

200 OK khi trùng trong cùng tenant
{ "duplicate": true, "job": { "status": "duplicate", ... }, "existing_job_id": "..." }

GET /api/vault/ingest/{id} → 200 cùng tenant, hoặc 404
```

Audit: action `upload`, entity `vault_ingest`, id job, size/ext/status — không có thân file, không secret.

## Cờ

`GOSO_VAULT_UPLOAD` mặc định tắt. Tắt: `POST /api/vault/upload` và `GET /api/vault/ingest/{id}` trả 404; `LimitAPI` **không** miễn trừ route này, vẫn cắt body ở 1 MiB. Bật: `LimitAPI` mới miễn trừ `POST /api/vault/upload` và áp trần `GOSO_VAULT_INGEST_MAX_BYTES` (mặc định 20 MiB). Migration thêm bảng, tắt cờ không rollback schema.

Bật trên prod: anh Dat duyệt **và** đã đặt hạn mức đĩa từng tenant.

## Kiểm thử

Mẫu Minh An (fixture tự tạo, không dữ liệu khách). Memory + SQLite + Postgres.

- Upload pdf, docx, xlsx, pptx, png, txt, md (đúng magic): 201, file trong `raw/`, hash khớp, job `uploaded`. Stream không giữ toàn bộ file trong một `[]byte` trên đường ghi raw (test có thể dùng hook/kích thước sát trần).
- Cùng bytes lần hai: 200 `duplicate`, không thêm file.
- Cùng hash khác tenant: hai lần 201, không thấy nhau.
- Extension giả (`.pdf` nhưng không phải PDF), MIME client giả: 400, không ghi.
- `.exe`, `.doc`: 400.
- Vượt trần từng file: 413, không ghi.
- Vượt hạn mức tenant: từ chối, không ghi.
- Cờ bật, hạn mức thiếu: fail-closed, không ghi.
- **Cờ tắt + body 2 MiB** gửi `POST /api/vault/upload`: `LimitAPI` vẫn cắt ở 1 MiB; không tạo file trong `raw/`. (Handler trả 404 nếu không đọc body; nếu body bị đọc thì lỗi trần 1 MiB. Cả hai đều không được ghi `raw/`.)
- **Cờ bật:** file 20 MiB (trong hạn mức đĩa, đúng magic, đã auth) upload thành công (201, có file trong `raw/`). Các route `/api` khác vẫn trần 1 MiB (ví dụ `PUT /api/vault/docs` với body > 1 MiB).
- **Request 20 MiB chưa đăng nhập** khi cờ bật (không Bearer / token sai): 401, không đọc body, không tạo temp, không có file trong `raw/`. Lặp với dev-mode không token.
- Viewer, view token, khóa `read`: 403, không đọc body. Admin và khóa `write`: 201 trên fixture nhỏ.
- Cờ tắt, body nhỏ: 404, không vào `raw/`.
- `PUT /api/vault/docs` và `POST /api/storage/upload` như cũ.
- `PutTenantAt` ghi path + hàng registry cùng lúc; ghi vào `raw/` bị từ chối; path escape/symlink bị từ chối.
- Sync **không xoá** hàng `vault_docs` của file trong `sources/` hoặc `wiki/` (kể cả khi có thêm hay thiếu file dưới `raw/`).
- Sync **không tạo** hàng `vault_docs` từ `raw/x.md`. Inventory không liệt kê `raw/`. Importer bỏ qua `raw/`.
- `go test ./...` và `npm run qa:tabs` xanh. PR này chưa đổi UI; không bắt ảnh prototype.

## Nghiệm thu

- [ ] `LimitAPI` chỉ miễn trừ `POST /api/vault/upload` khi cờ bật; cờ tắt vẫn trần 1 MiB và 404, body 2 MiB không vào `raw/`. Cờ bật: 20 MiB qua được; route `/api` khác vẫn 1 MiB.
- [ ] Sync không xoá hàng `sources/` hay `wiki/`; không tạo hàng từ `raw/x.md`.
- [ ] Auth + tenant trước khi đọc body; 20 MiB chưa auth không bị đọc.
- [ ] Stream ra temp; magic bytes; hash gốc; chống trùng theo tenant.
- [ ] `PutTenantAt` là cửa ghi cho pipeline.
- [ ] `raw/` loại khỏi sync, inventory, importer.
- [ ] Hạn mức đĩa từng tenant fail-closed khi thiếu hoặc vượt.
- [ ] Cờ tắt = hành vi cũ.
- [ ] QA ghi nợ forget 164: raw, job, sources, wiki dẫn xuất sẽ do 164 xoá; PR này chưa xoá theo người.

## Phụ thuộc

Không phụ thuộc Memory v3 để bắt đầu code. Dựa trên vault 149. Liên kết Linear: [MQG-235](https://linear.app/mqglobal/issue/MQG-235/spec-158-danh-tinh-xac-thuc-cho-user-key) (SPEC 158), [MQG-242](https://linear.app/mqglobal/issue/MQG-242/spec-164-scope-acl-forget) (SPEC 164).

## Ngân sách

Không tốn LLM. Trần từng file mặc định 20 MiB. Hạn mức đĩa: anh Dat đặt khi bật prod. Công sức chưa ước lượng bằng giờ.

## Rollback

Tắt `GOSO_VAULT_UPLOAD`. Không `DROP TABLE`. File đã upload giữ nguyên.
