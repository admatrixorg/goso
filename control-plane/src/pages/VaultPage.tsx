import { useEffect, useState } from "react";
import { vaultApi, type VaultDoc, type VaultLink, type VaultSearchHit, type VaultSyncResult } from "../api/vault";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function VaultPage() {
  const { t } = useI18n();
  const [docs, setDocs] = useState<VaultDoc[]>([]);
  const [selected, setSelected] = useState<VaultDoc | null>(null);
  const [inbound, setInbound] = useState<VaultLink[]>([]);
  const [outbound, setOutbound] = useState<VaultLink[]>([]);
  const [hits, setHits] = useState<VaultSearchHit[] | null>(null);
  const [sync, setSync] = useState<VaultSyncResult | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");

  async function load() {
    try {
      const j = await vaultApi.list();
      setDocs(j.docs ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  async function openDoc(id: string) {
    try {
      const [d, links] = await Promise.all([vaultApi.get(id), vaultApi.links(id)]);
      setSelected(d);
      setInbound(links.inbound ?? []);
      setOutbound(links.outbound ?? []);
      setTitle(d.title);
      setBody(d.body ?? "");
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function search() {
    const query = q.trim();
    if (!query) {
      setHits(null);
      return;
    }
    try {
      setHits(await vaultApi.search(query));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function putDoc() {
    if (!title.trim()) {
      setErr(t("vault.needTitle"));
      return;
    }
    try {
      const d = await vaultApi.put({ title: title.trim(), body });
      setSelected(d);
      setErr("");
      await load();
      await openDoc(d.id);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function runSync() {
    try {
      setSync(await vaultApi.sync());
      await load();
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="doc"
        title={t("vault.title")}
        description={t("vault.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button icon="refresh" onClick={() => void runSync()}>
              {t("vault.sync")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void putDoc()}>
              {t("vault.put")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {sync ? (
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
          {t("vault.syncResult", { upserted: sync.upserted, skipped: sync.skipped, deleted: sync.deleted })}
        </p>
      ) : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder={t("vault.search")} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void search(); }} />
        <Button icon="search" onClick={() => void search()}>
          {t("common.search")}
        </Button>
        <input className="z-field" placeholder={t("vault.titleField")} value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <textarea className="z-field" placeholder={t("vault.body")} value={body} onChange={(e) => setBody(e.target.value)} rows={6} style={{ minHeight: 90, resize: "vertical" }} />
      {hits ? (
        <Card>
          <CardHeader icon="search" title={t("vault.hits")} meta={String(hits.length)} />
          {hits.map((h) => (
            <button
              key={h.id}
              type="button"
              onClick={() => void openDoc(h.id)}
              style={{ display: "block", width: "100%", textAlign: "left", background: "transparent", border: "none", borderBottom: "1px solid var(--border-soft)", padding: "10px 16px" }}
            >
              <div style={{ fontSize: 13, fontWeight: 600 }}>{h.title}</div>
              <div style={{ fontSize: 12, color: "var(--text-3)" }}>{h.snippet || h.path}</div>
            </button>
          ))}
          {hits.length === 0 ? <EmptyState>{t("vault.emptyHits")}</EmptyState> : null}
        </Card>
      ) : null}
      <div className="z-two-col">
        <Card>
          <CardHeader icon="doc" title={t("vault.list")} meta={t("vault.meta", { n: docs.length })} />
          <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 2 }}>{t("vault.col.title")}</span>
            <span style={{ flex: 2 }}>{t("vault.col.path")}</span>
          </div>
          {docs.map((d) => (
            <div
              key={d.id}
              onClick={() => void openDoc(d.id)}
              style={{
                display: "flex",
                padding: "11px 16px",
                fontSize: 12.5,
                borderBottom: "1px solid var(--border-soft)",
                cursor: "pointer",
                background: selected?.id === d.id ? "var(--accent-soft)" : "transparent",
              }}
            >
              <span style={{ flex: 2, fontWeight: 600 }}>{d.title}</span>
              <span style={{ flex: 2, color: "var(--text-3)" }}>{d.path}</span>
            </div>
          ))}
          {loading ? <StatusLine kind="loading" /> : docs.length === 0 ? <EmptyState>{t("vault.empty")}</EmptyState> : null}
          </TableScroll>
        </Card>
        <Card>
          <CardHeader icon="hook" title={t("vault.links")} meta={selected?.title} />
          <div style={{ padding: "10px 16px", fontSize: 12.5 }}>
            <div style={{ fontWeight: 600, marginBottom: 6 }}>{t("vault.outbound")}</div>
            {(outbound ?? []).map((l, i) => (
              <div key={`o-${l.from_id}-${l.raw}-${i}`} style={{ color: "var(--text-2)", padding: "3px 0" }}>
                [[{l.raw}]] {l.to_id ? `→ ${l.to_id}` : ""}
              </div>
            ))}
            {outbound.length === 0 ? <div style={{ color: "var(--text-4)", fontStyle: "italic" }}>{t("vault.emptyLinks")}</div> : null}
            <div style={{ fontWeight: 600, margin: "12px 0 6px" }}>{t("vault.inbound")}</div>
            {(inbound ?? []).map((l, i) => (
              <div key={`i-${l.from_id}-${l.raw}-${i}`} style={{ color: "var(--text-2)", padding: "3px 0" }}>
                {l.from_id} → [[{l.raw}]]
              </div>
            ))}
            {inbound.length === 0 ? <div style={{ color: "var(--text-4)", fontStyle: "italic" }}>{t("vault.emptyLinks")}</div> : null}
            {selected?.body ? (
              <pre style={{ marginTop: 12, whiteSpace: "pre-wrap", fontSize: 12, color: "var(--text-2)", fontFamily: "inherit" }}>{selected.body}</pre>
            ) : null}
          </div>
        </Card>
      </div>
    </div>
  );
}
