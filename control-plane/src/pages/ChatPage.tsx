import { useEffect, useRef, useState } from "react";
import { api, PROMPT_MODES, type Message } from "../api/client";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function promptModeKey(mode: string): MsgKey {
  if (mode === "task") return "promptMode.task";
  if (mode === "minimal") return "promptMode.minimal";
  if (mode === "none") return "promptMode.none";
  return "promptMode.full";
}

function normalizePromptMode(mode?: string): string {
  const v = (mode || "").trim().toLowerCase();
  return PROMPT_MODES.includes(v as (typeof PROMPT_MODES)[number]) ? v : "full";
}

let localSeq = 0;
function localId(prefix: string): string {
  localSeq += 1;
  return `${prefix}-${Date.now()}-${localSeq}`;
}

export function ChatPage({
  sessionId,
  sessionLabel,
  onNew,
}: {
  sessionId: string;
  sessionLabel?: string;
  onNew?: () => void;
}) {
  const { t } = useI18n();
  const named = sessionLabel?.trim() || sessionId;
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(() => Boolean(sessionId));
  const [sending, setSending] = useState(false);
  const [promptMode, setPromptMode] = useState("full");
  const [savingMode, setSavingMode] = useState(false);
  const genRef = useRef(0);
  const sessionRef = useRef(sessionId);
  sessionRef.current = sessionId;

  function stillCurrent(forSession: string, gen: number): boolean {
    return gen === genRef.current && sessionRef.current === forSession;
  }

  async function load(forSession: string, gen: number) {
    try {
      const [msgRes, sessRes] = await Promise.allSettled([api.listMessages(forSession), api.listSessions()]);
      if (!stillCurrent(forSession, gen)) return;
      if (msgRes.status === "rejected") {
        setErr(formatPublicError(msgRes.reason));
        return;
      }
      setMsgs(msgRes.value.messages ?? []);
      if (sessRes.status === "fulfilled") {
        const sess = (sessRes.value.sessions ?? []).find((s) => s.id === forSession);
        setPromptMode(normalizePromptMode(sess?.prompt_mode));
      }
      setErr("");
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      setErr(formatPublicError(e));
    } finally {
      if (stillCurrent(forSession, gen)) setLoading(false);
    }
  }

  async function persistMode(next: string) {
    if (!sessionId || savingMode) return;
    const prev = promptMode;
    const mode = normalizePromptMode(next);
    setPromptMode(mode);
    setSavingMode(true);
    try {
      await api.updateSession(sessionId, { prompt_mode: mode });
      setErr("");
    } catch (e) {
      setPromptMode(prev);
      setErr(formatPublicError(e));
    } finally {
      setSavingMode(false);
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
    setSending(false);
    void load(sessionId, gen);
  }, [sessionId]);

  async function send() {
    const text = input.trim();
    if (!text || sending || savingMode || !sessionId) return;
    const forSession = sessionId;
    const gen = genRef.current;
    const userMsg: Message = {
      id: localId("local-user"),
      session_id: forSession,
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    };
    const asst: Message = {
      id: localId("local-asst"),
      session_id: forSession,
      role: "assistant",
      content: "",
      created_at: new Date().toISOString(),
    };
    setSending(true);
    setMsgs((m) => [...m, userMsg, asst]);
    setInput("");
    setErr("");
    try {
      await api.chatStream({ session_id: forSession, message: text, prompt_mode: promptMode }, (delta) => {
        if (!stillCurrent(forSession, gen)) return;
        setMsgs((m) => m.map((x) => (x.id === asst.id ? { ...x, content: x.content + delta } : x)));
      });
      if (!stillCurrent(forSession, gen)) return;
      await load(forSession, gen);
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      const msg = formatPublicError(e);
      setErr(msg);
      setMsgs((m) => {
        const without = m.filter((x) => x.id !== asst.id);
        return [
          ...without,
          {
            id: localId("local-err"),
            session_id: forSession,
            role: "assistant",
            content: msg,
            created_at: new Date().toISOString(),
          },
        ];
      });
    } finally {
      if (stillCurrent(forSession, gen)) setSending(false);
    }
  }

  if (!sessionId) {
    return (
      <div style={{ padding: "14px 22px 40px" }}>
        <SectionHeader icon="msg" title={t("chat.title")} description={t("chat.desc")} />
        <EmptyState>{t("chat.emptySession")}</EmptyState>
        {onNew ? (
          <div style={{ display: "flex", justifyContent: "center", marginTop: 8 }}>
            <Button variant="primary" icon="plus" onClick={onNew}>
              {t("chat.newSession")}
            </Button>
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <div style={{ padding: "14px 22px 0" }}>
        <SectionHeader
          icon="msg"
          title={named}
          description={t("chat.descSession", { id: sessionId })}
          actions={
            <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
              <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: "var(--text-2)" }}>
                {t("chat.promptMode")}
                <select
                  className="z-field"
                  aria-label={t("chat.promptMode")}
                  value={promptMode}
                  disabled={savingMode || sending}
                  onChange={(e) => void persistMode(e.target.value)}
                  style={{ minWidth: 120 }}
                >
                  {PROMPT_MODES.map((m) => (
                    <option key={m} value={m}>
                      {t(promptModeKey(m))}
                    </option>
                  ))}
                </select>
              </label>
              <Button icon="refresh" iconGesture onClick={() => void load(sessionId, genRef.current)}>
                {t("common.refresh")}
              </Button>
            </div>
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
      <div className="z-chat-composer">
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
          <span className="z-wide-only">{t("chat.agent")}</span>
        </span>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 0 }}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Enter") return;
            if (e.nativeEvent.isComposing || e.keyCode === 229) return;
            void send();
          }}
          disabled={sending || savingMode}
          placeholder={t("chat.placeholder")}
        />
        <button
          type="button"
          onClick={() => void send()}
          disabled={sending || savingMode}
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
