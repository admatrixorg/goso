import { useEffect, useRef, useState } from "react";
import { api, PROMPT_MODES, type Message } from "../api/client";
import { formatPublicError } from "../api/public-error";
import {
  isGoneStatus,
  normalizePromptMode,
  streamReconnectDelayMs,
  type ChatStreamState,
} from "../api/sessions";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine } from "../ui/StatusLine";

function promptModeKey(mode: string): MsgKey {
  if (mode === "task") return "promptMode.task";
  if (mode === "minimal") return "promptMode.minimal";
  if (mode === "none") return "promptMode.none";
  return "promptMode.full";
}

function streamKey(state: ChatStreamState): MsgKey | null {
  if (state === "connecting") return "chat.stream.connecting";
  if (state === "streaming") return "chat.stream.streaming";
  if (state === "reconnect") return "chat.stream.reconnect";
  if (state === "error") return "chat.stream.error";
  return null;
}

function streamTone(state: ChatStreamState): "accent" | "warning" | "neutral" {
  if (state === "reconnect") return "warning";
  if (state === "streaming" || state === "connecting") return "accent";
  return "neutral";
}

let localSeq = 0;
function localId(prefix: string): string {
  localSeq += 1;
  return `${prefix}-${Date.now()}-${localSeq}`;
}

export function ChatPage({
  sessionId,
  sessionLabel,
  onNew: _onNew,
  onGone,
}: {
  sessionId: string;
  sessionLabel?: string;
  onNew?: () => void;
  onGone?: (id: string) => void;
}) {
  void _onNew;
  const { t } = useI18n();
  const named = sessionLabel?.trim() || sessionId;
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(() => Boolean(sessionId));
  const [sending, setSending] = useState(false);
  const [stream, setStream] = useState<ChatStreamState>("idle");
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
        const reason = msgRes.reason;
        setErr(formatPublicError(reason));
        if (isGoneStatus(reason)) onGone?.(forSession);
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
      if (isGoneStatus(e)) onGone?.(forSession);
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
      setStream("idle");
      return;
    }
    setLoading(true);
    setMsgs([]);
    setErr("");
    setSending(false);
    setStream("idle");
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
    setStream("connecting");
    setMsgs((m) => [...m, userMsg, asst]);
    setInput("");
    setErr("");
    try {
      await api.chatStream({ session_id: forSession, message: text, prompt_mode: promptMode }, (delta) => {
        if (!stillCurrent(forSession, gen)) return;
        setStream("streaming");
        setMsgs((m) => m.map((x) => (x.id === asst.id ? { ...x, content: x.content + delta } : x)));
      });
      if (!stillCurrent(forSession, gen)) return;
      setStream("idle");
      await load(forSession, gen);
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      const msg = formatPublicError(e);
      setErr(msg);
      setStream("reconnect");
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
      await new Promise((r) => setTimeout(r, streamReconnectDelayMs(0)));
      if (!stillCurrent(forSession, gen)) return;
      try {
        await api.listMessages(forSession);
      } catch (probe) {
        if (!stillCurrent(forSession, gen)) return;
        if (isGoneStatus(probe)) onGone?.(forSession);
      }
      if (!stillCurrent(forSession, gen)) return;
      setErr(msg);
      setStream("error");
    } finally {
      if (stillCurrent(forSession, gen)) {
        setSending(false);
        setStream((cur) => (cur === "connecting" || cur === "streaming" || cur === "reconnect" ? "idle" : cur));
      }
    }
  }

  if (!sessionId) {
    return (
      <div style={{ padding: "14px 22px 40px" }} data-chat-state="no-session">
        <SectionHeader icon="msg" title={t("chat.title")} description={t("chat.desc")} />
        <EmptyState>{t("chat.emptySession")}</EmptyState>
      </div>
    );
  }

  const streamLabel = streamKey(stream);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <div style={{ padding: "14px 22px 0" }}>
        <SectionHeader
          icon="msg"
          title={named}
          description={t("chat.descSession", { id: sessionId })}
          actions={
            <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
              {streamLabel ? (
                <Badge tone={streamTone(stream)} role="status" data-chat-stream={stream}>
                  {t(streamLabel)}
                </Badge>
              ) : null}
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
                    {sendErr ? formatPublicError(m.content) : m.content}
                  </div>
                </div>
              );
            })
          : null}
        {!loading && !err && msgs.length === 0 ? <EmptyState>{t("chat.empty")}</EmptyState> : null}
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
          disabled={sending || savingMode || !input.trim()}
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
      <div style={{ padding: "0 22px 10px", display: "flex", gap: 10, flexWrap: "wrap", fontSize: 11.5, color: "var(--text-3)" }}>
        <span>{t("chat.attachUnavailable")}</span>
        <span>{t("chat.voiceUnavailable")}</span>
        <span>{t("chat.contextUnavailable")}</span>
      </div>
    </div>
  );
}
