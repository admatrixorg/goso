import { useEffect, useState } from "react";
import { api, type Message } from "../api/client";

export function ChatPage({ sessionId }: { sessionId: string }) {
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await api.listMessages(sessionId);
      setMsgs(j.messages ?? []);
      setErr("");
    } catch (e) { setErr(String(e)); }
  }
  useEffect(() => { if (sessionId) void load(); }, [sessionId]);

  async function send() {
    if (!input.trim()) return;
    try {
      await api.chat({ session_id: sessionId, message: input.trim() });
      setInput("");
      await load();
    } catch (e) { setErr(String(e)); }
  }

  if (!sessionId) return <p style={{ color: "#666" }}>Pick a session to chat.</p>;

  return (
    <section>
      <h3>Chat — {sessionId}</h3>
      {err && <p style={{ color: "crimson" }}>{err}</p>}
      <div style={{ border: "1px solid #ddd", padding: 8, maxHeight: 320, overflow: "auto", marginBottom: 8 }}>
        {msgs.map((m) => (
          <div key={m.id} style={{ marginBottom: 6 }}>
            <strong>{m.role}:</strong> {m.content}
          </div>
        ))}
        {msgs.length === 0 && <small style={{ color: "#666" }}>(no messages)</small>}
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <input style={{ flex: 1 }} value={input} onChange={(e) => setInput(e.target.value)} onKeyDown={(e) => e.key === "Enter" && void send()} placeholder="message" />
        <button onClick={() => void send()}>Send</button>
        <button onClick={() => void load()}>Refresh</button>
      </div>
    </section>
  );
}
