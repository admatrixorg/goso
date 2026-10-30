# QA — SPEC 132: Memory revive (recall xuyên phiên, admission, KG supersede, TTL, tenant 400)

Ngày: 2026-10-30. Nhánh `admatrixmdp/spec132-memory-revive`. Chạy trên Mac mini, go1.26.2, `go test ./gateway/...` 37 package OK, `go vet` sạch.

Nguồn: audit prod GoClaw 30/10 (`~/goclaw-audit-20261030/REPORT.md`): memory L1 của prod chưa từng tự nạp vào prompt lần nào, KG không hết hạn, prune không chạy, tenant sai lặng lẽ rơi về default. Spec này sửa các lỗ đó trên goso.

## AC và bằng chứng

| # | AC | Kết quả | Bằng chứng |
|---|----|---------|-----------|
| 1 | Ghi nhớ L1 (episodic/durable) của **cùng tenant + cùng agent** ở phiên khác được nạp vào đầu prompt khi câu hỏi liên quan | PASS | `pipeline.TestRunner_RecallAcrossSessions`; smoke: phiên 2 log `candidates=1 injected=1 runes=261`, reply đúng |
| 2 | Không nạp từ phiên hiện tại, agent khác, tenant khác, hay dòng `message` thô | PASS | `store.TestRecallMemories_CrossSessionTenantAgent` (memory + sqlite) |
| 3 | Câu chào/ack không nạp gì (admission: ≥2 token có nghĩa, VI+EN stopword) | PASS | `TestIsTrivialQuery`; smoke turn "ok cảm ơn" → `skipped=trivial` |
| 4 | Ngưỡng tương đối ≥0.5×best, tối đa 5 mục, ngân sách 1500 rune (`GOSO_MEMORY_RECALL_BUDGET`), mỗi mục ≤400 rune | PASS | `TestBuildRecallNote_Admission` |
| 5 | Ghi chú đứng trước lịch sử (không bị chôn giữa context) | PASS | `TestRunner_RecallAcrossSessions` kiểm index |
| 6 | Mỗi lượt đều có log `goso memory: recall … candidates= injected= runes=`; 0 lần nạp không còn lặng lẽ | PASS | smoke log dòng 12, 16 |
| 7 | Tắt bằng `GOSO_MEMORY_RECALL=0` | PASS | `TestRunner_RecallAcrossSessions` đoạn kill switch |
| 8 | Quan hệ KG `exclusive` tự đóng quan hệ cũ cùng (tenant, from, rel); supersede entity/relation đặt `valid_until`, không sửa tại chỗ | PASS | `store.TestKGExclusiveRelationAndSupersede` (memory + sqlite) |
| 9 | `POST /api/kg/entities/{id}/supersede`, `/relations/{id}/supersede`, cờ `exclusive` khi tạo quan hệ; sai tenant → 404; `valid_until` sai định dạng → 400 | PASS | `httpapi.TestKGAPI_SupersedeRoutes`; smoke POST supersede trả `valid_until` |
| 10 | Tool `memory_expand` không trả fact đã supersede cho agent | PASS | `pipeline.TestMemoryExpand_SupersededIsNotCurrent` |
| 11 | `PruneEpisodic` xoá episodic cũ, giữ durable, FTS đồng bộ (trigger `memories_ad` mới) | PASS | `store.TestPruneEpisodic_KeepsDurableAndSyncsFTS` |
| 12 | Vòng prune nền: `GOSO_MEMORY_TTL_DAYS` mặc định 90, 0 = tắt, chạy mỗi 6h, lần đầu sau 1 phút | PASS (một phần) | `TestPruneTTL`; smoke lần đầu chạy ngay lúc boot gặp `SQLITE_BUSY` → đã lùi 1 phút; chưa quan sát vòng 6h |
| 13 | `GOSO_MULTI_TENANT=1` + header tenant không hợp lệ → 400, không rơi về default; tắt multi-tenant thì bỏ qua header | PASS | `tenant.TestInvalid`, `httpapi.TestRejectInvalidTenant`; smoke: bad header 400, `acme` 200 |
| 14 | Prompt tóm tắt giữ tên, quyết định, sở thích, việc mở, đúng ngôn ngữ hội thoại | Chưa đo | Chỉ đổi prompt; provider thật chưa chạy trong phiên này |

## Chưa làm / ghi chú

- Postgres: `RecallMemories` đi đường quét 500 dòng gần nhất + chấm điểm trùng token (không bm25). Chưa chạy test PG trong phiên này.
- `AgentKey` hiện unique toàn hệ thống, không theo tenant (phát hiện khi viết test) — ngoài phạm vi spec.
- Recall chỉ lexical (FTS5/bm25 hoặc token overlap). Không có embedding; đúng với kết luận audit: lexical + admission tốt hơn vector 0.7/0.3 không bao giờ qua ngưỡng.

## Smoke đã chạy (port 18132, SQLite `/tmp/goso-smoke.db`, provider echo)

1. Tạo agent, 2 phiên. Phiên 1 chat `summarize=true` → episodic note có "Vietcombank".
2. Phiên 2 hỏi "An muốn thanh toán Vietcombank kiểu gì?" → log `candidates=1 injected=1 runes=261`.
3. Phiên 2 "ok cảm ơn" → `skipped=trivial`.
4. Supersede entity qua API → 200 kèm `valid_until`.
5. Bật multi-tenant: header `bad tenant!` → 400; `acme` → 200.
