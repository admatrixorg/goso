import { useDemoT } from "../demo/i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { SectionHeader } from "../ui/SectionHeader";
import { allMeetings } from "../demo/mock";

export function MeetingsPage() {
  const { d, t } = useDemoT();
  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="mic"
        title={d("meetings.title")}
        description={d("meetings.desc")}
        actions={
          <>
            <DemoBadge />
            <Button icon="refresh" iconGesture>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus">
              {d("meetings.upload")}
            </Button>
          </>
        }
      />
      <div style={{ background: "var(--card)", border: "1px solid var(--border)", borderRadius: 12, padding: "8px 10px", display: "flex", gap: 4, fontSize: 12.5, width: "fit-content" }}>
        <span style={{ padding: "5px 14px", borderRadius: 8, background: "var(--accent-soft)", color: "var(--accent)", fontWeight: 600 }}>{d("meetings.week")}</span>
        <span style={{ padding: "5px 14px", color: "var(--text-2)" }}>{d("meetings.month")}</span>
        <span style={{ padding: "5px 14px", color: "var(--text-2)" }}>{d("meetings.all")}</span>
      </div>
      <Card>
        <CardHeader icon="list" title={d("meetings.list")} meta={`${allMeetings.length} · DEMO`} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 2.6 }}>CUỘC HỌP</span>
          <span style={{ flex: 1.2 }}>THỜI GIAN</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>ĐỘ DÀI</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>THAM DỰ</span>
          <span style={{ flex: 1.5, textAlign: "right" }}>TRẠNG THÁI</span>
        </div>
        {allMeetings.map((m) => (
          <div key={m.title} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 2.6, display: "flex", alignItems: "center", gap: 10, minWidth: 0 }}>
              <span
                style={{
                  width: 26,
                  height: 26,
                  borderRadius: 7,
                  background: m.markBg,
                  color: m.markC,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  fontSize: 11,
                  fontWeight: 700,
                  flex: "none",
                }}
              >
                {m.mark}
              </span>
              <b style={{ fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{m.title}</b>
            </span>
            <span style={{ flex: 1.2, color: "var(--text-2)" }}>{m.when}</span>
            <span style={{ flex: 0.8, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{m.mins}</span>
            <span style={{ flex: 0.8, textAlign: "right", fontVariantNumeric: "tabular-nums", color: "var(--text-2)" }}>{m.people}</span>
            <span style={{ flex: 1.5, textAlign: "right" }}>
              {m.sbg ? <Badge tone={m.sc.includes("green") ? "positive" : "accent"}>{m.status}</Badge> : <span style={{ color: m.sc, fontSize: 11.5 }}>{m.status}</span>}
            </span>
          </div>
        ))}
        </TableScroll>
      </Card>
    </div>
  );
}
