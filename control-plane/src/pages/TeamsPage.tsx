import { useEffect, useMemo, useState } from "react";
import { api, ORCHESTRATION_MODES, type Agent } from "../api/client";
import { confirmNamed, typedConfirm } from "../api/confirm";
import { classifyPageState, inventoryBlocksMutation } from "../api/page-state";
import {
  teamsApi,
  type AgentLink,
  type EvolutionGuardrails,
  type EvolutionSuggestion,
  type Team,
  type TeamMember,
  type TeamMessage,
  type TeamTask,
} from "../api/teams";
import {
  agentLabel,
  filterLinks,
  filterTeams,
  isTeamLead,
  linkArrow,
  linkDirection,
  lockedFields,
  resolveAgentLinkLoad,
  safeEvolutionText,
  teamDisplayName,
  validateTeamDraft,
} from "../api/teams-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
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

function roleLabelKey(role: string): MsgKey {
  return role === "lead" ? "teams.role.lead" : "teams.role.member";
}

export function TeamsPage() {
  const { t } = useI18n();
  const [view, setView] = useState<"teams" | "links">("teams");
  const [teams, setTeams] = useState<Team[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState("");
  const [query, setQuery] = useState("");
  const [linkQuery, setLinkQuery] = useState("");
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [tasks, setTasks] = useState<TeamTask[]>([]);
  const [messages, setMessages] = useState<TeamMessage[]>([]);
  const [links, setLinks] = useState<AgentLink[]>([]);
  const [allLinks, setAllLinks] = useState<AgentLink[]>([]);
  const [suggestions, setSuggestions] = useState<EvolutionSuggestion[]>([]);
  const [guardrails, setGuardrails] = useState<EvolutionGuardrails>({ auto_adapt: false, min_runs: 20, locked: [] });
  const [err, setErr] = useState<unknown>(null);
  const [formErr, setFormErr] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [linksLoading, setLinksLoading] = useState(false);
  const [linksLoaded, setLinksLoaded] = useState(false);
  const [linksErr, setLinksErr] = useState<unknown>(null);
  const [agentsLoaded, setAgentsLoaded] = useState(false);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [createTeamOpen, setCreateTeamOpen] = useState(false);
  const [createLinkOpen, setCreateLinkOpen] = useState(false);

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

  const current = teams.find((x) => x.id === selected);
  const visible = useMemo(() => filterTeams(teams, query), [teams, query]);
  const locked = lockedFields(guardrails);
  const teamState = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: teams.length,
    keepStale: loaded && teams.length > 0,
  });
  const linkState = classifyPageState({
    loading: linksLoading,
    loaded: linksLoaded,
    error: linksErr,
    itemCount: allLinks.length,
    keepStale: linksLoaded && allLinks.length > 0,
  });
  const agentInvState = classifyPageState({
    loading,
    loaded: agentsLoaded,
    error: agentsErr,
    itemCount: agents.length,
    keepStale: agentsLoaded && agents.length > 0,
  });
  const createTeamBlocked = inventoryBlocksMutation(teamState.kind) || inventoryBlocksMutation(agentInvState.kind);
  const createLinkBlocked = inventoryBlocksMutation(linkState.kind) || inventoryBlocksMutation(agentInvState.kind);
  const filterEmpty = teamState.showItems && teams.length > 0 && visible.length === 0;
  const visibleLinks = useMemo(
    () => filterLinks(allLinks, linkQuery, (id) => agentLabel(agents, id)),
    [allLinks, linkQuery, agents],
  );
  const linksFilterEmpty = linkState.showItems && allLinks.length > 0 && visibleLinks.length === 0;

  function label(id: string): string {
    return agentLabel(agents, id);
  }

  function applyLinkLoad(result: ReturnType<typeof resolveAgentLinkLoad>) {
    setAllLinks(result.links);
    setLinksLoaded(result.loaded);
    setLinksErr(result.error);
  }

  async function loadAllLinks(prefetched?: { agents: Agent[]; error: unknown | null; loaded: boolean }) {
    setLinksLoading(true);
    try {
      let list = prefetched?.agents ?? [];
      let inventoryErr: unknown = prefetched?.error ?? null;
      let inventoryLoaded = prefetched?.loaded ?? false;
      if (!prefetched) {
        try {
          const a = await api.listAgents();
          list = a.agents ?? [];
          inventoryLoaded = true;
          inventoryErr = null;
          setAgents(list);
          setAgentsLoaded(true);
          setAgentsErr(null);
        } catch (e) {
          list = [];
          inventoryLoaded = false;
          inventoryErr = e;
          setAgents([]);
          setAgentsLoaded(false);
          setAgentsErr(e);
        }
      }
      let groups: Array<{ status: "fulfilled"; value: AgentLink[] } | { status: "rejected"; reason: unknown }> = [];
      if (inventoryLoaded) {
        const settled = await Promise.allSettled(list.map((a) => teamsApi.listLinks(a.id)));
        groups = settled.map((g) =>
          g.status === "fulfilled"
            ? { status: "fulfilled" as const, value: g.value.links ?? [] }
            : { status: "rejected" as const, reason: g.reason },
        );
      }
      applyLinkLoad(
        resolveAgentLinkLoad({
          agentInventoryError: inventoryErr,
          agentInventoryLoaded: inventoryLoaded,
          groups,
        }),
      );
    } catch (e) {
      setLinksErr(e);
    } finally {
      setLinksLoading(false);
    }
  }

  async function loadTeams() {
    setLoading(true);
    try {
      const [j, a] = await Promise.allSettled([teamsApi.list(), api.listAgents()]);
      if (j.status === "fulfilled") {
        setTeams(j.value.teams ?? []);
        setLoaded(true);
        setErr(null);
      } else {
        setErr(j.reason);
      }
      if (a.status === "fulfilled") {
        const nextAgents = a.value.agents ?? [];
        setAgents(nextAgents);
        setAgentsLoaded(true);
        setAgentsErr(null);
        void loadAllLinks({ agents: nextAgents, error: null, loaded: true });
      } else {
        setAgents([]);
        setAgentsLoaded(false);
        setAgentsErr(a.reason);
        void loadAllLinks({ agents: [], error: a.reason, loaded: false });
      }
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  async function loadDetail(id: string, agentForLinks?: string) {
    if (!id) return;
    setDetailLoading(true);
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
      setFormErr("");
    } catch (e) {
      setFormErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void loadTeams();
  }, []);

  useEffect(() => {
    if (!selected) {
      setMembers([]);
      setTasks([]);
      setMessages([]);
      setLinks([]);
      setSuggestions([]);
      return;
    }
    const tm = teams.find((x) => x.id === selected);
    if (tm) {
      setName(tm.name);
      setLead(tm.lead_agent_id || "");
    }
    void loadDetail(selected);
  }, [selected]);

  async function createTeam() {
    const v = validateTeamDraft(name, lead);
    if (v) {
      setFormErr(t(v));
      return;
    }
    try {
      const tm = await teamsApi.create({ name: name.trim(), lead_agent_id: lead.trim() });
      setFormErr("");
      setCreateTeamOpen(false);
      await loadTeams();
      setSelected(tm.id);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function saveTeam() {
    if (!selected) {
      await createTeam();
      return;
    }
    const v = validateTeamDraft(name, lead);
    if (v) {
      setFormErr(t(v));
      return;
    }
    try {
      await teamsApi.update(selected, { name: name.trim(), lead_agent_id: lead.trim() });
      await teamsApi.addMember(selected, { agent_id: lead.trim(), role: "lead" });
      setFormErr("");
      await loadTeams();
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function deleteTeam() {
    if (!selected || !current) return;
    const named = teamDisplayName(current);
    const typed = window.prompt(t("teams.confirmDelete", { name: named }));
    const result = typedConfirm(named, typed);
    if (result === "cancel") return;
    if (result === "mismatch") {
      setFormErr(t("teams.needConfirmName"));
      return;
    }
    try {
      await teamsApi.remove(selected);
      setSelected("");
      setName("");
      setLead("");
      setFormErr("");
      await loadTeams();
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function addMember() {
    if (!selected || !memberId.trim()) {
      setFormErr(t("teams.needMember"));
      return;
    }
    try {
      await teamsApi.addMember(selected, { agent_id: memberId.trim(), role: role.trim() || "member" });
      setMemberId("");
      setFormErr("");
      await loadTeams();
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function setMemberLead(agentId: string) {
    if (!selected || !current) return;
    try {
      await teamsApi.update(selected, { name: current.name, lead_agent_id: agentId });
      await teamsApi.addMember(selected, { agent_id: agentId, role: "lead" });
      setLead(agentId);
      setFormErr("");
      await loadTeams();
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function removeMember(m: TeamMember) {
    if (!selected || !current) return;
    if (isTeamLead(current, m.agent_id)) {
      setFormErr(t("teams.cannotRemoveLead"));
      return;
    }
    const named = label(m.agent_id);
    if (!confirmNamed(t("teams.confirmRemoveMember", { name: named }), (msg) => window.confirm(msg))) return;
    try {
      await teamsApi.removeMember(selected, m.agent_id);
      setFormErr("");
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function addTask() {
    if (!selected || !taskTitle.trim()) {
      setFormErr(t("teams.needTask"));
      return;
    }
    try {
      await teamsApi.createTask(selected, { title: taskTitle.trim(), status: "todo" });
      setTaskTitle("");
      setFormErr("");
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function moveTask(task: TeamTask) {
    const next = nextStatus(task.status);
    if (!selected || !next) return;
    try {
      await teamsApi.updateTask(selected, task.id, { status: next, title: task.title });
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function sendMsg() {
    if (!selected || !fromAgent.trim()) {
      setFormErr(t("teams.needFrom"));
      return;
    }
    if (!msgBody.trim()) {
      setFormErr(t("teams.needBody"));
      return;
    }
    try {
      await teamsApi.createMessage(selected, { from_agent_id: fromAgent.trim(), body: msgBody.trim() });
      setMsgBody("");
      setFormErr("");
      await loadDetail(selected);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function addLink() {
    if (createLinkBlocked) return;
    if (!linkAgent.trim()) {
      setFormErr(t("teams.needMember"));
      return;
    }
    if (!toAgent.trim()) {
      setFormErr(t("teams.needTo"));
      return;
    }
    try {
      await teamsApi.addLink(linkAgent.trim(), { to_agent_id: toAgent.trim(), bidirectional: bidir });
      setToAgent("");
      setFormErr("");
      setCreateLinkOpen(false);
      await loadAllLinks();
      if (selected) await loadDetail(selected, linkAgent.trim());
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function unlink(link: AgentLink) {
    const dir = linkDirection(link);
    const arrow = linkArrow(dir);
    const from = label(link.from_agent_id);
    const to = label(link.to_agent_id);
    if (!confirmNamed(t("teams.confirmUnlink", { from, arrow, to }), (msg) => window.confirm(msg))) return;
    try {
      await teamsApi.removeLink(link.from_agent_id, link.to_agent_id, dir === "bidirectional");
      setFormErr("");
      await loadAllLinks();
      if (selected) await loadDetail(selected, link.from_agent_id);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function applySug(sid: string) {
    if (!linkAgent.trim()) return;
    try {
      await teamsApi.applyEvolution(linkAgent.trim(), sid);
      await loadDetail(selected, linkAgent.trim());
    } catch (e) {
      setFormErr(formatPublicError(e));
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
      setFormErr("");
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function patchMemberMode(agentId: string, mode: string) {
    if (!ORCHESTRATION_MODES.includes(mode as (typeof ORCHESTRATION_MODES)[number])) return;
    try {
      await api.updateAgent(agentId, { orchestration_mode: mode });
      await loadTeams();
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  function startCreate() {
    if (createTeamBlocked) return;
    setSelected("");
    setName("");
    setLead("");
    setFormErr("");
    setCreateTeamOpen(true);
  }

  const colLabel: Record<(typeof COLS)[number], string> = {
    todo: t("teams.todo"),
    doing: t("teams.doing"),
    done: t("teams.done"),
  };

  const tabBtn = (id: "teams" | "links", labelText: string) => (
    <Button
      variant={view === id ? "primary" : "secondary"}
      onClick={() => {
        setView(id);
        if (id === "links") void loadAllLinks();
      }}
      data-teams-tab={id}
    >
      {labelText}
    </Button>
  );

  return (
    <PageChrome
      icon="layers"
      title={t("teams.title")}
      description={view === "links" ? t("teams.linksDesc") : t("teams.desc")}
      primary={
        view === "links" ? (
          <Button variant="primary" icon="plus" disabled={createLinkBlocked} onClick={() => setCreateLinkOpen(true)}>
            {t("teams.createLink")}
          </Button>
        ) : (
          <Button variant="primary" icon="plus" disabled={createTeamBlocked} onClick={startCreate}>
            {t("teams.create")}
          </Button>
        )
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void (view === "links" ? loadAllLinks() : selected ? loadDetail(selected) : loadTeams())}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          {tabBtn("teams", t("teams.tab.teams"))}
          {tabBtn("links", t("teams.tab.links"))}
          {view === "teams" ? (
            <input
              className="z-field"
              style={{ flex: 1, minWidth: 160 }}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("teams.search")}
              aria-label={t("teams.search")}
              autoComplete="off"
            />
          ) : (
            <input
              className="z-field"
              style={{ flex: 1, minWidth: 160 }}
              value={linkQuery}
              onChange={(e) => setLinkQuery(e.target.value)}
              placeholder={t("teams.linksSearch")}
              aria-label={t("teams.linksSearch")}
              autoComplete="off"
            />
          )}
        </>
      }
    >
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}

      {view === "links" ? (
        <div data-teams-view="links">
          <PageStatus kind={linkState.kind} errorText={linksErr ? formatPublicError(linksErr) : ""} onReload={() => void loadAllLinks()} />
          {createLinkOpen && !createLinkBlocked ? (
            <Card>
              <CardHeader icon="hook" title={t("teams.createLink")} />
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                <select className="z-field" value={linkAgent} onChange={(e) => setLinkAgent(e.target.value)} aria-label={t("teams.agentId")}>
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
                  {t("teams.bidirectional")}
                </label>
                <Button icon="plus" onClick={() => void addLink()}>
                  {t("teams.addLink")}
                </Button>
                <Button onClick={() => setCreateLinkOpen(false)}>{t("common.cancel")}</Button>
              </div>
              {agents.length === 0 && linkState.kind !== "permission" && linkState.kind !== "error" ? (
                <EmptyState>{t("teams.linksNeedAgents")}</EmptyState>
              ) : null}
            </Card>
          ) : null}
          <Card>
            <CardHeader icon="hook" title={t("teams.linksTitle")} meta={linkState.showItems ? String(visibleLinks.length) : "—"} />
            <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
              <span style={{ flex: 1.4 }}>{t("teams.col.source")}</span>
              <span style={{ flex: 1.4 }}>{t("teams.col.target")}</span>
              <span style={{ flex: 1 }}>{t("teams.col.direction")}</span>
              <span style={{ flex: 1 }}>{t("teams.col.status")}</span>
              <span style={{ flex: 1.4 }}>{t("teams.col.description")}</span>
              <span style={{ flex: 0.8 }}>{t("teams.col.actions")}</span>
            </div>
            {linkState.showItems
              ? visibleLinks.map((l, i) => {
                  const dir = linkDirection(l);
                  return (
                    <div key={`${l.from_agent_id}-${l.to_agent_id}-${i}`} style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)", flexWrap: "wrap" }}>
                      <span style={{ flex: 1.4 }}>{label(l.from_agent_id)}</span>
                      <span style={{ flex: 1.4 }}>{label(l.to_agent_id)}</span>
                      <span style={{ flex: 1 }}>
                        <Badge tone={dir === "bidirectional" ? "accent" : "neutral"}>
                          {dir === "bidirectional" ? t("teams.bidirectional") : t("teams.directed")} {linkArrow(dir)}
                        </Badge>
                      </span>
                      <span style={{ flex: 1, color: "var(--text-3)", fontSize: 11.5 }}>{t("teams.statusUnavailable")}</span>
                      <span style={{ flex: 1.4, color: "var(--text-3)", fontSize: 11.5 }}>{t("teams.descriptionUnavailable")}</span>
                      <span style={{ flex: 0.8 }}>
                        <Button onClick={() => void unlink(l)} style={{ padding: "4px 10px" }}>
                          {t("teams.unlink")}
                        </Button>
                      </span>
                    </div>
                  );
                })
              : null}
            {linkState.showEmpty ? <EmptyState>{t("teams.linksEmpty")}</EmptyState> : null}
            {linksFilterEmpty ? <EmptyState>{t("teams.emptySearch")}</EmptyState> : null}
          </Card>
        </div>
      ) : (
        <div data-teams-view="teams">
          <PageStatus kind={teamState.kind} errorText={err ? formatPublicError(err) : ""} onReload={() => void loadTeams()} />
          <div className="z-team-split">
            <Card>
              <CardHeader icon="layers" title={t("teams.list")} meta={teamState.showItems ? t("teams.meta", { n: visible.length }) : "—"} />
              {teamState.showItems
                ? visible.map((tm) => {
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
                        <div style={{ fontSize: 13, fontWeight: 600 }}>{teamDisplayName(tm)}</div>
                        <div style={{ fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>
                          {t("teams.lead")}: {tm.lead_agent_id ? label(tm.lead_agent_id) : "—"}
                        </div>
                      </button>
                    );
                  })
                : null}
              {teamState.showEmpty ? <EmptyState>{t("teams.empty")}</EmptyState> : null}
              {filterEmpty ? <EmptyState>{t("teams.emptySearch")}</EmptyState> : null}
            </Card>
            <div style={{ display: "flex", flexDirection: "column", gap: 14, minWidth: 0 }}>
              {(createTeamOpen && !createTeamBlocked) || selected ? (
                <Card>
                  <CardHeader icon="user" title={selected ? teamDisplayName(current || { id: selected, name }) : t("teams.create")} />
                  <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
                    <input className="z-field" placeholder={t("teams.name")} value={name} onChange={(e) => setName(e.target.value)} />
                    <select className="z-field" value={lead} onChange={(e) => setLead(e.target.value)} aria-label={t("teams.lead")}>
                      <option value="">{t("teams.lead")}</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.display_name || a.agent_key} ({a.id})
                        </option>
                      ))}
                    </select>
                    <Button variant="primary" icon="plus" onClick={() => void saveTeam()}>
                      {selected ? t("teams.save") : t("teams.create")}
                    </Button>
                    {selected ? <Button onClick={() => void deleteTeam()}>{t("teams.delete")}</Button> : null}
                    {!selected ? <Button onClick={() => setCreateTeamOpen(false)}>{t("common.cancel")}</Button> : null}
                  </div>
                </Card>
              ) : null}
              {!selected && !createTeamOpen && teamState.kind === "ready" ? <EmptyState>{t("teams.pick")}</EmptyState> : null}
              {selected && detailLoading ? <StatusLine kind="loading" /> : null}
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
                      <select className="z-field" value={role} onChange={(e) => setRole(e.target.value)} aria-label={t("teams.role")}>
                        <option value="member">{t("teams.role.member")}</option>
                        <option value="lead">{t("teams.role.lead")}</option>
                      </select>
                      <Button icon="plus" onClick={() => void addMember()}>
                        {t("teams.addMember")}
                      </Button>
                    </div>
                    {members.map((m) => {
                      const ag = agents.find((x) => x.id === m.agent_id);
                      const mode = ag?.orchestration_mode || "";
                      const leadRow = isTeamLead(current, m.agent_id);
                      return (
                        <div key={m.agent_id} style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)", flexWrap: "wrap" }}>
                          <span style={{ flex: 2, fontWeight: 600 }}>{label(m.agent_id)}</span>
                          <Badge tone={leadRow ? "accent" : "neutral"}>{t(roleLabelKey(leadRow ? "lead" : m.role))}</Badge>
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
                          {!leadRow ? (
                            <Button onClick={() => void setMemberLead(m.agent_id)} style={{ padding: "4px 10px" }}>
                              {t("teams.setLead")}
                            </Button>
                          ) : null}
                          <Button onClick={() => void removeMember(m)} style={{ padding: "4px 10px" }}>
                            {t("teams.removeMember")}
                          </Button>
                        </div>
                      );
                    })}
                    {members.length === 0 && !detailLoading ? <EmptyState>{t("teams.emptyMembers")}</EmptyState> : null}
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
                                <div style={{ fontSize: 11, color: "var(--text-3)", marginTop: 2 }}>{task.assignee_agent_id ? label(task.assignee_agent_id) : task.id}</div>
                              </button>
                            ))}
                          </div>
                        );
                      })}
                    </div>
                    {tasks.length === 0 && !detailLoading ? <EmptyState>{t("teams.emptyTasks")}</EmptyState> : null}
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
                        <div style={{ fontWeight: 600 }}>{label(m.from_agent_id)}</div>
                        <div style={{ color: "var(--text-2)", marginTop: 2 }}>{m.body}</div>
                      </div>
                    ))}
                    {messages.length === 0 && !detailLoading ? <EmptyState>{t("teams.emptyMessages")}</EmptyState> : null}
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
                      {locked.map((k) => (
                        <Badge key={k} tone="warning">
                          {t("teams.locked")}: {k}
                        </Badge>
                      ))}
                    </div>
                    {suggestions.map((s) => (
                      <div key={s.id} style={{ padding: "10px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", display: "flex", gap: 8, alignItems: "flex-start" }}>
                        <div style={{ flex: 1 }}>
                          <div style={{ fontWeight: 600 }}>{s.rule}</div>
                          <div style={{ color: "var(--text-2)", marginTop: 2 }}>{safeEvolutionText(s.text)}</div>
                        </div>
                        <Badge tone={s.status === "applied" ? "positive" : "warning"}>{s.status}</Badge>
                        {s.status !== "applied" ? (
                          <Button variant="primary" onClick={() => void applySug(s.id)} style={{ padding: "4px 10px" }}>
                            {t("common.apply")}
                          </Button>
                        ) : null}
                      </div>
                    ))}
                    {suggestions.length === 0 && !detailLoading ? <EmptyState>{t("teams.emptyEvolution")}</EmptyState> : null}
                  </Card>
                </>
              ) : null}
            </div>
          </div>
        </div>
      )}
    </PageChrome>
  );
}
