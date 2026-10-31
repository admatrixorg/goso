import { useEffect, useRef, useState } from "react";
import { api, PROMPT_MODES, type Agent, type Message } from "../api/client";
import {
  agentDisplayName,
  applyMention,
  choiceFromSelect,
  ensureModelChoice,
  filterMentionAgents,
  highlightMentions,
  mentionQueryAt,
  mentionStillPresent,
  modelOptions,
  replyCaption,
  selectValueForChoice,
  type ModelChoice,
  type TurnAgent,
} from "../api/chat-turn";
import { formatPublicError } from "../api/public-error";
import {
  agentLabel,
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

type Mention = { id: string; name: string };

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
  const [agents, setAgents] = useState<Agent[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(() => Boolean(sessionId));
  const [sending, setSending] = useState(false);
  const [stream, setStream] = useState<ChatStreamState>("idle");
  const [promptMode, setPromptMode] = useState("full");
  const [sessionAgent, setSessionAgent] = useState("");
  const [modelOverride, setModelOverride] = useState("");
  const [providerOverride, setProviderOverride] = useState("");
  const [savingMeta, setSavingMeta] = useState(false);
  const [mention, setMention] = useState<Mention | null>(null);
  const [mentionQ, setMentionQ] = useState<string | null>(null);
  const genRef = useRef(0);
  const sessionRef = useRef(sessionId);
  const inputRef = useRef<HTMLInputElement>(null);
  const agentSelectRef = useRef<HTMLSelectElement>(null);
  sessionRef.current = sessionId;

  function stillCurrent(forSession: string, gen: number): boolean {
    return gen === genRef.current && sessionRef.current === forSession;
  }

  async function load(forSession: string, gen: number) {
    try {
      const [msgRes, sessRes, agentRes] = await Promise.allSettled([
        api.listMessages(forSession),
        api.listSessions(),
        api.listAgents(),
      ]);
      if (!stillCurrent(forSession, gen)) return;
      if (agentRes.status === "fulfilled") setAgents(agentRes.value.agents ?? []);
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
        setSessionAgent(sess?.agent_id || "");
        setModelOverride(sess?.model_override || "");
        setProviderOverride(sess?.provider_override || "");
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
    if (!sessionId || savingMeta) return;
    const prev = promptMode;
    const mode = normalizePromptMode(next);
    setPromptMode(mode);
    setSavingMeta(true);
    try {
      await api.updateSession(sessionId, { prompt_mode: mode });
      setErr("");
    } catch (e) {
      setPromptMode(prev);
      setErr(formatPublicError(e));
    } finally {
      setSavingMeta(false);
    }
  }

  async function persistAgent(next: string) {
    if (!sessionId || savingMeta || !next || next === sessionAgent) return;
    const prev = sessionAgent;
    setSessionAgent(next);
    setSavingMeta(true);
    try {
      await api.updateSession(sessionId, { agent_id: next });
      setErr("");
      await load(sessionId, genRef.current);
    } catch (e) {
      setSessionAgent(prev);
      setErr(formatPublicError(e));
    } finally {
      setSavingMeta(false);
    }
  }

  async function persistModel(value: string) {
    if (!sessionId || savingMeta) return;
    const choice = choiceFromSelect(value);
    const prevM = modelOverride;
    const prevP = providerOverride;
    setModelOverride(choice?.model || "");
    setProviderOverride(choice?.provider || "");
    setSavingMeta(true);
    try {
      await api.updateSession(sessionId, {
        model_override: choice?.model || "",
        provider_override: choice?.provider || "",
      });
      setErr("");
    } catch (e) {
      setModelOverride(prevM);
      setProviderOverride(prevP);
      setErr(formatPublicError(e));
    } finally {
      setSavingMeta(false);
    }
  }

  useEffect(() => {
    const gen = genRef.current + 1;
    genRef.current = gen;
    if (!sessionId) {
      setMsgs([]);
      setAgents([]);
      setErr("");
      setLoading(false);
      setSending(false);
      setStream("idle");
      setSessionAgent("");
      setModelOverride("");
      setProviderOverride("");
      setMention(null);
      setMentionQ(null);
      return;
    }
    setLoading(true);
    setMsgs([]);
    setErr("");
    setSending(false);
    setStream("idle");
    setMention(null);
    setMentionQ(null);
    void load(sessionId, gen);
  }, [sessionId]);

  function pickMention(agent: TurnAgent) {
    const name = agentDisplayName(agent);
    const el = inputRef.current;
    const cursor = el?.selectionStart ?? input.length;
    const next = applyMention(input, cursor, name);
    setInput(next.text);
    setMention({ id: agent.id, name });
    setMentionQ(null);
    requestAnimationFrame(() => {
      const node = inputRef.current;
      if (!node) return;
      node.focus();
      node.setSelectionRange(next.cursor, next.cursor);
    });
  }

  function onInput(value: string, cursor: number) {
    setInput(value);
    setMentionQ(mentionQueryAt(value, cursor));
    setMention((cur) => (cur && mentionStillPresent(value, cur.name) ? cur : null));
  }

  async function send() {
    const text = input.trim();
    if (!text || sending || savingMeta || !sessionId) return;
    const forSession = sessionId;
    const gen = genRef.current;
    const activeMention = mention && mentionStillPresent(text, mention.name) ? mention : null;
    const turnAgent = activeMention?.id || sessionAgent;
    const turnModel = modelOverride || agents.find((a) => a.id === turnAgent)?.model || "";
    const userMsg: Message = {
      id: localId("local-user"),
      session_id: forSession,
      role: "user",
      content: text,
      agent_id: sessionAgent,
      mention_agent_id: activeMention?.id,
      created_at: new Date().toISOString(),
    };
    const asst: Message = {
      id: localId("local-asst"),
      session_id: forSession,
      role: "assistant",
      content: "",
      agent_id: turnAgent,
      model: turnModel,
      created_at: new Date().toISOString(),
    };
    setSending(true);
    setStream("connecting");
    setMsgs((m) => [...m, userMsg, asst]);
    setInput("");
    setMention(null);
    setMentionQ(null);
    setErr("");
    try {
      await api.chatStream(
        {
          session_id: forSession,
          message: text,
          prompt_mode: promptMode,
          ...(activeMention ? { agent_id: activeMention.id } : {}),
        },
        (delta) => {
          if (!stillCurrent(forSession, gen)) return;
          setStream("streaming");
          setMsgs((m) => m.map((x) => (x.id === asst.id ? { ...x, content: x.content + delta } : x)));
        },
      );
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
  const names = agents.map(agentDisplayName);
  const choices = ensureModelChoice(modelOptions(agents), providerOverride, modelOverride);
  const modelValue = modelOverride
    ? selectValueForChoice({ provider: providerOverride, model: modelOverride })
    : "";
  const mentionHits = mentionQ === null ? [] : filterMentionAgents(agents, mentionQ);
  const busy = sending || savingMeta;

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
                  disabled={busy}
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
              if (m.role === "event") {
                const who = agentLabel(agents, m.agent_id || "");
                return (
                  <div key={m.id} className="z-chat-switch" data-chat-switch={m.agent_id || ""}>
                    {t("chat.switched", { name: who })}
                  </div>
                );
              }
              const sendErr = m.id.startsWith("local-err-");
              const caption =
                m.role === "assistant" && !sendErr
                  ? replyCaption(m.agent_id ? agentLabel(agents, m.agent_id) : "", m.model || "")
                  : "";
              const spans = m.role === "user" ? highlightMentions(m.content, names) : [{ text: m.content, mention: false }];
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
                    {sendErr
                      ? formatPublicError(m.content)
                      : spans.map((span, i) =>
                          span.mention ? (
                            <span key={i} className="z-chat-mention">
                              {span.text}
                            </span>
                          ) : (
                            <span key={i}>{span.text}</span>
                          ),
                        )}
                  </div>
                  {caption ? (
                    <div className="z-chat-caption" data-reply-caption={caption}>
                      {caption}
                    </div>
                  ) : null}
                </div>
              );
            })
          : null}
        {!loading && !err && msgs.length === 0 ? <EmptyState>{t("chat.empty")}</EmptyState> : null}
      </div>
      <div className="z-chat-composer">
        {mentionQ !== null ? (
          <div className="z-chat-mention-pop" role="listbox" aria-label={t("chat.mentionList")} data-chat-mention>
            {mentionHits.length === 0 ? (
              <div style={{ padding: "8px 10px", fontSize: 13, color: "var(--text-3)" }}>{t("chat.mentionEmpty")}</div>
            ) : (
              mentionHits.map((a, i) => (
                <button
                  key={a.id}
                  type="button"
                  role="option"
                  data-active={i === 0 ? "true" : "false"}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    pickMention(a);
                  }}
                >
                  {agentDisplayName(a)}
                </button>
              ))
            )}
          </div>
        ) : null}
        <label className="z-chat-pick">
          {t("chat.agent")}
          <select
            ref={agentSelectRef}
            className="z-field"
            aria-label={t("chat.agent")}
            data-chat-agent={sessionAgent}
            value={sessionAgent}
            disabled={busy || !sessionAgent}
            onChange={(e) => void persistAgent(e.target.value)}
          >
            {agents
              .filter((a) => a.enabled !== false || a.id === sessionAgent)
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {agentDisplayName(a)}
                </option>
              ))}
          </select>
        </label>
        <label className="z-chat-pick">
          {t("chat.model")}
          <select
            className="z-field"
            aria-label={t("chat.model")}
            data-chat-model={modelValue || "follow"}
            value={modelValue}
            disabled={busy}
            onChange={(e) => void persistModel(e.target.value)}
          >
            <option value="">{t("chat.followAgent")}</option>
            {choices.map((c: ModelChoice) => (
              <option key={selectValueForChoice(c)} value={selectValueForChoice(c)}>
                {c.provider ? `${c.model} · ${c.provider}` : c.model}
              </option>
            ))}
          </select>
        </label>
        <input
          ref={inputRef}
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          value={input}
          onChange={(e) => onInput(e.target.value, e.target.selectionStart ?? e.target.value.length)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
              e.preventDefault();
              agentSelectRef.current?.focus();
              return;
            }
            if (e.key === "Escape" && mentionQ !== null) {
              e.preventDefault();
              setMentionQ(null);
              return;
            }
            if (e.key !== "Enter") return;
            if (e.nativeEvent.isComposing || e.keyCode === 229) return;
            if (mentionQ !== null && mentionHits[0]) {
              e.preventDefault();
              pickMention(mentionHits[0]);
              return;
            }
            e.preventDefault();
            void send();
          }}
          disabled={sending || savingMeta}
          placeholder={t("chat.placeholder")}
        />
        <button
          type="button"
          onClick={() => void send()}
          disabled={sending || savingMeta || !input.trim()}
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
