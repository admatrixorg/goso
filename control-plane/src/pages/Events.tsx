import { useEffect, useState } from "react";
import { api, type GatewayEvent } from "../api/client";

export function EventsPage() {
  const [events, setEvents] = useState<GatewayEvent[]>([]);
  const [kind, setKind] = useState("");
  const [connector, setConnector] = useState("");
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await api.listEvents({
        kind: kind || undefined,
        connector: connector || undefined,
        limit: 100,
      });
      setEvents(j.events ?? []);
      setErr("");
    } catch (e) {
      setErr(String(e));
    }
  }
  useEffect(() => { void load(); }, []);

  return (
    <section>
      <h2>Events</h2>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <div style={{ display: "flex", gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
        <input placeholder="kind" value={kind} onChange={(e) => setKind(e.target.value)} />
        <input placeholder="connector" value={connector} onChange={(e) => setConnector(e.target.value)} />
        <button onClick={() => void load()}>Refresh</button>
      </div>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
        <thead>
          <tr>
            <th align="left">ts</th>
            <th align="left">kind</th>
            <th align="left">connector</th>
            <th align="left">tool</th>
            <th align="left">trace</th>
            <th align="left">summary</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e, i) => (
            <tr key={e.trace_id + e.kind + i}>
              <td><small>{e.ts}</small></td>
              <td>{e.kind}</td>
              <td>{e.connector}</td>
              <td>{e.tool}</td>
              <td><code>{e.trace_id}</code></td>
              <td><small>{e.summary}</small></td>
            </tr>
          ))}
          {events.length === 0 && (
            <tr><td colSpan={6} style={{ color: "#666" }}>(no events)</td></tr>
          )}
        </tbody>
      </table>
    </section>
  );
}
