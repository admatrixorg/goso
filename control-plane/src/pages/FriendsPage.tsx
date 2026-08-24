import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { DemoBadge } from "../ui/DemoBadge";
import { friends } from "../demo/mock";

export function FriendsPage() {
  return (
    <div style={{ display: "flex", height: "100%", background: "var(--card)" }}>
      <div style={{ width: 230, flex: "none", borderRight: "1px solid var(--border)", padding: 12, display: "flex", flexDirection: "column", gap: 9 }}>
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 10.5, fontWeight: 700, letterSpacing: ".5px", color: "var(--text-2)" }}>
          NICK ZALO CỦA BẠN <DemoBadge />
        </div>
        <div style={{ background: "var(--surface-2)", borderRadius: 8, padding: "6px 10px", fontSize: 12, color: "var(--text-4)" }}>Tìm nick…</div>
        <div style={{ display: "flex", gap: 8, alignItems: "center", border: "1.5px solid var(--accent)", background: "var(--accent-soft)", borderRadius: 10, padding: "8px 10px" }}>
          <div style={{ width: 26, height: 26, borderRadius: "50%", background: "#c0392b", color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 11 }}>
            PN
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 12.5, fontWeight: 600 }}>Phan Nhung</div>
            <div style={{ fontSize: 10.5, color: "var(--text-3)" }}>0918287799</div>
          </div>
        </div>
        <div style={{ marginTop: "auto", fontSize: 11, color: "var(--text-3)" }}>
          <span style={{ color: "var(--green)" }}>●</span> DEMO · không gọi Zalo
        </div>
      </div>
      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", padding: "12px 16px", gap: 10 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ fontSize: 16, fontWeight: 700 }}>Bạn bè</div>
          <DemoBadge />
          <div style={{ marginLeft: "auto", display: "flex", gap: 8 }}>
            <Button icon="refresh" iconGesture>
              Làm mới ngay
            </Button>
          </div>
        </div>
        <div style={{ flex: 1, border: "1px solid var(--border)", borderRadius: 12, overflow: "auto" }}>
          <div style={{ display: "flex", gap: 8, padding: "8px 12px", background: "var(--surface-1)", borderBottom: "1px solid var(--border)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 2.2 }}>KHÁCH HÀNG</span>
            <span style={{ flex: 1 }}>TRẠNG THÁI KB</span>
            <span style={{ width: 58 }}>SCORE</span>
            <span style={{ width: 56 }}>TIN (I/O)</span>
          </div>
          {friends.map((f) => (
            <div key={f.name} style={{ display: "flex", gap: 8, padding: "9px 12px", borderBottom: "1px solid var(--border-soft)", fontSize: 12, alignItems: "center" }}>
              <div style={{ flex: 2.2, display: "flex", gap: 8, alignItems: "center", minWidth: 0 }}>
                <div style={{ width: 30, height: 30, borderRadius: "50%", background: f.av, color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 11, flex: "none" }}>
                  {f.ini}
                </div>
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 600 }}>{f.name}</div>
                  <div style={{ fontSize: 10, color: "var(--text-4)" }}>{f.meta}</div>
                </div>
              </div>
              <span style={{ flex: 1 }}>
                <Badge tone="positive">Đã kết bạn</Badge>
              </span>
              <span style={{ width: 58, fontVariantNumeric: "tabular-nums" }}>{f.scoreN}</span>
              <span style={{ width: 56, fontVariantNumeric: "tabular-nums", color: "var(--text-2)" }}>{f.msgs}</span>
            </div>
          ))}
        </div>
        <div style={{ fontSize: 12, color: "var(--text-3)", fontStyle: "italic" }}>4 hàng DEMO — chưa nối nick Zalo.</div>
      </div>
    </div>
  );
}
