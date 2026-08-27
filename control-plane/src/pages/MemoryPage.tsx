import { useEffect, useState } from "react";
import { api, type Session } from "../api/client";
import { memoryApi, type MemoryHit, type MemoryNote } from "../api/memory";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function MemoryPage() {
  const { t } = useI18n();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionId, setSessionId] = useState("");
  const [notes, setNotes] = useState<MemoryNote[]>([]);
  const [hits, setHits] = useState<MemoryHit[] | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [notesLoading, setNotesLoading] = useState(false);
  const [q, setQ] = useState("");
  const [body, setBody] = useState("");

  async function loadSessions() {
    try {
      const j = await api.listSessions();
      setSessions(j.sessions ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  async function loadNotes(sid: string) {
    if (!sid) {
      setNotes([]);
      setNotesLoading(false);
      return;
    }
    setNotesLoading(true);
    try {
      const j = await memoryApi.list(sid);
      setNotes(j.memories ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setNotesLoading(false);
    }
  }

  useEffect(() => {
    void loadSessions();
  }, []);

  useEffect(() => {
    void loadNotes(sessionId);
  }, [sessionId]);

  async function postNote() {
    if (!sessionId || !body.trim()) return;
    try {
      await memoryApi.create({ session_id: sessionId, body: body.trim(), kind: "episodic" });
      setBody("");
      await loadNotes(sessionId);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function search() {
    const query = q.trim();
    if (!query) {
      setHits(null);
      return;
    }
    try {
      setHits(await memoryApi.search(query));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="inbox"
        title={t("memory.title")}
        description={t("memory.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void (sessionId ? loadNotes(sessionId) : loadSessions())}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <select className="z-field" value={sessionId} onChange={(e) => setSessionId(e.target.value)} aria-label={t("memory.session")}>
          <option value="">{t("memory.pickSession")}</option>
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              {s.label || s.id}
            </option>
          ))}
        </select>
        <input className="z-field" placeholder={t("memory.note")} value={body} onChange={(e) => setBody(e.target.value)} style={{ flex: 1, minWidth: 180 }} />
        <Button variant="primary" icon="plus" onClick={() => void postNote()}>
          {t("memory.post")}
        </Button>
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder={t("memory.search")} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void search(); }} />
        <Button icon="search" onClick={() => void search()}>
          {t("common.search")}
        </Button>
      </div>
      {hits ? (
        <Card>
          <CardHeader icon="search" title={t("memory.hits")} meta={String(hits.length)} />
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1 }}>{t("memory.col.kind")}</span>
            <span style={{ flex: 2 }}>{t("memory.col.session")}</span>
            <span style={{ flex: 3 }}>{t("memory.col.body")}</span>
          </div>
          {hits.map((h) => (
            <div key={h.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
              <span style={{ flex: 1 }}>{h.kind}</span>
              <span style={{ flex: 2, color: "var(--text-3)" }}>{h.session_id}</span>
              <span style={{ flex: 3, color: "var(--text-2)" }}>{h.snippet}</span>
            </div>
          ))}
          {hits.length === 0 ? <EmptyState>{t("memory.emptyHits")}</EmptyState> : null}
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="inbox" title={t("memory.list")} meta={t("memory.meta", { n: notes.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1 }}>{t("memory.col.kind")}</span>
          <span style={{ flex: 4 }}>{t("memory.col.body")}</span>
        </div>
        {notes.map((n) => (
          <div key={n.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1, fontWeight: 600 }}>{n.kind}</span>
            <span style={{ flex: 4, color: "var(--text-2)" }}>{n.body}</span>
          </div>
        ))}
        {loading || notesLoading ? <StatusLine kind="loading" /> : notes.length === 0 ? <EmptyState>{t("memory.empty")}</EmptyState> : null}
      </Card>
    </div>
  );
}
