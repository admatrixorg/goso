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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";

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
      variant={view === id ? "default" : "outline"}
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
          <Button variant="default" icon="plus" disabled={createLinkBlocked} onClick={() => setCreateLinkOpen(true)}>
            {t("teams.createLink")}
          </Button>
        ) : (
          <Button variant="default" icon="plus" disabled={createTeamBlocked} onClick={startCreate}>
            {t("teams.create")}
          </Button>
        )
      }
      refresh={
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void (view === "links" ? loadAllLinks() : selected ? loadDetail(selected) : loadTeams())}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          {tabBtn("teams", t("teams.tab.teams"))}
          {tabBtn("links", t("teams.tab.links"))}
          {view === "teams" ? (
            <SearchInput value={query} onChange={setQuery} placeholder={t("teams.search")} aria-label={t("teams.search")} />
          ) : (
            <SearchInput value={linkQuery} onChange={setLinkQuery} placeholder={t("teams.linksSearch")} aria-label={t("teams.linksSearch")} />
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
              <div className="flex flex-col gap-3 p-4">
                <label className={labelClass}>
                  {t("teams.agentId")}
                  <select className={selectClass + " mt-1 w-full"} value={linkAgent} onChange={(e) => setLinkAgent(e.target.value)} aria-label={t("teams.agentId")}>
                    <option value="">{t("teams.agentId")}</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.display_name || a.agent_key}
                      </option>
                    ))}
                  </select>
                </label>
                <label className={labelClass}>
                  {t("teams.toAgent")}
                  <select className={selectClass + " mt-1 w-full"} value={toAgent} onChange={(e) => setToAgent(e.target.value)} aria-label={t("teams.toAgent")}>
                    <option value="">{t("teams.toAgent")}</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.display_name || a.agent_key}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex items-center gap-1.5 text-sm">
                  <input type="checkbox" checked={bidir} onChange={(e) => setBidir(e.target.checked)} />
                  {t("teams.bidirectional")}
                </label>
                <div className="flex flex-wrap gap-2">
                  <Button variant="default" icon="plus" onClick={() => void addLink()}>
                    {t("teams.addLink")}
                  </Button>
                  <Button variant="outline" onClick={() => setCreateLinkOpen(false)}>
                    {t("common.cancel")}
                  </Button>
                </div>
              </div>
              {agents.length === 0 && linkState.kind !== "permission" && linkState.kind !== "error" ? (
                <EmptyState icon="friends" title={t("teams.linksNeedAgents")} />
              ) : null}
            </Card>
          ) : null}
          {linkState.showEmpty ? <EmptyState icon="friends" title={t("teams.linksEmpty")} /> : null}
          {linksFilterEmpty ? <EmptyState icon="search" title={t("teams.emptySearch")} /> : null}
          {linkState.showItems ? (
            <>
            <p className="m-0 text-xs text-muted-foreground">{t("teams.linksTitle")} · {visibleLinks.length}</p>
            <DataTable minWidth="800px">
              <THead>
                <Th>{t("teams.col.source")}</Th>
                <Th>{t("teams.col.target")}</Th>
                <Th>{t("teams.col.direction")}</Th>
                <Th>{t("teams.col.status")}</Th>
                <Th>{t("teams.col.description")}</Th>
                <Th className="text-right">{t("teams.col.actions")}</Th>
              </THead>
              <tbody>
                {visibleLinks.map((l, i) => {
                  const dir = linkDirection(l);
                  return (
                    <Tr key={`${l.from_agent_id}-${l.to_agent_id}-${i}`}>
                      <Td className="font-semibold">{label(l.from_agent_id)}</Td>
                      <Td>{label(l.to_agent_id)}</Td>
                      <Td>
                        <StatusBadge
                          status={dir === "bidirectional" ? "info" : "default"}
                          label={`${dir === "bidirectional" ? t("teams.bidirectional") : t("teams.directed")} ${linkArrow(dir)}`}
                        />
                      </Td>
                      <Td className="text-muted-foreground">{t("teams.statusUnavailable")}</Td>
                      <Td className="text-muted-foreground">{t("teams.descriptionUnavailable")}</Td>
                      <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                        <Button variant="ghost" size="sm" className="text-destructive" onClick={() => void unlink(l)}>
                          {t("teams.unlink")}
                        </Button>
                      </Td>
                    </Tr>
                  );
                })}
              </tbody>
            </DataTable>
            </>
          ) : null}
        </div>
      ) : (
        <div data-teams-view="teams">
          <PageStatus kind={teamState.kind} errorText={err ? formatPublicError(err) : ""} onReload={() => void loadTeams()} />
          <div className="z-team-split">
            <div>
              {teamState.showEmpty ? <EmptyState icon="friends" title={t("teams.empty")} /> : null}
              {filterEmpty ? <EmptyState icon="search" title={t("teams.emptySearch")} /> : null}
              <p className="m-0 px-1 text-xs text-muted-foreground">{teamState.showItems ? t("teams.meta", { n: visible.length }) : "—"}</p>
              {teamState.showItems ? (
                <DataTable minWidth="240px" className="mt-0">
                  <THead>
                    <Th>{t("teams.list")}</Th>
                    <Th>{t("teams.lead")}</Th>
                  </THead>
                  <tbody>
                    {visible.map((tm) => (
                      <Tr key={tm.id} selected={selected === tm.id} onClick={() => setSelected(tm.id)}>
                        <Td className="font-semibold">{teamDisplayName(tm)}</Td>
                        <Td className="text-muted-foreground">{tm.lead_agent_id ? label(tm.lead_agent_id) : "—"}</Td>
                      </Tr>
                    ))}
                  </tbody>
                </DataTable>
              ) : null}
            </div>
            <div className="flex min-w-0 flex-col gap-3.5">
              {(createTeamOpen && !createTeamBlocked) || selected ? (
                <Card>
                  <CardHeader icon="user" title={selected ? teamDisplayName(current || { id: selected, name }) : t("teams.create")} />
                  <div className="flex flex-col gap-3 p-4">
                    <label className={labelClass}>
                      {t("teams.name")}
                      <input className={fieldClass + " mt-1"} placeholder={t("teams.name")} value={name} onChange={(e) => setName(e.target.value)} />
                    </label>
                    <label className={labelClass}>
                      {t("teams.lead")}
                      <select className={selectClass + " mt-1 w-full"} value={lead} onChange={(e) => setLead(e.target.value)} aria-label={t("teams.lead")}>
                        <option value="">{t("teams.lead")}</option>
                        {agents.map((a) => (
                          <option key={a.id} value={a.id}>
                            {a.display_name || a.agent_key} ({a.id})
                          </option>
                        ))}
                      </select>
                    </label>
                    <div className="flex flex-wrap gap-2">
                      <Button variant="default" icon="plus" onClick={() => void saveTeam()}>
                        {selected ? t("teams.save") : t("teams.create")}
                      </Button>
                      {selected ? (
                        <Button variant="ghost" className="text-destructive" onClick={() => void deleteTeam()}>
                          {t("teams.delete")}
                        </Button>
                      ) : null}
                      {!selected ? (
                        <Button variant="outline" onClick={() => setCreateTeamOpen(false)}>
                          {t("common.cancel")}
                        </Button>
                      ) : null}
                    </div>
                  </div>
                </Card>
              ) : null}
              {!selected && !createTeamOpen && teamState.kind === "ready" ? <EmptyState icon="friends" title={t("teams.pick")} /> : null}
              {selected && detailLoading ? <StatusLine kind="loading" /> : null}
              {selected ? (
                <>
                  <Card>
                    <CardHeader icon="user" title={t("teams.members")} meta={String(members.length)} />
                    <div className="flex flex-col gap-3 p-4">
                      <div className="flex flex-wrap items-end gap-2">
                        <label className={labelClass}>
                          {t("teams.agentId")}
                          <select className={selectClass + " mt-1"} value={memberId} onChange={(e) => setMemberId(e.target.value)} aria-label={t("teams.agentId")}>
                            <option value="">{t("teams.agentId")}</option>
                            {agents.map((a) => (
                              <option key={a.id} value={a.id}>
                                {a.display_name || a.agent_key}
                              </option>
                            ))}
                          </select>
                        </label>
                        <label className={labelClass}>
                          {t("teams.role")}
                          <select className={selectClass + " mt-1"} value={role} onChange={(e) => setRole(e.target.value)} aria-label={t("teams.role")}>
                            <option value="member">{t("teams.role.member")}</option>
                            <option value="lead">{t("teams.role.lead")}</option>
                          </select>
                        </label>
                        <Button icon="plus" variant="outline" onClick={() => void addMember()}>
                          {t("teams.addMember")}
                        </Button>
                      </div>
                      {members.length === 0 && !detailLoading ? <EmptyState icon="friends" title={t("teams.emptyMembers")} /> : null}
                      {members.length ? (
                        <DataTable minWidth="520px" className="mt-0">
                          <THead>
                            <Th>{t("teams.members")}</Th>
                            <Th>{t("teams.role")}</Th>
                            <Th>{t("teams.col.mode")}</Th>
                            <Th className="text-right" />
                          </THead>
                          <tbody>
                            {members.map((m) => {
                              const ag = agents.find((x) => x.id === m.agent_id);
                              const mode = ag?.orchestration_mode || "";
                              const leadRow = isTeamLead(current, m.agent_id);
                              return (
                                <Tr key={m.agent_id}>
                                  <Td className="font-semibold">{label(m.agent_id)}</Td>
                                  <Td>
                                    <StatusBadge status={leadRow ? "info" : "default"} label={t(roleLabelKey(leadRow ? "lead" : m.role))} />
                                  </Td>
                                  <Td onClick={(e) => e.stopPropagation()}>
                                    {ag ? (
                                      <select
                                        className={selectClass}
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
                                      <span className="text-muted-foreground">{t(modeLabelKey(mode))}</span>
                                    )}
                                  </Td>
                                  <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                                    <div className="flex flex-wrap justify-end gap-2">
                                      {!leadRow ? (
                                        <Button variant="outline" size="sm" onClick={() => void setMemberLead(m.agent_id)}>
                                          {t("teams.setLead")}
                                        </Button>
                                      ) : null}
                                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => void removeMember(m)}>
                                        {t("teams.removeMember")}
                                      </Button>
                                    </div>
                                  </Td>
                                </Tr>
                              );
                            })}
                          </tbody>
                        </DataTable>
                      ) : null}
                    </div>
                  </Card>
                  <Card>
                    <CardHeader icon="list" title={t("teams.tasks")} />
                    <div className="flex flex-col gap-3 p-4">
                      <div className="flex flex-wrap gap-2">
                        <input className={fieldClass} placeholder={t("teams.taskTitle")} value={taskTitle} onChange={(e) => setTaskTitle(e.target.value)} />
                        <Button icon="plus" variant="outline" onClick={() => void addTask()}>
                          {t("teams.addTask")}
                        </Button>
                      </div>
                      <div className="grid grid-cols-3 gap-2">
                        {COLS.map((col) => {
                          const rows = tasks.filter((x) => x.status === col);
                          return (
                            <div key={col} className="min-h-20 rounded-md bg-muted/40 p-2">
                              <div className="px-1.5 pb-2 pt-1 text-xs font-semibold tracking-wide text-muted-foreground">
                                {colLabel[col]} · {rows.length}
                              </div>
                              {rows.map((task) => (
                                <button
                                  key={task.id}
                                  type="button"
                                  onClick={() => void moveTask(task)}
                                  className="mb-1.5 block w-full rounded-md border bg-card px-2.5 py-2 text-left text-sm"
                                >
                                  <div className="font-semibold">{task.title}</div>
                                  <div className="mt-0.5 text-xs text-muted-foreground">{task.assignee_agent_id ? label(task.assignee_agent_id) : task.id}</div>
                                </button>
                              ))}
                            </div>
                          );
                        })}
                      </div>
                      {tasks.length === 0 && !detailLoading ? <EmptyState icon="friends" title={t("teams.emptyTasks")} /> : null}
                    </div>
                  </Card>
                  <Card>
                    <CardHeader icon="inbox" title={t("teams.messages")} meta={String(messages.length)} />
                    <div className="flex flex-col gap-3 p-4">
                      <div className="flex flex-wrap gap-2">
                        <select className={selectClass} value={fromAgent} onChange={(e) => setFromAgent(e.target.value)} aria-label={t("teams.from")}>
                          <option value="">{t("teams.from")}</option>
                          {agents.map((a) => (
                            <option key={a.id} value={a.id}>
                              {a.display_name || a.agent_key}
                            </option>
                          ))}
                        </select>
                        <input className={fieldClass + " min-w-[180px] flex-1"} placeholder={t("teams.body")} value={msgBody} onChange={(e) => setMsgBody(e.target.value)} />
                        <Button icon="plus" variant="outline" onClick={() => void sendMsg()}>
                          {t("teams.sendMessage")}
                        </Button>
                      </div>
                      {messages.map((m) => (
                        <div key={m.id} className="border-t py-2.5 text-sm">
                          <div className="font-semibold">{label(m.from_agent_id)}</div>
                          <div className="mt-0.5 text-muted-foreground">{m.body}</div>
                        </div>
                      ))}
                      {messages.length === 0 && !detailLoading ? <EmptyState icon="friends" title={t("teams.emptyMessages")} /> : null}
                    </div>
                  </Card>
                  <Card>
                    <CardHeader icon="bolt" title={t("teams.evolution")} />
                    <div className="flex flex-col gap-3 p-4">
                      <div className="flex flex-wrap items-center gap-3">
                        <label className="flex items-center gap-1.5 text-sm">
                          <input
                            type="checkbox"
                            checked={guardrails.auto_adapt}
                            onChange={(e) => void saveGuardrails({ auto_adapt: e.target.checked })}
                          />
                          {t("teams.autoAdapt")}
                        </label>
                        <label className={`flex items-center gap-1.5 ${labelClass}`}>
                          {t("teams.minRuns")}
                          <input
                            className={fieldClass + " w-[72px]"}
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
                          />
                        </label>
                        {locked.map((k) => (
                          <StatusBadge key={k} status="warning" label={`${t("teams.locked")}: ${k}`} />
                        ))}
                      </div>
                      {suggestions.map((s) => (
                        <div key={s.id} className="flex items-start gap-2 border-b py-2.5 text-sm">
                          <div className="flex-1">
                            <div className="font-semibold">{s.rule}</div>
                            <div className="mt-0.5 text-muted-foreground">{safeEvolutionText(s.text)}</div>
                          </div>
                          <StatusBadge status={s.status === "applied" ? "success" : "warning"} label={s.status} />
                          {s.status !== "applied" ? (
                            <Button variant="default" size="sm" onClick={() => void applySug(s.id)}>
                              {t("common.apply")}
                            </Button>
                          ) : null}
                        </div>
                      ))}
                      {suggestions.length === 0 && !detailLoading ? <EmptyState icon="friends" title={t("teams.emptyEvolution")} /> : null}
                    </div>
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
