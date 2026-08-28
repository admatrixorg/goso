import { useDemoT } from "../demo/i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { DemoBadge } from "../ui/DemoBadge";
import { Icon } from "../ui/Icon";
import { agentChips, inbox, meetSources, recentMeetings, weekStats } from "../demo/mock";

export function HomePage({ onMeetings, onChat }: { onMeetings: () => void; onChat: () => void }) {
  const { d, t } = useDemoT();
  const h = new Date().getHours();
  const greet = h < 11 ? d("home.greet.morning") : h < 14 ? d("home.greet.noon") : h < 18 ? d("home.greet.afternoon") : d("home.greet.evening");

  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: 0 }}>
      <div style={{ flex: 1, minWidth: 0, display: "flex", justifyContent: "center", padding: "34px 28px 56px" }}>
        <div style={{ width: "100%", maxWidth: 724, display: "flex", flexDirection: "column", gap: 26 }}>
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <div style={{ fontSize: 26, fontWeight: 700, letterSpacing: "-.6px" }}>{greet}</div>
              <DemoBadge />
            </div>
            <div style={{ fontSize: 13.5, color: "var(--text-3)", marginTop: 7 }}>{d("home.prompt")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)", marginTop: 6, lineHeight: 1.5 }}>{d("home.liveGateway")}</div>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 12,
                background: "var(--surface-1)",
                border: "1px solid var(--border)",
                borderRadius: 12,
                padding: "13px 13px 13px 14px",
              }}
            >
              <span
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  background: "var(--accent-soft)",
                  color: "var(--accent)",
                  borderRadius: 8,
                  padding: "4px 9px",
                  fontSize: 12,
                  fontWeight: 600,
                  flex: "none",
                }}
              >
                <Icon name="bolt" size={13} />
                {t("chat.agent")}
              </span>
              <span style={{ flex: 1, fontSize: 13, color: "var(--text-4)", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {d("home.example")}
              </span>
              <button
                type="button"
                onClick={onChat}
                aria-label={t("chat.send")}
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: "50%",
                  background: "var(--btn-dark-bg)",
                  color: "var(--btn-dark-fg)",
                  border: "none",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  flex: "none",
                }}
              >
                <Icon name="arrow-up" size={15} />
              </button>
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
              {agentChips.map((c) => (
                <span
                  key={c}
                  style={{
                    border: "1px solid var(--border)",
                    borderRadius: 999,
                    padding: "7px 14px",
                    fontSize: 12.5,
                    color: "var(--text-2)",
                    background: "var(--card)",
                    cursor: "pointer",
                  }}
                >
                  {c}
                </span>
              ))}
            </div>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)" }}>{d("home.sources")}</div>
              <div style={{ flex: 1 }} />
              <Button icon="plus" variant="secondary" style={{ padding: "6px 12px" }}>
                {d("home.addSource")}
              </Button>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3,1fr)", gap: 11 }}>
              {meetSources.map((m) => (
                <Card key={m.name} style={{ padding: "13px 14px", display: "flex", flexDirection: "column", gap: 8 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span
                      style={{
                        width: 22,
                        height: 22,
                        borderRadius: 6,
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
                      {m.isIcon ? <Icon name="mic" size={13} /> : m.mark}
                    </span>
                    <b style={{ fontSize: 13, fontWeight: 600 }}>{m.name}</b>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: m.stateC }}>
                    <span style={{ width: 7, height: 7, borderRadius: "50%", background: m.dot, flex: "none" }} />
                    {m.state}
                  </div>
                  <div style={{ fontSize: 12, color: "var(--text-3)" }}>{m.note}</div>
                  {m.needsConnect ? (
                    <Button variant="quiet" style={{ alignSelf: "flex-start", padding: "5px 12px" }}>
                      {t("common.connect")}
                    </Button>
                  ) : null}
                </Card>
              ))}
            </div>
            <div
              style={{
                display: "flex",
                gap: 10,
                background: "var(--warn-bg)",
                border: "1px solid var(--hint-border)",
                borderRadius: 11,
                padding: "12px 14px",
              }}
            >
              <span style={{ color: "var(--orange)", display: "flex", flex: "none", marginTop: 1 }}>
                <Icon name="shield" size={15} />
              </span>
              <div style={{ fontSize: 12, color: "var(--warn-text)", lineHeight: 1.6 }}>
                {d("home.privacy")}
              </div>
            </div>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)" }}>{d("home.recent")}</div>
              <div style={{ flex: 1 }} />
              <button type="button" onClick={onMeetings} style={{ background: "none", border: "none", fontSize: 12.5, color: "var(--accent)", fontWeight: 500 }}>
                {d("home.seeAll")}
              </button>
            </div>
            <Card>
              {recentMeetings.map((m) => (
                <button
                  key={m.title}
                  type="button"
                  onClick={onMeetings}
                  style={{
                    display: "flex",
                    width: "100%",
                    alignItems: "center",
                    gap: 11,
                    padding: "12px 14px",
                    border: "none",
                    borderBottom: "1px solid var(--border-soft)",
                    background: "transparent",
                    textAlign: "left",
                  }}
                >
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
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{m.title}</div>
                    <div style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>
                      <Icon name="clock" size={12} />
                      <span>
                        {m.when} · {m.mins}
                      </span>
                      <Icon name="friends" size={12} />
                      <span>{m.people}</span>
                    </div>
                  </div>
                  {m.sbg ? (
                    <Badge tone="accent">{m.status}</Badge>
                  ) : (
                    <span style={{ color: m.sc, fontSize: 11.5, whiteSpace: "nowrap", display: "flex", alignItems: "center", gap: 6 }}>
                      {m.pending ? (
                        <span data-motion="pulse" style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--accent)", flex: "none", animation: "zPulse 1.6s linear infinite" }} />
                      ) : null}
                      {m.status}
                    </span>
                  )}
                  <Icon name="chev-right" size={14} style={{ color: "var(--text-4)" }} />
                </button>
              ))}
            </Card>
          </div>
        </div>
      </div>

      <div style={{ width: 268, flex: "none", padding: "34px 24px 56px", display: "flex", flexDirection: "column", gap: 24 }}>
        <div>
          <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)", marginBottom: 11 }}>{d("home.inbox")}</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {inbox.map((n) => (
              <div key={n.label} style={{ display: "flex", gap: 9, alignItems: "flex-start", padding: "7px 8px", borderRadius: 8, fontSize: 12.5 }}>
                <span style={{ width: 7, height: 7, borderRadius: "50%", background: n.dot, flex: "none", marginTop: 6 }} />
                {n.label}
              </div>
            ))}
          </div>
        </div>
        <div>
          <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)", marginBottom: 11 }}>{d("home.week")}</div>
          {weekStats.map((w) => (
            <div key={w.label} style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 8px", fontSize: 12.5, color: "var(--text-2)" }}>
              <span style={{ flex: 1 }}>{w.label}</span>
              <b style={{ fontVariantNumeric: "tabular-nums", color: "var(--text)", fontWeight: 600 }}>{w.value}</b>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
