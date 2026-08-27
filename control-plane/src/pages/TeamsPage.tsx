import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";
import {
  teamsApi,
  type EvolutionSuggestion,
  type AgentLink,
  type Team,
  type TeamMember,
  type TeamMessage,
  type TeamTask,
} from "../api/teams";
import { useI18n } from "../i18n";
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
      } else {
        setLinks([]);
        setSuggestions([]);
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
    if (!name.trim() || !lead.trim()) return;
    try {
      const tm = await teamsApi.create({ name: name.trim(), lead_agent_id: lead.trim() });
      setName("");
      await loadTeams();
      setSelected(tm.id);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addMember() {
    if (!selected || !memberId.trim()) return;
    try {
      await teamsApi.addMember(selected, { agent_id: memberId.trim(), role: role.trim() || "member" });
      setMemberId("");
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addTask() {
    if (!selected || !taskTitle.trim()) return;
    try {
      await teamsApi.createTask(selected, { title: taskTitle.trim(), status: "todo" });
      setTaskTitle("");
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
    if (!selected || !fromAgent.trim() || !msgBody.trim()) return;
    try {
      await teamsApi.createMessage(selected, { from_agent_id: fromAgent.trim(), body: msgBody.trim() });
      setMsgBody("");
      await loadDetail(selected);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addLink() {
    if (!linkAgent.trim() || !toAgent.trim()) return;
    try {
      await teamsApi.addLink(linkAgent.trim(), { to_agent_id: toAgent.trim(), bidirectional: bidir });
      setToAgent("");
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
      <div style={{ display: "grid", gridTemplateColumns: "280px 1fr", gap: 14, alignItems: "start" }}>
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
                {members.map((m) => (
                  <div key={m.agent_id} style={{ display: "flex", padding: "10px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)" }}>
                    <span style={{ flex: 2, fontWeight: 600 }}>{m.agent_id}</span>
                    <span style={{ flex: 1, color: "var(--text-2)" }}>{m.role}</span>
                  </div>
                ))}
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
