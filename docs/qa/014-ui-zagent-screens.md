# QA — ZAgent remaining screens (control-plane)

Date: 2026-08-24  
Branch: `admatrixmdp/ui-zagent-screens`

Không thêm endpoint backend. Mọi số/hàng mới là **DEMO**.

| Màn | Status | Data |
|-----|--------|------|
| Home + nguồn họp + họp gần đây | done | mock `demo/mock.ts` |
| Cuộc họp / Bản ghi đã nhận | done | mock |
| Việc của tôi (KPI, phiên theo dõi, cần rep, timeline) | done | mock |
| Bạn bè | tương đương (4 hàng) | mock |
| Lịch hẹn | placeholder tuần trống | mock 0 lịch |
| Kho ảnh | placeholder empty | none |
| Marketing / Tệp KH | placeholder empty | none |
| Cài đặt: nguồn, tài khoản, giao diện | done (cơ bản) | theme thật; nguồn mock |

Desktop: vẫn chỉ Agents/Sessions/Chat. `wails.json` không đổi.

Giả định: không dựng 13 trang settings CRM / 7 tab marketing / heatmap — ngoài API GOSO.
