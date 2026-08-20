import { useEffect, useState } from "react";
import { api, type Session } from "../api/client";

export function SessionsPage({ onPick }: { onPick: (id: string) => void }) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await api.listSessions();
      setSessions(j.sessions ?? []);
      setErr("");
    } catch (e) { setErr(String(e)); }
  }
  useEffect(() => { void load(); }, []);

  return (
    <section>
      <h2>Sessions</h2>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <button onClick={() => void load()}>Refresh</button>
      <ul>
        {sessions.map((s) => (
          <li key={s.id}>
            <button onClick={() => onPick(s.id)}>{s.label || s.id}</button>
            <small> agent={s.agent_id} </small>
          </li>
        ))}
        {sessions.length === 0 && <li style={{ color: "#666" }}>(no sessions)</li>}
      </ul>
    </section>
  );
}
