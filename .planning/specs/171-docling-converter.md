# SPEC 171 — Bộ chuyển Docling (OCR tiếng Việt), sidecar và hàng đợi

Trạng thái: đã code ở PR #42; bằng chứng image chuyển sang cổng trước khi bật cờ (docs/qa/171-docling-converter/README.md). Một PR vào `main`. Không merge từ file này.

Mẫu: Minh An (kể cả ảnh/PDF scan tự tạo). Không dùng bản án hay tài liệu khách. Không chép GoClaw/Dewee.

## Mục tiêu

Chuyển file trong `raw/` thành markdown UTF-8 trong `sources/`, chạy ngoài process gateway, có hàng đợi, OCR tiếng Việt trên Linux/Docker. Gateway chỉ đổi trạng thái job và nhận kết quả qua `PutTenantAt`.

## Phạm vi

- Worker/sidecar riêng (compose service). Gọi Docling hoặc docling-serve qua HTTP **nội bộ**. Không nhúng Python vào binary Go. Ghi `sources/` chỉ bằng `PutTenantAt` (path do **code** đặt, ví dụ `sources/inbox/<jobid>.md` hoặc `sources/inbox/<jobid>-partNN.md` cho tới 173). Không ghi `raw/`, không ghi `wiki/`.
- OCR: **chỉ Tesseract CLI `vie`+`eng`**. Image sidecar **không** cài EasyOCR (kéo PyTorch nặng). EasyOCR là mục **đánh giá sau**, chỉ mở lại khi đã có bench Minh An — không phải tuỳ chọn cấu hình trong PR này. Không dùng `ocrmac`. Không dùng RapidOCR.
- Sidecar chạy **non-root**, có giới hạn CPU và RAM (ghi rõ số trong compose của PR; số cụ thể cho VPS2 **chưa đo**, chọn trần an toàn cho dev và để anh Dat/FM chỉnh khi cài prod). Mạng: **không egress** (không gateway ra ngoài, không pull model, không DNS ra Internet). Chỉ lắng nghe trên mạng nội bộ compose, chỉ gateway gọi được.
- Model Docling và tessdata `vie`+`eng` **nhúng sẵn trong image**. PR phải **chứng minh không tải model lúc chạy**: build không còn `wget`/`curl` model ở entrypoint; khi chạy, chặn mạng, một job chuyển thành công trên fixture; log/test không có download. Việc “image hiện tại của upstream có kéo model lúc chạy hay không” **chưa kiểm** — đây là việc PR phải chứng minh, không giả định đã xong.
- Timeout **từng trang** (env, có mặc định trong PR, chưa đo trên VPS2). Hết giờ trang → trang đó lỗi; quá số lần thử (đề xuất 2, chưa đo) job `failed` class `timeout`, không treo `converting`.
- Khi 170 và 171 cùng bật, job `uploaded` (pdf/office/ảnh, hoặc txt/md cần chuẩn hoá) chuyển `converting`. txt/md hợp lệ: không OCR; chuẩn hoá UTF-8, NFC, LF, rồi `PutTenantAt` vào `sources/inbox/`.
- Markdown > 1 MiB (`security.MaxAPIBody`): tách theo heading, mỗi phần ≤ 1 MiB, không cắt giữa ký tự UTF-8, mỗi phần một lần `PutTenantAt`.
- Kết quả toàn khoảng trắng / không có chữ: `failed` class `empty_conversion`, **giữ raw**. Xét nội dung, không dùng ngưỡng byte cứng làm chết file ngắn hợp lệ.
- Timeout/thử lại/`error_class`: `timeout`, `ocr_failed`, `unsupported`, `empty_conversion`, `too_large`, `not_configured`, `internal`.
- Hook no-op 173/172/174 nếu cờ tắt: job dừng ở `converted` (file đã ở `sources/inbox/`).
- Giấy phép: mã Docling MIT (đã mở LICENSE). PR kèm bảng giấy phép model/gói thực sự đóng trong image (tessdata, layout). Thiếu bảng thì chưa bật prod.

## Ngoài phạm vi

- Phân loại, đặt tên cuối, wiki, UI (175 làm UI sau PR này).
- Sửa logic 152 ngoài `raw/` đã loại ở 170.
- GPU. EasyOCR. Tải model lúc chạy. Egress “chỉ khi thiếu model”.
- Xoá raw sau khi chuyển (cấm).

## Dữ liệu / API

Worker không mở cổng ra ngoài. Job thêm trường additive: `source_paths`, `ocr_engine` (luôn tesseract trong PR này), `ocr_lang` (`vie+eng`), `page_count`, `converted_at`. Không lưu nguyên văn trong bảng job.

`GET /api/vault/ingest/{id}` trả thêm các trường trên.

Docling chỉ đọc `raw/` (mount đọc) và trả markdown cho gateway; gateway `PutTenantAt` vào `sources/`.

## Cờ

`GOSO_VAULT_DOCLING` mặc định tắt. Tắt: job ở lại `uploaded`; không gọi sidecar. Thiếu sidecar khi cờ bật: `failed` class `not_configured` (fail-closed).

Bật prod: anh Dat duyệt, đã có hạn mức đĩa (170), và đã có bằng chứng image không tải model lúc chạy.

## Kiểm thử

- Fixture Minh An: PDF chữ, docx, xlsx, pptx, một ảnh/PDF scan tiếng Việt có dấu. Một cụm từ có dấu chỉ có trong trang phải xuất hiện trong markdown. **Chất lượng Tesseract `vie` trên tập thật chưa kiểm trước PR** — nghiệm thu PR là fixture; kết quả trên VPS2 gắn nhãn chưa kiểm nếu chưa đo.
- txt/md không gọi OCR, vẫn vào `sources/inbox/` qua `PutTenantAt`.
- Rỗng → `failed`, raw còn.
- Markdown lớn tách heading, mỗi phần ≤ 1 MiB, ghép lại khớp.
- Cờ tắt: không gọi sidecar.
- Sidecar down: `not_configured`/`internal`, gateway không 500 lõi.
- Khác tenant không đọc `source_paths` của nhau.
- Image/ci: chứng minh không download lúc chạy (chặn mạng + job fixture thành công, hoặc tương đương ghi trong `docs/qa/171-docling-converter/`).
- Sidecar: process non-root; không egress (test/compose policy).
- Timeout từng trang: fixture giả trang treo → `timeout`, không treo job.
- CI mặc định dùng converter giả (fake) cho `go test`. Job OCR thật là bằng chứng tay hoặc nightly, nói rõ trong QA. `go test ./...` và `qa:tabs` xanh.

## Nghiệm thu

- [ ] Chỉ Tesseract `vie`+`eng`; EasyOCR không có trong image. — phần bằng chứng image là cổng trước khi bật cờ, không phải cổng PR
- [ ] Non-root, có trần CPU/RAM, không egress, model nhúng sẵn, có bằng chứng không tải lúc chạy. — phần bằng chứng image là cổng trước khi bật cờ, không phải cổng PR
- [ ] Timeout từng trang. Raw không bị sửa/xoá. Ghi qua `PutTenantAt`.
- [ ] QA ghi nợ forget 164 (raw, job, sources, wiki dẫn xuất).
- [ ] qa:tabs xanh.

## Phụ thuộc

Bắt buộc 170. Hook 173/172/174. Linear: cùng project Ingest & Wiki, sau 170.

## Ngân sách

CPU/RAM sidecar **chưa đo trên VPS2**. Token LLM: 0. VLM là việc sau, không làm ở PR này.

## Chưa kiểm chứng

- Việc image upstream hiện tải model lúc chạy hay không (PR phải chứng minh bản của Goso thì không).
- Chất lượng Tesseract `vie`.
- Dư địa CPU/RAM/đĩa VPS2.

## Rollout

- Compose `volume.subpath` needs Docker Engine 26 or newer. Freelance manager checks the version on VPS2 before deploy.
- The sidecar can read every tenant's vault read-only and is restricted only by the code jail (`jail.py`). This is an accepted risk.
- Per job the gateway may wait up to `page_timeout` × `attempts` × `page_budget` (64 min by default), and the worker is sequential, so one large file blocks the queue. This is to be measured on VPS2.

## Rollback

Tắt `GOSO_VAULT_DOCLING`. File đã chuyển giữ nguyên. Không xoá raw.
