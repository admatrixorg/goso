import { useEffect, useRef, useState } from "react";
import { storageApi, type StorageEntry, type StorageListing, type StoragePreview } from "../api/storage";
import {
  asPublicListing,
  asPublicPreview,
  formatBytes,
  formatWhen,
  isImageType,
  publicHasSecrets,
  quotaOver,
  storageConfirmMatch,
} from "../api/storage-ops";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function StoragePage() {
  const { t } = useI18n();
  const [listing, setListing] = useState<StorageListing | null>(null);
  const [path, setPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [selected, setSelected] = useState<StorageEntry | null>(null);
  const [preview, setPreview] = useState<StoragePreview | null>(null);
  const [imageUrl, setImageUrl] = useState("");
  const [confirm, setConfirm] = useState<StorageEntry | null>(null);
  const [typed, setTyped] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const imageRef = useRef("");
  const na = t("storage.na");
  const configured = listing?.configured === true;
  const over = listing ? quotaOver(listing.used_bytes, listing.max_bytes) : false;
  const matched = confirm ? storageConfirmMatch(typed, confirm) : false;

  function revokeImage() {
    if (imageRef.current) URL.revokeObjectURL(imageRef.current);
    imageRef.current = "";
    setImageUrl("");
  }

  async function load(nextPath = path) {
    setLoading(true);
    try {
      const raw = await storageApi.list(nextPath);
      const next = asPublicListing(raw);
      setListing(next);
      const leak = publicHasSecrets(raw) || (raw.entries || []).some((e) => publicHasSecrets(e));
      setErr(leak ? t("storage.leak") : "");
      if (!next.configured) setOk("");
    } catch (e) {
      setErr(formatPublicError(e));
      setListing(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load("");
    return () => {
      if (imageRef.current) URL.revokeObjectURL(imageRef.current);
    };
  }, []);

  async function openDir(next: string) {
    setPath(next);
    setSelected(null);
    setPreview(null);
    revokeImage();
    setConfirm(null);
    setOk("");
    await load(next);
  }

  async function openFile(row: StorageEntry) {
    setSelected(row);
    setConfirm(null);
    setOk("");
    setBusy("preview:" + row.path);
    revokeImage();
    try {
      if (isImageType(row.type)) {
        const blob = await storageApi.download(row.path);
        const url = URL.createObjectURL(blob);
        imageRef.current = url;
        setImageUrl(url);
        setPreview({ path: row.path, type: row.type, size: row.size, kind: "binary", bytes: 0 });
      } else {
        const raw = await storageApi.preview(row.path);
        const next = asPublicPreview(raw);
        if (!next) {
          setErr(t("storage.leak"));
          setPreview(null);
          return;
        }
        setPreview(next);
      }
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
      setPreview(null);
    } finally {
      setBusy("");
    }
  }

  async function onUpload(file: File) {
    setBusy("upload");
    try {
      await storageApi.upload(file, path);
      setOk(t("storage.uploadOk"));
      setErr("");
      await load(path);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function onDownload(row: StorageEntry) {
    setBusy("download:" + row.path);
    try {
      const blob = await storageApi.download(row.path);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = row.name;
      a.click();
      URL.revokeObjectURL(url);
      setOk(t("storage.downloadOk"));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitConfirm() {
    if (!confirm || !storageConfirmMatch(typed, confirm)) {
      setErr(t("storage.mismatch"));
      return;
    }
    setBusy("delete:" + confirm.path);
    try {
      await storageApi.remove(confirm.path, typed.trim());
      setOk(t("storage.deleteOk"));
      setErr("");
      setConfirm(null);
      setTyped("");
      if (selected?.path === confirm.path) {
        setSelected(null);
        setPreview(null);
        revokeImage();
      }
      await load(path);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const entries = listing?.entries ?? [];
  const crumbs = listing?.breadcrumbs ?? [{ name: "workspace", path: "" }];

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="doc"
        title={t("storage.title")}
        description={t("storage.desc")}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <input
              ref={fileRef}
              type="file"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void onUpload(f);
              }}
            />
            <Button
              icon="plus"
              disabled={loading || Boolean(busy) || !configured || over}
              onClick={() => fileRef.current?.click()}
            >
              {t("storage.upload")}
            </Button>
            <Button icon="refresh" iconGesture onClick={() => void load(path)} disabled={loading || Boolean(busy)}>
              {t("common.refresh")}
            </Button>
          </div>
        }
      />
      <Card>
        <CardHeader icon="lock" title={t("storage.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("storage.howBody")}
        </p>
      </Card>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {listing && configured ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: over ? "var(--orange)" : "var(--text-3)" }}>
          {t("storage.quota", { used: formatBytes(listing.used_bytes), max: formatBytes(listing.max_bytes) })}
          {over ? ` · ${t("storage.quotaFull")}` : ""}
        </p>
      ) : null}
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={t("storage.confirmDeleteTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {t("storage.confirmDeletePreview", { name: confirm.name })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("storage.confirmHint")}</p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("storage.confirmPlaceholder")}
              aria-label={t("storage.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {t("storage.confirmDelete")}
              </Button>
              <Button
                variant="quiet"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("storage.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {selected && (preview || imageUrl) ? (
        <Card>
          <CardHeader
            icon="eye"
            title={t("storage.preview")}
            meta={`${selected.name} · ${formatBytes(selected.size)}`}
          />
          <div style={{ padding: "0 16px 16px" }}>
            {preview?.kind === "denied" ? (
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("storage.previewDenied")}</p>
            ) : imageUrl ? (
              <img src={imageUrl} alt={selected.name} style={{ maxWidth: "100%", maxHeight: 280, borderRadius: 8 }} />
            ) : preview?.kind === "text" ? (
              <pre
                style={{
                  margin: 0,
                  maxHeight: 280,
                  overflow: "auto",
                  fontSize: 12,
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                  color: "var(--text-2)",
                }}
              >
                {preview.text || ""}
                {preview.truncated ? `\n${t("storage.previewTruncated")}` : ""}
              </pre>
            ) : (
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("storage.previewBinary")}</p>
            )}
            <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
              <Button variant="quiet" disabled={Boolean(busy) || selected.dir} onClick={() => void onDownload(selected)}>
                {t("storage.download")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {!loading && listing && !configured ? <EmptyState>{t("storage.notConfigured")}</EmptyState> : null}
      {configured ? (
        <Card>
          <CardHeader
            icon="doc"
            title={t("storage.list")}
            meta={t("storage.list.meta", { n: entries.length })}
          />
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap", padding: "10px 16px 0", fontSize: 12.5 }}>
            {crumbs.map((c, i) => (
              <span key={`${c.path}:${i}`} style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                {i > 0 ? <span style={{ color: "var(--text-4)" }}>/</span> : null}
                <button
                  type="button"
                  onClick={() => void openDir(c.path)}
                  style={{
                    background: "none",
                    border: 0,
                    padding: 0,
                    color: c.path === path ? "var(--text)" : "var(--accent)",
                    cursor: "pointer",
                    fontWeight: c.path === path ? 600 : 500,
                  }}
                >
                  {c.path === "" ? t("storage.root") : c.name}
                </button>
              </span>
            ))}
          </div>
          <TableScroll>
            <div
              style={{
                display: "flex",
                padding: "8px 16px",
                borderBottom: "1px solid var(--border-soft)",
                fontSize: 10,
                fontWeight: 600,
                letterSpacing: ".4px",
                color: "var(--text-3)",
              }}
            >
              <span style={{ flex: 2 }}>{t("storage.col.name")}</span>
              <span style={{ flex: 1 }}>{t("storage.col.type")}</span>
              <span style={{ flex: 0.8 }}>{t("storage.col.size")}</span>
              <span style={{ flex: 1.4 }}>{t("storage.col.mtime")}</span>
              <span style={{ flex: 1.6 }} />
            </div>
            {!loading && entries.length === 0 ? <EmptyState>{t("storage.empty")}</EmptyState> : null}
            {entries.map((row) => {
              const on = selected?.path === row.path;
              return (
                <div
                  key={row.path}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    padding: "11px 16px",
                    fontSize: 12.5,
                    borderBottom: "1px solid var(--border-soft)",
                    gap: 8,
                    background: on ? "var(--bg-2)" : undefined,
                  }}
                >
                  <button
                    type="button"
                    onClick={() => (row.dir ? void openDir(row.path) : void openFile(row))}
                    style={{
                      flex: 2,
                      fontWeight: 600,
                      textAlign: "left",
                      background: "none",
                      border: 0,
                      color: "inherit",
                      cursor: "pointer",
                      padding: 0,
                    }}
                  >
                    {row.name}
                    {row.dir ? "/" : ""}
                  </button>
                  <span style={{ flex: 1, color: "var(--text-2)" }}>
                    {row.dir ? t("storage.type.dir") : row.type || na}
                  </span>
                  <span style={{ flex: 0.8, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
                    {row.dir ? na : formatBytes(row.size)}
                  </span>
                  <span style={{ flex: 1.4, color: "var(--text-3)" }}>{formatWhen(row.mtime, na)}</span>
                  <span style={{ flex: 1.6, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
                    {!row.dir ? (
                      <Button variant="quiet" disabled={Boolean(busy)} onClick={() => void onDownload(row)}>
                        {t("storage.download")}
                      </Button>
                    ) : (
                      <Button variant="quiet" disabled={Boolean(busy)} onClick={() => void openDir(row.path)}>
                        {t("storage.open")}
                      </Button>
                    )}
                    <Button
                      variant="quiet"
                      disabled={Boolean(busy)}
                      onClick={() => {
                        setConfirm(row);
                        setTyped("");
                        setOk("");
                        setErr("");
                      }}
                    >
                      {t("common.delete")}
                    </Button>
                  </span>
                </div>
              );
            })}
          </TableScroll>
          {listing?.truncated ? (
            <p style={{ margin: "8px 16px 14px", fontSize: 12, color: "var(--orange)" }}>{t("storage.truncated")}</p>
          ) : null}
        </Card>
      ) : null}
      {listing && configured && listing.hidden_skipped > 0 ? (
        <p style={{ margin: 0, fontSize: 12, color: "var(--text-4)" }}>{t("storage.hiddenNote")}</p>
      ) : null}
    </div>
  );
}
