import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { fileChips, messageFromComposer, planPaste, type Chip } from "../api/chat-attach";
import { api, type Agent, type Message } from "../api/client";
import {
  agentDisplayName,
  applyMention,
  choiceFromSelect,
  collapseSwitchRuns,
  dayPart,
  ensureModelChoice,
  filterMentionAgents,
  highlightMentions,
  isOfflineError,
  mentionQueryAt,
  mentionStep,
  mentionStillPresent,
  modelFace,
  modelOptions,
  replyCaption,
  selectValueForChoice,
  shortLabel,
  type DayPart,
  type ModelChoice,
  type TurnAgent,
} from "../api/chat-turn";
import { formatPublicError } from "../api/public-error";
import { agentLabel, isGoneStatus, normalizePromptMode, streamReconnectDelayMs, type ChatStreamState } from "../api/sessions";
import { useI18n, type MsgKey } from "../i18n";
import { Icon } from "../ui/Icon";

function helloKey(part: DayPart): MsgKey {
  if (part === "noon") return "chat.hello.noon";
  if (part === "afternoon") return "chat.hello.afternoon";
  if (part === "evening") return "chat.hello.evening";
  return "chat.hello.morning";
}

const SUGGESTIONS: MsgKey[] = ["chat.sug.cash", "chat.sug.contract", "chat.sug.brief", "chat.sug.vault"];

let localSeq = 0;
function localId(prefix: string): string {
  localSeq += 1;
  return `${prefix}-${Date.now()}-${localSeq}`;
}

type Mention = { id: string; name: string };

export function ChatPage({
  sessionId,
  sessionLabel,
  onNew,
  onGone,
}: {
  sessionId: string;
  sessionLabel?: string;
  onNew?: () => void;
  onGone?: (id: string) => void;
}) {
  const { t } = useI18n();
  const named = sessionLabel?.trim() || "";
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
  const [mentionIdx, setMentionIdx] = useState(0);
  const [menu, setMenu] = useState<null | { kind: "agent" | "model"; bottom: number; left: number; maxHeight: number }>(null);
  const [chips, setChips] = useState<Chip[]>([]);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [dragOver, setDragOver] = useState(false);
  const genRef = useRef(0);
  const fileRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const dragDepth = useRef(0);
  const sessionRef = useRef(sessionId);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const threadRef = useRef<HTMLDivElement>(null);
  sessionRef.current = sessionId;

  function stillCurrent(forSession: string, gen: number): boolean {
    return gen === genRef.current && sessionRef.current === forSession;
  }

  function shownError(e: unknown): string {
    const msg = formatPublicError(e);
    return isOfflineError(msg) ? t("chat.offline") : msg;
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
        setErr(shownError(reason));
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
      setErr(shownError(e));
      if (isGoneStatus(e)) onGone?.(forSession);
    } finally {
      if (stillCurrent(forSession, gen)) setLoading(false);
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
      setErr(shownError(e));
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
      setErr(shownError(e));
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
      setMentionIdx(0);
      setMenu(null);
      setInput("");
      clearChips();
      abortRef.current?.abort();
      return;
    }
    setLoading(true);
    setMsgs([]);
    setErr("");
    setSending(false);
    setStream("idle");
    setMention(null);
    setMentionQ(null);
    setMentionIdx(0);
    setMenu(null);
    setInput("");
    clearChips();
    abortRef.current?.abort();
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
    setMentionIdx(0);
    setMention((cur) => (cur && mentionStillPresent(value, cur.name) ? cur : null));
    const node = inputRef.current;
    if (node) {
      node.style.height = "auto";
      node.style.height = `${Math.min(220, node.scrollHeight)}px`;
    }
  }

  function openMenu(kind: "agent" | "model", el: HTMLElement) {
    const r = el.getBoundingClientRect();
    const width = 280;
    setMentionQ(null);
    setMenu((cur) => {
      if (cur?.kind === kind) return null;
      return {
        kind,
        bottom: Math.max(8, window.innerHeight - r.top + 6),
        left: Math.max(8, Math.min(r.right - width, window.innerWidth - width - 8)),
        maxHeight: Math.max(140, r.top - 16),
      };
    });
  }

  useEffect(() => {
    if (!menu) return;
    const close = (e: PointerEvent) => {
      const t = e.target as Node | null;
      if (t && (t as HTMLElement).closest?.(".z-gs-pop, .z-gs-pick")) return;
      setMenu(null);
    };
    window.addEventListener("pointerdown", close);
    return () => window.removeEventListener("pointerdown", close);
  }, [menu]);

  useEffect(() => {
    const el = threadRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [msgs, sending]);

  function clearChips() {
    setPreviews((prev) => {
      for (const url of Object.values(prev)) URL.revokeObjectURL(url);
      return {};
    });
    setChips([]);
  }

  function takeFiles(list: File[]) {
    const got = fileChips(list, () => localId("chip"));
    const images = list.filter((file) => file.size <= 200 * 1024 * 1024 && file.type.startsWith("image/"));
    const urls: Record<string, string> = {};
    for (const chip of got.chips) {
      if (!chip.image) continue;
      const file = images.shift();
      if (file) urls[chip.id] = URL.createObjectURL(file);
    }
    if (Object.keys(urls).length) setPreviews((prev) => ({ ...prev, ...urls }));
    if (got.chips.length) setChips((prev) => [...prev, ...got.chips]);
    if (got.errors.length) setErr(got.errors.map((name) => t("chat.attachTooBig", { name })).join(" "));
  }

  function dropChip(id: string) {
    setChips((prev) => prev.filter((chip) => chip.id !== id));
    setPreviews((prev) => {
      const url = prev[id];
      if (url) URL.revokeObjectURL(url);
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }

  function stop() {
    abortRef.current?.abort();
  }

  async function send() {
    const text = messageFromComposer(input, chips, (name) => t("chat.attachUnread", { name }));
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
    clearChips();
    if (inputRef.current) inputRef.current.style.height = "auto";
    setMention(null);
    setMentionQ(null);
    setErr("");
    const ctrl = new AbortController();
    abortRef.current = ctrl;
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
        ctrl.signal,
      );
      if (!stillCurrent(forSession, gen)) return;
      setStream("idle");
      await load(forSession, gen);
    } catch (e) {
      if (!stillCurrent(forSession, gen)) return;
      if (e instanceof Error && e.name === "AbortError") {
        setMsgs((m) => m.filter((x) => x.id !== asst.id || x.content.trim()));
        setStream("idle");
        setErr("");
        return;
      }
      const msg = shownError(e);
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

  const names = agents.map(agentDisplayName);
  const choices = ensureModelChoice(modelOptions(agents), providerOverride, modelOverride);
  const mentionHits = mentionQ === null ? [] : filterMentionAgents(agents, mentionQ);
  const activeMention = Math.min(mentionIdx, Math.max(0, mentionHits.length - 1));
  const busy = sending || savingMeta;
  const visible = collapseSwitchRuns(msgs.filter((m) => m.role !== "tool" && !m.id.startsWith("local-err-")));
  const empty = Boolean(sessionId) && !loading && visible.length === 0 && !err;
  const showHello = !sessionId || empty;
  const sessionAgentRow = agents.find((a) => a.id === sessionAgent);
  const agentFace = sessionAgentRow ? agentDisplayName(sessionAgentRow) : t("chat.agent");
  const modelLabel = modelFace(sessionAgentRow?.model, modelOverride, t("chat.followAgent"));
  const firstUser = msgs.find((m) => m.role === "user")?.content.trim() || "";
  const title = (firstUser || (visible.length ? named : "")).slice(0, 60);
  const lastAi = visible.map((m) => m.role).lastIndexOf("assistant");

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (mentionQ !== null) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setMentionIdx((i) => mentionStep(i, mentionHits.length, 1));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setMentionIdx((i) => mentionStep(i, mentionHits.length, -1));
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionQ(null);
        return;
      }
      if ((e.key === "Enter" || e.key === "Tab") && mentionHits[activeMention]) {
        e.preventDefault();
        pickMention(mentionHits[activeMention]);
        return;
      }
    }
    if (e.key === "Escape" && menu) {
      e.preventDefault();
      setMenu(null);
      return;
    }
    if (e.key !== "Enter" || e.shiftKey) return;
    if (e.nativeEvent.isComposing || e.keyCode === 229) return;
    e.preventDefault();
    void send();
  }

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== "u" || !sessionId) return;
      e.preventDefault();
      fileRef.current?.click();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sessionId]);

  return (
    <div
      className={showHello ? "z-gs is-empty" : "z-gs"}
      data-chat-state={sessionId ? "ready" : "no-session"}
      data-chat-stream={stream}
      onDragEnter={(e) => {
        if (![...e.dataTransfer.types].includes("Files")) return;
        e.preventDefault();
        dragDepth.current += 1;
        setDragOver(true);
      }}
      onDragOver={(e) => {
        if ([...e.dataTransfer.types].includes("Files")) e.preventDefault();
      }}
      onDragLeave={() => {
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (dragDepth.current === 0) setDragOver(false);
      }}
      onDrop={(e) => {
        if (![...e.dataTransfer.types].includes("Files")) return;
        e.preventDefault();
        dragDepth.current = 0;
        setDragOver(false);
        takeFiles(Array.from(e.dataTransfer.files));
      }}
    >
      {dragOver ? <div className="z-gs-drop">{t("chat.drop")}</div> : null}
      <div className="z-gs-bar">
        <span className="z-gs-ttl">{title}</span>
      </div>
      <div className="z-gs-thread" ref={threadRef}>
        <div className="z-gs-col">
          {visible.map((m, i) => {
            if (m.role === "event") {
              return (
                <div key={m.id} className="z-gs-sys" data-chat-switch={m.agent_id || ""}>
                  {t("chat.switched", { name: agentLabel(agents, m.agent_id || "") })}
                </div>
              );
            }
            if (m.role === "user") {
              const spans = highlightMentions(m.content, names);
              return (
                <div key={m.id} className="z-gs-u">
                  {spans.map((span, n) =>
                    span.mention ? (
                      <span key={n} className="z-gs-mention">
                        {span.text}
                      </span>
                    ) : (
                      <span key={n}>{span.text}</span>
                    ),
                  )}
                </div>
              );
            }
            const caption = replyCaption(m.agent_id ? agentLabel(agents, m.agent_id) : "", m.model || "");
            const pending = m.role === "assistant" && !m.content && sending;
            return (
              <div key={m.id} className={i === lastAi ? "z-gs-ai is-last" : "z-gs-ai"}>
                {caption ? (
                  <div className="z-gs-who" data-reply-caption={caption}>
                    <b>{m.agent_id ? agentLabel(agents, m.agent_id) : ""}</b>
                    {m.model ? <span>· {m.model}</span> : null}
                  </div>
                ) : null}
                {pending ? (
                  <div className="z-gs-dots" aria-hidden="true">
                    <i />
                    <i />
                    <i />
                  </div>
                ) : (
                  <div className="z-gs-body">{m.content}</div>
                )}
                {m.role === "assistant" && m.content ? (
                  <div className="z-gs-acts">
                    <button
                      type="button"
                      aria-label={t("chat.copy")}
                      onClick={() => {
                        void navigator.clipboard?.writeText(m.content).catch(() => undefined);
                      }}
                    >
                      <Icon name="doc" size={16} />
                    </button>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>
      <div className="z-gs-dock">
        {showHello ? <div className="z-gs-hello">{t(helloKey(dayPart(new Date())))}</div> : null}
        <div className="z-gs-comp">
          {mentionQ !== null ? (
            <div className="z-gs-pop z-gs-mention-pop" role="listbox" aria-label={t("chat.mentionList")} data-chat-mention>
              <h6>{t("chat.mentionList")}</h6>
              {mentionHits.length === 0 ? (
                <div style={{ padding: "7px 9px", color: "var(--gs-faint)" }}>{t("chat.mentionEmpty")}</div>
              ) : (
                mentionHits.map((a, i) => (
                  <button
                    key={a.id}
                    type="button"
                    role="option"
                    data-active={i === activeMention ? "true" : "false"}
                    onMouseDown={(e) => {
                      e.preventDefault();
                      pickMention(a);
                    }}
                  >
                    <span className="nm">
                      {agentDisplayName(a)}
                      {a.model ? <small>{a.model}</small> : null}
                    </span>
                  </button>
                ))
              )}
            </div>
          ) : null}
          {chips.length ? (
            <div className="z-gs-atts">
              {chips.map((chip) => {
                const preview = chip.kind === "file" ? previews[chip.id] : "";
                return (
                  <button
                    key={chip.id}
                    type="button"
                    className={preview ? "z-gs-chip" : "z-gs-chip is-doc"}
                    onClick={() => dropChip(chip.id)}
                    aria-label={t("chat.attachRemove", { name: chip.name })}
                  >
                    {preview ? <img src={preview} alt="" /> : null}
                    {preview ? null : (
                      <>
                        <b>{chip.name}</b>
                        <small>{chip.typeLabel}</small>
                      </>
                    )}
                    <span className="x" aria-hidden="true">
                      ×
                    </span>
                  </button>
                );
              })}
            </div>
          ) : null}
          <textarea
            ref={inputRef}
            rows={1}
            aria-label={t("chat.placeholder", { name: agentFace })}
            value={input}
            disabled={!sessionId || sending || savingMeta}
            placeholder={sessionId ? t("chat.placeholder", { name: shortLabel(agentFace, 28) }) : t("chat.emptySession")}
            onChange={(e) => onInput(e.target.value, e.target.selectionStart ?? e.target.value.length)}
            onKeyDown={onKeyDown}
            onPaste={(e) => {
              const files = Array.from(e.clipboardData.files);
              if (files.length) {
                e.preventDefault();
                takeFiles(files);
                return;
              }
              const plan = planPaste(
                { files: [], html: e.clipboardData.getData("text/html"), text: e.clipboardData.getData("text/plain") },
                () => localId("chip"),
              );
              if (!plan.handled) return;
              e.preventDefault();
              if (plan.chips.length) setChips((prev) => [...prev, ...plan.chips]);
            }}
          />
          <div className="z-gs-row">
            <button
              type="button"
              className="z-gs-ic"
              aria-label={t("chat.attach")}
              title={t("chat.attachHint")}
              disabled={!sessionId || sending || savingMeta}
              onClick={() => fileRef.current?.click()}
            >
              <Icon name="plus" size={16} />
            </button>
            <input
              ref={fileRef}
              type="file"
              multiple
              hidden
              aria-hidden="true"
              onChange={(e) => {
                takeFiles(Array.from(e.target.files ?? []));
                e.target.value = "";
              }}
            />
            <span className="z-gs-grow" />
            <button
              type="button"
              className="z-gs-pick"
              aria-label={t("chat.agent")}
              aria-expanded={menu?.kind === "agent"}
              data-chat-agent={sessionAgent}
              disabled={!sessionId || busy || !sessionAgent}
              onClick={(e) => openMenu("agent", e.currentTarget)}
            >
              <span>{agentFace}</span>
              <Caret />
            </button>
            <button
              type="button"
              className="z-gs-pick"
              aria-label={t("chat.model")}
              aria-expanded={menu?.kind === "model"}
              data-chat-model={modelOverride ? selectValueForChoice({ provider: providerOverride, model: modelOverride }) : "follow"}
              disabled={!sessionId || busy}
              onClick={(e) => openMenu("model", e.currentTarget)}
            >
              <span>{modelLabel}</span>
              <Caret />
            </button>
            <button
              type="button"
              className="z-gs-send"
              aria-label={sending ? t("chat.stop") : t("chat.send")}
              disabled={!sessionId || savingMeta || (!sending && !input.trim() && chips.length === 0)}
              onClick={() => (sending ? stop() : void send())}
            >
              {sending ? <StopMark /> : <Icon name="arrow-up" size={16} />}
            </button>
          </div>
        </div>
        {err ? <p className="z-gs-err">{err}</p> : null}
        {showHello ? (
          <div className="z-gs-sugs">
            {SUGGESTIONS.map((key) => (
              <button
                key={key}
                type="button"
                onClick={() => {
                    if (!sessionId) {
                      onNew?.();
                      return;
                    }
                  const text = t(key);
                  setInput(text);
                  inputRef.current?.focus();
                }}
              >
                {t(key)}
              </button>
            ))}
          </div>
        ) : null}
      </div>
      {menu ? (
        <div className="z-gs-pop" role="listbox" style={{ bottom: menu.bottom, left: menu.left, maxHeight: menu.maxHeight }}>
          {menu.kind === "agent" ? (
            <>
              <h6>{t("chat.agent")}</h6>
              {agents
                .filter((a) => a.enabled !== false || a.id === sessionAgent)
                .map((a) => (
                  <button
                    key={a.id}
                    type="button"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      setMenu(null);
                      void persistAgent(a.id);
                    }}
                  >
                    <span className="ck">{a.id === sessionAgent ? "✓" : ""}</span>
                    <span className="nm">{agentDisplayName(a)}</span>
                  </button>
                ))}
            </>
          ) : (
            <>
              <button
                type="button"
                onMouseDown={(e) => {
                  e.preventDefault();
                  setMenu(null);
                  void persistModel("");
                }}
              >
                <span className="ck">{modelOverride ? "" : "✓"}</span>
                <span className="nm">
                  {t("chat.followAgent")}
                  {sessionAgentRow?.model ? <small>{sessionAgentRow.model}</small> : null}
                </span>
              </button>
              <hr />
              {choices.map((c: ModelChoice) => {
                const value = selectValueForChoice(c);
                const on = Boolean(modelOverride) && c.model === modelOverride && c.provider === providerOverride;
                return (
                  <button
                    key={value}
                    type="button"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      setMenu(null);
                      void persistModel(value);
                    }}
                  >
                    <span className="ck">{on ? "✓" : ""}</span>
                    <span className="nm">{c.model}</span>
                  </button>
                );
              })}
            </>
          )}
        </div>
      ) : null}
    </div>
  );
}

function StopMark() {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
      <rect x="1" y="1" width="10" height="10" rx="2" fill="currentColor" stroke="none" />
    </svg>
  );
}

function Caret() {
  return (
    <svg className="z-gs-chev" viewBox="0 0 24 24" aria-hidden="true">
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}
