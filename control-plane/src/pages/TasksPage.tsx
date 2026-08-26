import { useDemoT } from "../demo/i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { Icon, type IconName } from "../ui/Icon";
import { SectionHeader } from "../ui/SectionHeader";
import { taskKpis, taskTimeline } from "../demo/mock";

export function TasksPage({ onChat }: { onChat: () => void }) {
  const { d } = useDemoT();
  return (
    <div style={{ padding: "26px 28px 60px", display: "flex", flexDirection: "column", gap: 16 }}>
      <SectionHeader
        icon="check"
        title={d("tasks.title")}
        description={d("tasks.desc")}
        actions={
          <>
            <DemoBadge />
            <Button>{d("tasks.receive")}</Button>
            <Button variant="primary" onClick={onChat}>
              {d("tasks.chat")}
            </Button>
          </>
        }
      />
      <div style={{ display: "flex", gap: 10 }}>
        {taskKpis.map((k) => (
          <div key={k.label} style={{ flex: 1, background: "var(--card)", border: "1px solid var(--border)", borderRadius: 12, borderLeft: `3px solid ${k.c}`, padding: "10px 13px", minWidth: 0 }}>
            <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".6px", color: "var(--text-3)", display: "flex", gap: 5, alignItems: "center" }}>
              <span style={{ color: k.c, display: "flex" }}>
                <Icon name={k.ic as IconName} size={12} />
              </span>
              <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{k.label}</span>
            </div>
            <div style={{ fontSize: 22, fontWeight: 700, fontVariantNumeric: "tabular-nums", letterSpacing: "-.2px", color: k.vc }}>{k.value}</div>
            <div style={{ fontSize: 10.5, color: "var(--text-4)" }}>{k.sub}</div>
          </div>
        ))}
      </div>
      <Card style={{ display: "flex", alignItems: "center" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "12px 16px", fontWeight: 600, fontSize: 13, flex: "none" }}>
          <span style={{ color: "var(--accent)", display: "flex" }}>
            <Icon name="eye" size={14} />
          </span>
          Phiên theo dõi
        </div>
        <div style={{ flex: 1, display: "grid", gridTemplateColumns: "repeat(4,1fr)", textAlign: "center" }}>
          {[
            { n: "0", l: "Đang theo dõi" },
            { n: "0", l: "KH vừa rep", c: "var(--green)" },
            { n: "0", l: "Tạm dừng", c: "var(--orange)" },
            { n: "0", l: "Chốt tháng" },
          ].map((x) => (
            <div key={x.l} style={{ padding: "9px 0", borderLeft: "1px solid var(--border-soft)" }}>
              <div style={{ fontSize: 19, fontWeight: 700, fontVariantNumeric: "tabular-nums", color: x.c }}>{x.n}</div>
              <div style={{ fontSize: 10.5, color: "var(--text-3)" }}>{x.l}</div>
            </div>
          ))}
        </div>
      </Card>
      <Card>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 14px", borderBottom: "1px solid var(--border-soft)", fontWeight: 600, fontSize: 13 }}>
          <span style={{ color: "var(--red)", display: "flex" }}>
            <Icon name="flame" size={14} />
          </span>
          Cần rep gấp
          <span style={{ marginLeft: "auto", background: "var(--red-bg)", color: "var(--red)", borderRadius: 10, fontSize: 11, fontWeight: 600, padding: "1px 8px" }}>1</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 11, padding: "11px 14px" }}>
          <span style={{ width: 32, height: 32, borderRadius: "50%", background: "#2c3e50", color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 13, flex: "none" }}>
            N
          </span>
          <span style={{ flex: 1, minWidth: 0 }}>
            <span style={{ display: "block", fontWeight: 600, fontSize: 12.5 }}>Nguyên Crypto</span>
            <span style={{ display: "block", fontSize: 11.5, color: "var(--text-3)" }}>phải rồi em</span>
          </span>
          <Badge tone="accent">Mới</Badge>
          <span style={{ fontSize: 11, color: "var(--text-4)" }}>1 ngày trước</span>
          <Badge tone="solid">1</Badge>
        </div>
      </Card>
      <Card>
        <CardHeader icon="history" title="Timeline" meta="DEMO" />
        {taskTimeline.map((t) => (
          <div key={t.time + t.title} style={{ display: "flex", gap: 12, padding: "11px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 12.5 }}>
            <span style={{ width: 72, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{t.time}</span>
            <span style={{ flex: 1 }}>{t.title}</span>
            <Badge tone="neutral">{t.kind}</Badge>
          </div>
        ))}
      </Card>
    </div>
  );
}
