import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

const hours = ["07:00", "08:00", "09:00", "10:00", "11:00", "12:00", "13:00", "14:00"];
const weekDays = [
  { dow: "T2", num: "24" },
  { dow: "T3", num: "25" },
  { dow: "T4", num: "26" },
  { dow: "T5", num: "27" },
  { dow: "T6", num: "28" },
  { dow: "T7", num: "29" },
  { dow: "CN", num: "30" },
];

export function CalendarPage() {
  return (
    <div style={{ padding: "14px 22px", display: "flex", flexDirection: "column", gap: 10 }}>
      <SectionHeader
        icon="cal"
        title="Lịch hẹn"
        description="0 lịch · tuần DEMO — chưa có API lịch. Click slot trống không tạo sự kiện thật."
        actions={
          <>
            <DemoBadge />
            <Button variant="primary" icon="plus">
              Tạo nhắc hẹn
            </Button>
          </>
        }
      />
      <div style={{ display: "flex", background: "var(--surface-2)", borderRadius: 10, padding: 3, width: "fit-content", fontSize: 12.5 }}>
        <span style={{ background: "var(--btn-dark-bg)", color: "var(--btn-dark-fg)", borderRadius: 7, padding: "5px 16px", fontWeight: 600 }}>Tuần</span>
        <span style={{ padding: "5px 16px", color: "var(--text-2)" }}>Danh sách</span>
      </div>
      <Card style={{ overflow: "hidden" }}>
        <div style={{ display: "flex", borderBottom: "1px solid var(--border)" }}>
          <div style={{ width: 52, flex: "none" }} />
          {weekDays.map((w, i) => (
            <div key={w.dow} style={{ flex: 1, textAlign: "center", padding: "8px 0", borderLeft: "1px solid var(--border-soft)", background: i === 6 ? "var(--today)" : undefined }}>
              <div style={{ fontSize: 10.5, fontWeight: 600, color: "var(--text-3)" }}>{w.dow}</div>
              <div style={{ fontSize: 17, fontWeight: 700 }}>{w.num}</div>
              <div style={{ fontSize: 9.5, color: "var(--text-4)" }}>0 lịch</div>
            </div>
          ))}
        </div>
        {hours.map((h) => (
          <div key={h} style={{ display: "flex", borderBottom: "1px solid var(--border-soft)", height: 34 }}>
            <div style={{ width: 52, flex: "none", fontSize: 10, color: "var(--text-4)", fontVariantNumeric: "tabular-nums", textAlign: "right", padding: "2px 6px 0 0" }}>
              {h}
            </div>
            {weekDays.map((w) => (
              <div key={w.dow} style={{ flex: 1, borderLeft: "1px solid var(--border-soft)" }} />
            ))}
          </div>
        ))}
        <EmptyState>Không có lịch hẹn.</EmptyState>
      </Card>
    </div>
  );
}
