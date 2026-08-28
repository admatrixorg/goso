import { useEffect, useState } from "react";
import { api, ORCHESTRATION_MODES, type Agent } from "../api/client";
import {
  teamsApi,
  type EvolutionSuggestion,
  type EvolutionGuardrails,
  type AgentLink,
  type Team,
  type TeamMember,
  type TeamMessage,
  type TeamTask,
} from "../api/teams";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

const COLS = ["todo", "doing", "done"] as const;

function nextStatus(s: string): string | null {
  if (s === "todo") return "doing";
  if (s === "doing") return "done";
  return null;
}

function modeLabelKey(mode: string): MsgKey {
  if (mode === "auto") return "agents.mode.auto";
  if (mode === "explicit") return "agents.mode.explicit";
  if (mode === "manual") return "agents.mode.manual";
  return "agents.mode.unset";
}

export function TeamsPage() {
  const { t } = useI18n();
  const [teams, setTeams] = useState<Team[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState("");
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [tasks, setTasks] = useState<TeamTask[]>([]);
  const [messages, setMessages] = useState<TeamMessage[]>([]);
  const [links, setLinks] = useState<AgentLink[]>([]);
  const [suggestions, setSuggestions] = useState<EvolutionSuggestion[]>([]);
  const [guardrails, setGuardrails] = useState<EvolutionGuardrails>({ auto_adapt: false, min_runs: 20, locked: [] });
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);

  const [name, setName] = useState("");
  const [lead, setLead] = useState("");
  const [memberId, setMemberId] = useState("");
  const [role, setRole] = useState("member");
  const [taskTitle, setTaskTitle] = useState("");
  const [fromAgent, setFromAgent] = useState("");
  const [msgBody, setMsgBody] = useState("");
  const [linkAgent, setLinkAgent] = useState("");
  const [toAgent, setToAgent] = useState("");
  const [bidir, setBidir] = useState(false);

  async function loadTeams() {
    try {
      const [j, a] = await Promise.all([teamsApi.list(), api.listAgents()]);
      setTeams(j.teams ?? []);
      setAgents(a.agents ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  async function loadDetail(id: string, agentForLinks?: string) {
    if (!id) return;
    try {
      const [m, tk, msg] = await Promise.all([
        teamsApi.listMembers(id),
        teamsApi.listTasks(id),
        teamsApi.listMessages(id),
      ]);
      const mem = m.members ?? [];
      setMembers(mem);
      setTasks(tk.tasks ?? []);
      setMessages(msg.messages ?? []);
      const team = teams.find((x) => x.id === id);
      const aid = agentForLinks || linkAgent || team?.lead_agent_id || mem[0]?.agent_id || "";
      if (aid) {
        setLinkAgent(aid);
        const [lk, ev] = await Promise.all([teamsApi.listLinks(aid), teamsApi.listEvolution(aid)]);
        setLinks(lk.links ?? []);
        setSuggestions(ev.suggestions ?? []);
        if (ev.guardrails) {
          setGuardrails({
            auto_adapt: Boolean(ev.guardrails.auto_adapt),
            min_runs: ev.guardrails.min_runs > 0 ? ev.guardrails.min_runs : 20,
            locked: ev.guardrails.locked ?? [],
          });
        } else {
          setGuardrails({ auto_adapt: false, min_runs: 20, locked: [] });
        }
      } else {
        setLinks([]);
        setSuggestions([]);
        setGuardrails({ auto_adapt: false, min_runs: 20, locked: [] });
      }
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  useEffect(() => {
    void loadTeams();
  }, []);

  useEffect(() => {
    if (selected) void loadDetail(selected);
  }, [selected]);

  async function createTeam() {
    if (!name.trim()) {
      setErr(t("teams.needName"));
      return;
    }
    if (!lead.trim()) {
      setErr(t("teams.needLead"));
      return;
    }
    try {
      const tm = await teamsApi.create({ name: name.trim(), lead_agent_id: lead.trim() });
      setName("");
      setErr("");
      await loadTeams();
      setSelected(tm.id);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addMember() {
    if (!selected || !memberId.trim()) {
      setErr(t("teams.needMember"));
      return;
    }
    try {
      await teamsApi.addMember(selected, { agent_id: memberId.trim(), role: role.trim() || "member" });
      setMemberId("");
      setErr("");
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addTask() {
    if (!selected || !taskTitle.trim()) {
      setErr(t("teams.needTask"));
      return;
    }
    try {
      await teamsApi.createTask(selected, { title: taskTitle.trim(), status: "todo" });
      setTaskTitle("");
      setErr("");
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function moveTask(task: TeamTask) {
    const next = nextStatus(task.status);
    if (!selected || !next) return;
    try {
      await teamsApi.updateTask(selected, task.id, { status: next, title: task.title });
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function sendMsg() {
    if (!selected || !fromAgent.trim()) {
      setErr(t("teams.needFrom"));
      return;
    }
    if (!msgBody.trim()) {
      setErr(t("teams.needBody"));
      return;
    }
    try {
      await teamsApi.createMessage(selected, { from_agent_id: fromAgent.trim(), body: msgBody.trim() });
      setMsgBody("");
      setErr("");
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addLink() {
    if (!linkAgent.trim()) {
      setErr(t("teams.needMember"));
      return;
    }
    if (!toAgent.trim()) {
      setErr(t("teams.needTo"));
      return;
    }
    try {
      await teamsApi.addLink(linkAgent.trim(), { to_agent_id: toAgent.trim(), bidirectional: bidir });
      setToAgent("");
      setErr("");
      await loadDetail(selected, linkAgent.trim());
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function applySug(sid: string) {
    if (!linkAgent.trim()) return;
    try {
      await teamsApi.applyEvolution(linkAgent.trim(), sid);
      await loadDetail(selected, linkAgent.trim());
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function saveGuardrails(next: { auto_adapt?: boolean; min_runs?: number }) {
    if (!linkAgent.trim()) return;
    try {
      const j = await teamsApi.patchEvolution(linkAgent.trim(), next);
      if (j.guardrails) {
        setGuardrails({
          auto_adapt: Boolean(j.guardrails.auto_adapt),
          min_runs: j.guardrails.min_runs > 0 ? j.guardrails.min_runs : 20,
          locked: j.guardrails.locked ?? [],
        });
      }
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function patchMemberMode(agentId: string, mode: string) {
    if (!ORCHESTRATION_MODES.includes(mode as (typeof ORCHESTRATION_MODES)[number])) return;
    try {
      await api.updateAgent(agentId, { orchestration_mode: mode });
      await loadTeams();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  const colLabel: Record<(typeof COLS)[number], string> = {
    todo: t("teams.todo"),
    doing: t("teams.doing"),
    done: t("teams.done"),
  };

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="layers"
        title={t("teams.title")}
        description={t("teams.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void (selected ? loadDetail(selected) : loadTeams())}>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void createTeam()}>
              {t("teams.create")}
            </Button>
          </>
        }
      />
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder={t("teams.name")} value={name} onChange={(e) => setName(e.target.value)} />
        <select className="z-field" value={lead} onChange={(e) => setLead(e.target.value)} aria-label={t("teams.lead")}>
          <option value="">{t("teams.lead")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.display_name || a.agent_key} ({a.id})
            </option>
          ))}
        </select>
      </div>
      <div className="z-team-split">
        <Card>
          <CardHeader icon="layers" title={t("teams.list")} meta={t("teams.meta", { n: teams.length })} />
          {teams.map((tm) => {
            const on = selected === tm.id;
            return (
              <button
                key={tm.id}
                type="button"
                onClick={() => setSelected(tm.id)}
                style={{
                  display: "block",
                  width: "100%",
                  textAlign: "left",
                  background: on ? "var(--accent-soft)" : "transparent",
                  border: "none",
                  borderBottom: "1px solid var(--border-soft)",
                  padding: "11px 16px",
                  color: on ? "var(--accent)" : "var(--text)",
                }}
              >
                <div style={{ fontSize: 13, fontWeight: 600 }}>{tm.name}</div>
                <div style={{ fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>{tm.lead_agent_id || tm.id}</div>
              </button>
            );
          })}
          {loading ? <StatusLine kind="loading" /> : teams.length === 0 ? <EmptyState>{t("teams.empty")}</EmptyState> : null}
        </Card>
        <div style={{ display: "flex", flexDirection: "column", gap: 14, minWidth: 0 }}>
          {!selected ? <EmptyState>{t("teams.pick")}</EmptyState> : null}
          {selected ? (
            <>
              <Card>
                <CardHeader icon="user" title={t("teams.members")} meta={String(members.length)} />
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                  <select className="z-field" value={memberId} onChange={(e) => setMemberId(e.target.value)} aria-label={t("teams.agentId")}>
                    <option value="">{t("teams.agentId")}</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.display_name || a.agent_key}
                      </option>
                    ))}
                  </select>
                  <input className="z-field" placeholder={t("teams.role")} value={role} onChange={(e) => setRole(e.target.value)} />
                  <Button icon="plus" onClick={() => void addMember()}>
                    {t("teams.addMember")}
                  </Button>
                </div>
                {members.map((m) => {
                  const ag = agents.find((x) => x.id === m.agent_id);
                  const mode = ag?.orchestration_mode || "";
                  return (
                    <div key={m.agent_id} style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)" }}>
                      <span style={{ flex: 2, fontWeight: 600 }}>{m.agent_id}</span>
                      <span style={{ flex: 1, color: "var(--text-2)" }}>{m.role}</span>
                      {ag ? (
                        <select
                          className="z-field"
                          aria-label={t("teams.col.mode")}
                          value={mode}
                          onChange={(e) => void patchMemberMode(m.agent_id, e.target.value)}
                        >
                          {mode ? null : <option value="">{t("agents.mode.unset")}</option>}
                          {ORCHESTRATION_MODES.map((x) => (
                            <option key={x} value={x}>
                              {t(modeLabelKey(x))}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <span style={{ color: "var(--text-3)" }}>{t(modeLabelKey(mode))}</span>
                      )}
                    </div>
                  );
                })}
                {members.length === 0 ? <EmptyState>{t("teams.emptyMembers")}</EmptyState> : null}
              </Card>
              <Card>
                <CardHeader icon="list" title={t("teams.tasks")} />
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                  <input className="z-field" placeholder={t("teams.taskTitle")} value={taskTitle} onChange={(e) => setTaskTitle(e.target.value)} />
                  <Button icon="plus" onClick={() => void addTask()}>
                    {t("teams.addTask")}
                  </Button>
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 8, padding: "0 12px 12px" }}>
                  {COLS.map((col) => {
                    const rows = tasks.filter((x) => x.status === col);
                    return (
                      <div key={col} style={{ background: "var(--surface-1)", borderRadius: 10, padding: 8, minHeight: 80 }}>
                        <div style={{ fontSize: 11, fontWeight: 700, letterSpacing: ".4px", color: "var(--text-3)", padding: "4px 6px 8px" }}>
                          {colLabel[col]} · {rows.length}
                        </div>
                        {rows.map((task) => (
                          <button
                            key={task.id}
                            type="button"
                            onClick={() => void moveTask(task)}
                            style={{
                              display: "block",
                              width: "100%",
                              textAlign: "left",
                              background: "var(--card)",
                              border: "1px solid var(--border)",
                              borderRadius: 8,
                              padding: "8px 10px",
                              marginBottom: 6,
                              fontSize: 12.5,
                            }}
                          >
                            <div style={{ fontWeight: 600 }}>{task.title}</div>
                            <div style={{ fontSize: 11, color: "var(--text-3)", marginTop: 2 }}>{task.assignee_agent_id || task.id}</div>
                          </button>
                        ))}
                      </div>
                    );
                  })}
                </div>
                {tasks.length === 0 ? <EmptyState>{t("teams.emptyTasks")}</EmptyState> : null}
              </Card>
              <Card>
                <CardHeader icon="inbox" title={t("teams.messages")} meta={String(messages.length)} />
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                  <select className="z-field" value={fromAgent} onChange={(e) => setFromAgent(e.target.value)} aria-label={t("teams.from")}>
                    <option value="">{t("teams.from")}</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.display_name || a.agent_key}
                      </option>
                    ))}
                  </select>
                  <input className="z-field" placeholder={t("teams.body")} value={msgBody} onChange={(e) => setMsgBody(e.target.value)} style={{ flex: 1, minWidth: 180 }} />
                  <Button icon="plus" onClick={() => void sendMsg()}>
                    {t("teams.sendMessage")}
                  </Button>
                </div>
                {messages.map((m) => (
                  <div key={m.id} style={{ padding: "10px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)" }}>
                    <div style={{ fontWeight: 600 }}>{m.from_agent_id}</div>
                    <div style={{ color: "var(--text-2)", marginTop: 2 }}>{m.body}</div>
                  </div>
                ))}
                {messages.length === 0 ? <EmptyState>{t("teams.emptyMessages")}</EmptyState> : null}
              </Card>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                <Card>
                  <CardHeader icon="hook" title={t("teams.links")} />
                  <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                    <select className="z-field" value={linkAgent} onChange={(e) => { setLinkAgent(e.target.value); if (selected) void loadDetail(selected, e.target.value); }} aria-label={t("teams.agentId")}>
                      <option value="">{t("teams.agentId")}</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.display_name || a.agent_key}
                        </option>
                      ))}
                    </select>
                    <select className="z-field" value={toAgent} onChange={(e) => setToAgent(e.target.value)} aria-label={t("teams.toAgent")}>
                      <option value="">{t("teams.toAgent")}</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.display_name || a.agent_key}
                        </option>
                      ))}
                    </select>
                    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: "var(--text-2)" }}>
                      <input type="checkbox" checked={bidir} onChange={(e) => setBidir(e.target.checked)} />
                      bidirectional
                    </label>
                    <Button icon="plus" onClick={() => void addLink()}>
                      {t("teams.addLink")}
                    </Button>
                  </div>
                  {links.map((l, i) => (
                    <div key={`${l.from_agent_id}-${l.to_agent_id}-${i}`} style={{ padding: "8px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)" }}>
                      {l.from_agent_id} → {l.to_agent_id}
                    </div>
                  ))}
                  {links.length === 0 ? <EmptyState>{t("teams.emptyLinks")}</EmptyState> : null}
                </Card>
                <Card>
                  <CardHeader icon="bolt" title={t("teams.evolution")} />
                  <div style={{ padding: "10px 16px", display: "flex", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
                    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12.5 }}>
                      <input
                        type="checkbox"
                        checked={guardrails.auto_adapt}
                        onChange={(e) => void saveGuardrails({ auto_adapt: e.target.checked })}
                      />
                      {t("teams.autoAdapt")}
                    </label>
                    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12.5 }}>
                      {t("teams.minRuns")}
                      <input
                        className="z-field"
                        type="number"
                        min={1}
                        value={guardrails.min_runs}
                        onChange={(e) => {
                          const n = Number(e.target.value);
                          setGuardrails((g) => ({ ...g, min_runs: n }));
                        }}
                        onBlur={() => {
                          const n = Number(guardrails.min_runs);
                          if (!Number.isFinite(n) || n <= 0) {
                            setGuardrails((g) => ({ ...g, min_runs: 20 }));
                            void saveGuardrails({ min_runs: 20 });
                            return;
                          }
                          void saveGuardrails({ min_runs: n });
                        }}
                        aria-label={t("teams.minRuns")}
                        style={{ width: 72 }}
                      />
                    </label>
                  </div>
                  {suggestions.map((s) => (
                    <div key={s.id} style={{ padding: "10px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", display: "flex", gap: 8, alignItems: "flex-start" }}>
                      <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: 600 }}>{s.rule}</div>
                        <div style={{ color: "var(--text-2)", marginTop: 2 }}>{s.text}</div>
                      </div>
                      <Badge tone={s.status === "applied" ? "positive" : "warning"}>{s.status}</Badge>
                      {s.status !== "applied" ? (
                        <Button variant="primary" onClick={() => void applySug(s.id)} style={{ padding: "4px 10px" }}>
                          {t("common.apply")}
                        </Button>
                      ) : null}
                    </div>
                  ))}
                  {suggestions.length === 0 ? <EmptyState>{t("teams.emptyEvolution")}</EmptyState> : null}
                </Card>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}
