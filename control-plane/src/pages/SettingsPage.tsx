import { useState } from "react";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { Icon, type IconName } from "../ui/Icon";
import { meetSources } from "../demo/mock";

const MENU: { group: string; items: { id: string; label: string; ic: IconName }[] }[] = [
  { group: "NGUỒN", items: [{ id: "sources", label: "Nguồn cuộc họp", ic: "mic" }] },
  { group: "TÀI KHOẢN", items: [{ id: "account", label: "Tài khoản", ic: "user" }] },
  { group: "GIAO DIỆN", items: [{ id: "theme", label: "Giao diện", ic: "sun" }] },
];

export function SettingsPage({ dark, onToggleTheme }: { dark: boolean; onToggleTheme: () => void }) {
  const [page, setPage] = useState("sources");

  return (
    <div style={{ display: "flex", height: "100%", minHeight: 0 }}>
      <div style={{ width: 250, flex: "none", background: "var(--card)", borderRight: "1px solid var(--border)", overflowY: "auto", padding: "14px 10px" }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", fontWeight: 700, fontSize: 15, padding: "0 8px 10px" }}>
          Cài đặt <DemoBadge />
        </div>
        {MENU.map((g) => (
          <div key={g.group}>
            <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)", padding: "10px 12px 4px" }}>{g.group}</div>
            {g.items.map((i) => {
              const on = page === i.id;
              return (
                <button
                  key={i.id}
                  type="button"
                  onClick={() => setPage(i.id)}
                  style={{
                    width: "100%",
                    border: "none",
                    borderRadius: 8,
                    padding: "7px 12px",
                    fontSize: 13,
                    display: "flex",
                    gap: 9,
                    alignItems: "center",
                    background: on ? "var(--accent-soft)" : "transparent",
                    color: on ? "var(--accent)" : "var(--text-2)",
                    fontWeight: on ? 600 : 400,
                    textAlign: "left",
                  }}
                >
                  <Icon name={i.ic} size={14} />
                  {i.label}
                </button>
              );
            })}
          </div>
        ))}
      </div>
      <div style={{ flex: 1, overflowY: "auto", padding: "16px 26px", display: "flex", flexDirection: "column", gap: 12 }}>
        {page === "sources" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>Nguồn cuộc họp</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>Kết nối Meet / Zoom / file offline. Trạng thái dưới đây là DEMO — chưa persist.</div>
            {meetSources.map((m) => (
              <Card key={m.name} style={{ padding: 14, display: "flex", alignItems: "center", gap: 12 }}>
                <b style={{ flex: 1 }}>{m.name}</b>
                <span style={{ color: m.stateC, fontSize: 12.5 }}>{m.state}</span>
                {m.needsConnect ? <Button>Kết nối</Button> : <Button variant="quiet">Ngắt</Button>}
              </Card>
            ))}
          </>
        )}
        {page === "account" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>Tài khoản</div>
            <Card>
              <CardHeader icon="user" title="Control Plane" meta="không lưu secret trên UI" />
              <div style={{ padding: 16, fontSize: 13, color: "var(--text-2)", lineHeight: 1.6 }}>
                Token admin lấy từ <code>VITE_GOSO_ADMIN_TOKEN</code> hoặc <code>localStorage.goso_token</code>. Org CRM:{" "}
                <code>VITE_GOSOCRM_ORG_ID</code>. Không hiện giá trị secret.
              </div>
            </Card>
          </>
        )}
        {page === "theme" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>Giao diện</div>
            <Card style={{ padding: 16, display: "flex", alignItems: "center", gap: 12 }}>
              <span style={{ flex: 1, fontSize: 13 }}>Dark mode là token flip, không phải design thứ hai.</span>
              <Button variant="primary" onClick={onToggleTheme}>
                {dark ? "Chuyển sáng" : "Chuyển tối"}
              </Button>
            </Card>
          </>
        )}
      </div>
    </div>
  );
}
