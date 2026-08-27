import { useEffect, useRef, useState } from "react";
import { api, type Message } from "../api/client";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

let localSeq = 0;
function localId(prefix: string): string {
  localSeq += 1;
  return `${prefix}-${Date.now()}-${localSeq}`;
}

export function ChatPage({ sessionId }: { sessionId: string }) {
  const { t } = useI18n();
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(() => Boolean(sessionId));
  const [sending, setSending] = useState(false);
  const genRef = useRef(0);
  const sessionRef = useRef(sessionId);
  sessionRef.current = sessionId;

  function stillCurrent(forSession: string, gen: number): boolean {
    return gen === genRef.current && sessionRef.current === forSession;
  }

  async function load(forSession: string, gen: number) {
    try {
      const j = await api.listMessages(forSession);
      if (!stillCurrent(forSession, gen)) return;
      setMsgs(j.messages ?? []);
      setErr("");
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      setErr(formatPublicError(e));
    } finally {
      if (stillCurrent(forSession, gen)) setLoading(false);
    }
  }
  useEffect(() => {
    const gen = genRef.current + 1;
    genRef.current = gen;
    if (!sessionId) {
      setMsgs([]);
      setErr("");
      setLoading(false);
      setSending(false);
      return;
    }
    setLoading(true);
    setMsgs([]);
    setErr("");
    void load(sessionId, gen);
  }, [sessionId]);

  async function send() {
    const text = input.trim();
    if (!text || sending || !sessionId) return;
    const forSession = sessionId;
    const gen = genRef.current;
    const userMsg: Message = {
      id: localId("local-user"),
      session_id: forSession,
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    };
    setSending(true);
    setMsgs((m) => [...m, userMsg]);
    setInput("");
    setErr("");
    try {
      await api.chat({ session_id: forSession, message: text });
      if (!stillCurrent(forSession, gen)) return;
      await load(forSession, gen);
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      const msg = formatPublicError(e);
      setErr(msg);
      setMsgs((m) => [
        ...m,
        {
          id: localId("local-err"),
          session_id: forSession,
          role: "assistant",
          content: msg,
          created_at: new Date().toISOString(),
        },
      ]);
    } finally {
      if (stillCurrent(forSession, gen)) setSending(false);
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
            <Button icon="refresh" iconGesture onClick={() => void load(sessionId, genRef.current)}>
              {t("common.refresh")}
            </Button>
          }
        />
      </div>
      {err ? (
        <div style={{ margin: "8px 22px 0" }}>
          <StatusLine kind="error">{err}</StatusLine>
        </div>
      ) : null}
      <div style={{ flex: 1, overflow: "auto", padding: "14px 22px", display: "flex", flexDirection: "column", gap: 10 }}>
        {loading ? <StatusLine kind="loading" /> : null}
        {!loading
          ? msgs.map((m) => {
              const sendErr = m.id.startsWith("local-err-");
              return (
                <div
                  key={m.id}
                  style={{
                    alignSelf: m.role === "user" ? "flex-end" : "flex-start",
                    maxWidth: "72%",
                    background: m.role === "user" ? "var(--accent-soft)" : "var(--card)",
                    border: `1px solid ${sendErr ? "var(--red)" : "var(--border)"}`,
                    borderRadius: 12,
                    padding: "10px 13px",
                    fontSize: 13,
                  }}
                >
                  <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".4px", color: "var(--text-3)", marginBottom: 4 }}>
                    {m.role.toUpperCase()}
                  </div>
                  <div
                    style={{
                      whiteSpace: "pre-wrap",
                      textWrap: "pretty" as const,
                      color: sendErr ? "var(--red)" : undefined,
                    }}
                  >
                    {m.content}
                  </div>
                </div>
              );
            })
          : null}
        {!loading && msgs.length === 0 ? <EmptyState>{t("chat.empty")}</EmptyState> : null}
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
          onKeyDown={(e) => {
            if (e.key !== "Enter") return;
            if (e.nativeEvent.isComposing || e.keyCode === 229) return;
            void send();
          }}
          disabled={sending}
          placeholder={t("chat.placeholder")}
        />
        <button
          type="button"
          onClick={() => void send()}
          disabled={sending}
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
