import { useEffect, useState } from "react";
import { api, type Message } from "../api/client";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { SectionHeader } from "../ui/SectionHeader";

export function ChatPage({ sessionId }: { sessionId: string }) {
  const { t } = useI18n();
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await api.listMessages(sessionId);
      setMsgs(j.messages ?? []);
      setErr("");
    } catch (e) {
      setErr(String(e));
    }
  }
  useEffect(() => {
    if (sessionId) void load();
  }, [sessionId]);

  async function send() {
    if (!input.trim()) return;
    try {
      await api.chat({ session_id: sessionId, message: input.trim() });
      setInput("");
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  if (!sessionId) {
    return (
      <div style={{ padding: "14px 22px 40px" }}>
        <SectionHeader icon="msg" title={t("chat.title")} description={t("chat.desc")} />
        <EmptyState>{t("chat.emptySession")}</EmptyState>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <div style={{ padding: "14px 22px 0" }}>
        <SectionHeader
          icon="msg"
          title={t("chat.title")}
          description={t("chat.descSession", { id: sessionId })}
          actions={
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
          }
        />
      </div>
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: "8px 22px 0" }}>{err}</p> : null}
      <div style={{ flex: 1, overflow: "auto", padding: "14px 22px", display: "flex", flexDirection: "column", gap: 10 }}>
        {msgs.map((m) => (
          <div
            key={m.id}
            style={{
              alignSelf: m.role === "user" ? "flex-end" : "flex-start",
              maxWidth: "72%",
              background: m.role === "user" ? "var(--accent-soft)" : "var(--card)",
              border: "1px solid var(--border)",
              borderRadius: 12,
              padding: "10px 13px",
              fontSize: 13,
            }}
          >
            <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".4px", color: "var(--text-3)", marginBottom: 4 }}>
              {m.role.toUpperCase()}
            </div>
            <div style={{ whiteSpace: "pre-wrap", textWrap: "pretty" as const }}>{m.content}</div>
          </div>
        ))}
        {msgs.length === 0 ? <EmptyState>{t("chat.empty")}</EmptyState> : null}
      </div>
      <div
        style={{
          padding: "12px 22px 18px",
          borderTop: "1px solid var(--border)",
          background: "var(--chrome)",
          display: "flex",
          alignItems: "center",
          gap: 12,
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
        <input
          className="z-field"
          style={{ flex: 1 }}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void send()}
          placeholder={t("chat.placeholder")}
        />
        <button
          type="button"
          onClick={() => void send()}
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
    </div>
  );
}
