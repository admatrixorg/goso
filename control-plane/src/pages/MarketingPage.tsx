import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { EmptyState } from "../ui/EmptyState";
import { mkMenu } from "../demo/mock";

export function MarketingPage() {
  return (
    <div style={{ display: "flex", height: "100%", minHeight: 0 }}>
      <div style={{ width: 210, flex: "none", background: "var(--card)", borderRight: "1px solid var(--border)", padding: "14px 10px", display: "flex", flexDirection: "column", gap: 4 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", fontWeight: 700, fontSize: 15, padding: "0 8px 10px" }}>
          Marketing <DemoBadge />
        </div>
        {mkMenu.map((m, i) => (
          <div
            key={m}
            style={{
              borderRadius: 8,
              padding: "7px 12px",
              fontSize: 13,
              background: i === 0 ? "var(--accent-soft)" : "transparent",
              color: i === 0 ? "var(--accent)" : "var(--text-2)",
              fontWeight: i === 0 ? 600 : 400,
            }}
          >
            {m}
          </div>
        ))}
      </div>
      <div style={{ flex: 1, overflowY: "auto", padding: "14px 22px", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 17, fontWeight: 700 }}>Tệp khách hàng</div>
            <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>
              Paste / Excel / Lead Ads đổ về tệp. Placeholder — chưa có API marketing trên GOSO gateway.
            </div>
          </div>
          <Button variant="primary" icon="plus">
            Tạo tệp
          </Button>
        </div>
        <div style={{ display: "flex", gap: 10 }}>
          {["Tổng tệp", "Lead Ads", "Paste / File", "SĐT trong các tệp", "SĐT có Zalo"].map((l) => (
            <Card key={l} style={{ flex: 1, padding: "11px 14px" }}>
              <div style={{ fontSize: 20, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>0</div>
              <div style={{ fontSize: 11, color: "var(--text-3)" }}>{l}</div>
            </Card>
          ))}
        </div>
        <EmptyState>
          Chưa có tệp khách hàng nào.
          <div style={{ fontStyle: "normal", marginTop: 8 }}>Bấm "Tạo tệp" không ghi DB — DEMO.</div>
        </EmptyState>
      </div>
    </div>
  );
}
