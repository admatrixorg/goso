/** Wireframe-shaped DEMO fixtures. Not live CRM. Source: ZAgent UI.dc.html. */

export const meetSources = [
  { name: "Google Meet", mark: "M", isIcon: false, markBg: "var(--surface-2)", markC: "var(--text-2)", dot: "var(--green)", state: "Đã kết nối", stateC: "var(--green)", note: "4 lịch tuần này · tự lấy transcript", needsConnect: false },
  { name: "Zoom", mark: "Z", isIcon: false, markBg: "var(--accent-soft)", markC: "var(--accent)", dot: "var(--green)", state: "Đã kết nối", stateC: "var(--green)", note: "Cloud Recording đang bật", needsConnect: false },
  { name: "Ghi âm tại chỗ", mark: "", isIcon: true, markBg: "var(--surface-2)", markC: "var(--text-3)", dot: "var(--text-4)", state: "Chưa bật", stateC: "var(--text-3)", note: "Tải file họp offline lên", needsConnect: true },
];

export const recentMeetings = [
  { mark: "M", markBg: "var(--surface-2)", markC: "var(--text-2)", title: "Demo ZAgent — CP Dược Vinh Phát", when: "Hôm nay · 09:30", mins: "42 phút", people: "5 người", status: "11 đề xuất chờ duyệt", sc: "var(--accent)", sbg: "var(--accent-soft)", pending: false },
  { mark: "Z", markBg: "var(--accent-soft)", markC: "var(--accent)", title: "Báo giá gói Enterprise — Nội thất Hòa An", when: "Hôm qua · 15:00", mins: "28 phút", people: "3 người", status: "Đang phân tích 68%", sc: "var(--text-3)", sbg: "", pending: true },
  { mark: "M", markBg: "var(--surface-2)", markC: "var(--text-2)", title: "Kick-off triển khai — Bảo hiểm An Tâm", when: "11/08 · 14:00", mins: "61 phút", people: "7 người", status: "Chờ transcript từ nền tảng", sc: "var(--text-4)", sbg: "", pending: false },
];

export const allMeetings = recentMeetings.concat([
  { mark: "M", markBg: "var(--surface-2)", markC: "var(--text-2)", title: "Review pipeline Q3 — nội bộ PKD", when: "10/08 · 10:00", mins: "35 phút", people: "4 người", status: "Đã duyệt 8 việc", sc: "var(--green)", sbg: "var(--green-bg)", pending: false },
  { mark: "Z", markBg: "var(--accent-soft)", markC: "var(--accent)", title: "Chốt hợp đồng — Vận tải Đại Nam", when: "08/08 · 16:30", mins: "22 phút", people: "2 người", status: "Đã duyệt 3 việc", sc: "var(--green)", sbg: "var(--green-bg)", pending: false },
]);

export const inbox = [
  { dot: "var(--accent)", label: "11 đề xuất từ cuộc họp Vinh Phát" },
  { dot: "var(--orange)", label: 'Broadcast "Ưu đãi tháng 8" chờ duyệt' },
  { dot: "var(--red)", label: "2 nick mất kết nối lúc 08:12" },
];

export const weekStats = [
  { label: "Cuộc họp đã phân tích", value: "7" },
  { label: "Việc sinh ra từ họp", value: "34" },
  { label: "Deal đổi trạng thái", value: "5" },
  { label: "Cam kết miệng bị bỏ quên", value: "3" },
];

export const agentChips = [
  "Phân tích cuộc họp Vinh Phát",
  "Deal nào im lặng quá 7 ngày",
  "Việc quá hạn của đội sale",
  "Nick nào sắp chạm ngưỡng gửi",
];

export const taskKpis = [
  { ic: "inbox" as const, label: "CHƯA REP", value: "1", sub: "Cần trả lời ngay", c: "var(--red)", vc: "var(--red)" },
  { ic: "cal" as const, label: "HẸN HÔM NAY", value: "0", sub: "Lịch hẹn của bạn", c: "var(--orange)", vc: "var(--text)" },
  { ic: "eye" as const, label: "ĐANG THEO DÕI", value: "0", sub: "0 KH vừa rep", c: "var(--accent)", vc: "var(--text)" },
  { ic: "user" as const, label: "KH CỦA TÔI", value: "461", sub: "1 mới hôm nay", c: "var(--accent)", vc: "var(--text)" },
  { ic: "clock" as const, label: "KH ĐÌNH TRỆ", value: "0", sub: ">7 ngày không nhắn", c: "var(--text-4)", vc: "var(--text)" },
  { ic: "check" as const, label: "CHỐT THÁNG", value: "0", sub: "Khách đã chốt", c: "var(--green)", vc: "var(--green)" },
];

export const taskTimeline = [
  { time: "09:12", title: "Duyệt 11 đề xuất họp Vinh Phát", kind: "meet" },
  { time: "08:40", title: "Nháp tin cho Nguyên Crypto — chờ anh gửi", kind: "chat" },
  { time: "Hôm qua", title: "Broadcast Ưu đãi tháng 8 — nháp", kind: "mkt" },
];

export const friends = [
  { name: "Dat Nguyen Ai", meta: "ID 3671238388375687309 · 84916683619 · Nam", nicks: "1 nick", tag: "active", score: 25, scoreN: "25", msgs: "20 / 1", ini: "A", av: "#c07a4b" },
  { name: "Phan Duy", meta: "ID 1062406867165470913 · Nam", nicks: "0", tag: "—", score: 0, scoreN: "0", msgs: "0 / 0", ini: "D", av: "#2c3e50" },
  { name: "Nhật Nguyệt Minh", meta: "ID 6533090510570070229 · 84979855168 · Nữ", nicks: "0", tag: "—", score: 0, scoreN: "0", msgs: "0 / 0", ini: "M", av: "#8e6c88" },
  { name: "Sông Lam Phan", meta: "ID 2431736299086499520 · 84913380122 · Nam", nicks: "0", tag: "—", score: 0, scoreN: "0", msgs: "0 / 0", ini: "P", av: "#4a90b8" },
];

export const mkMenu = [
  "Tệp khách hàng",
  "Quét nhóm",
  "Mục tiêu",
  "Chăm sóc",
  "Sequence",
  "Broadcast",
  "Khối nội dung",
  "Mẫu tin",
];
