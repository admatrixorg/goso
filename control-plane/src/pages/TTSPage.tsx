import { useEffect, useState } from "react";
import { ttsApi, type TTSStatus } from "../api/tts";
import {
  TTS_APPLY,
  TTS_PROVIDERS,
  emptyStatus,
  formatTTSTest,
  parseTTSTestError,
  publicHasSecrets,
  requiresKey,
  statusKind,
  ttsConfirmMatch,
  ttsWriteBody,
  type TTSTestView,
} from "../api/tts-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

const PROVIDER_KEYS: Record<string, MsgKey> = {
  none: "tts.provider.none",
  openai: "tts.provider.openai",
  elevenlabs: "tts.provider.elevenlabs",
  google: "tts.provider.google",
  azure: "tts.provider.azure",
  edge: "tts.provider.edge",
};

const APPLY_KEYS: Record<string, MsgKey> = {
  off: "tts.apply.off",
  reply: "tts.apply.reply",
  all: "tts.apply.all",
};

type Form = {
  provider: string;
  enabled: boolean;
  api_key: string;
  voice: string;
  model: string;
  language: string;
  region: string;
  endpoint: string;
  auto_apply: string;
  max_chars: string;
  timeout_ms: string;
};

function formFrom(row: TTSStatus): Form {
  return {
    provider: row.provider || "none",
    enabled: row.enabled !== false,
    api_key: "",
    voice: row.voice || "",
    model: row.model || "",
    language: row.language || "",
    region: row.region || "",
    endpoint: row.endpoint || "",
    auto_apply: row.auto_apply || "off",
    max_chars: String(row.max_chars || 4096),
    timeout_ms: String(row.timeout_ms || 15000),
  };
}

export function TTSPage() {
  const { t } = useI18n();
  const [row, setRow] = useState<TTSStatus>(emptyStatus());
  const [form, setForm] = useState<Form>(formFrom(emptyStatus()));
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [testView, setTestView] = useState<TTSTestView | null>(null);
  const [confirm, setConfirm] = useState(false);
  const [typed, setTyped] = useState("");
  const [advanced, setAdvanced] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const next = await ttsApi.get();
      setRow(next);
      setForm(formFrom(next));
      setErr(publicHasSecrets(next) ? t("tts.leak") : "");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const kind = statusKind(row);
  const needKey = requiresKey(form.provider);
  const envLocked = Boolean(row.env_owned);
  const matched = ttsConfirmMatch(typed, row.provider);

  function patch<K extends keyof Form>(key: K, value: Form[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function save() {
    setBusy("save");
    try {
      const next = await ttsApi.put(
        ttsWriteBody({
          provider: form.provider,
          enabled: form.enabled,
          api_key: form.api_key,
          voice: form.voice,
          model: form.model,
          language: form.language,
          region: form.region,
          endpoint: form.endpoint,
          auto_apply: form.auto_apply,
          max_chars: Number(form.max_chars),
          timeout_ms: Number(form.timeout_ms),
        }),
      );
      if (publicHasSecrets(next)) {
        setErr(t("tts.leak"));
      } else {
        setRow(next);
        setForm(formFrom(next));
        setOk(t("tts.saved"));
        setErr("");
      }
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function test() {
    setBusy("test");
    setTestView(null);
    try {
      const raw = await ttsApi.test();
      const view = formatTTSTest(raw);
      setTestView(view);
      if (view.ok) {
        setOk(t("tts.testOk"));
        setErr("");
      } else {
        setErr(view.error || t("tts.testFail"));
      }
    } catch (e) {
      const parsed = parseTTSTestError(e);
      if (parsed) {
        setTestView(parsed);
        setErr(parsed.error || t("tts.testFail"));
      } else {
        setErr(formatPublicError(e));
      }
    } finally {
      setBusy("");
    }
  }

  async function clearKey() {
    if (!matched) {
      setErr(t("tts.mismatch"));
      return;
    }
    setBusy("clear");
    try {
      const next = await ttsApi.clear(typed.trim());
      if (publicHasSecrets(next)) {
        setErr(t("tts.leak"));
      } else {
        setRow(next);
        setForm(formFrom(next));
        setOk(t("tts.cleared"));
        setErr("");
        setConfirm(false);
        setTyped("");
      }
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="mic"
        title={t("tts.title")}
        description={t("tts.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("tts.hint")}</p>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}

      {!loading && kind === "not_configured" ? <EmptyState>{t("tts.empty")}</EmptyState> : null}
      {!loading && kind === "disabled" ? <EmptyState>{t("tts.disabled")}</EmptyState> : null}

      <Card>
        <CardHeader
          icon="mic"
          title={t("tts.status")}
          meta={
            <span style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
              <Badge tone={kind === "ready" ? "positive" : kind === "disabled" ? "warning" : "neutral"}>
                {kind === "ready" ? t("tts.configured") : kind === "disabled" ? t("tts.badge.disabled") : t("tts.notConfigured")}
              </Badge>
              <Badge tone={row.key_set ? "positive" : "neutral"}>{row.key_set ? t("tts.keySet") : t("tts.keyMissing")}</Badge>
              {row.env_owned ? <Badge tone="warning">{t("tts.env")}</Badge> : null}
            </span>
          }
        />
        <div style={{ display: "flex", flexDirection: "column", gap: 10, padding: "0 16px 16px" }}>
          <label style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 13 }}>
            <input
              type="checkbox"
              checked={form.enabled}
              disabled={Boolean(busy)}
              onChange={(e) => patch("enabled", e.target.checked)}
            />
            {t("tts.enabled")}
          </label>
          <label style={{ fontSize: 12.5, color: "var(--text-3)" }}>
            {t("tts.provider")}
            <select
              className="z-field"
              value={form.provider}
              disabled={Boolean(busy)}
              onChange={(e) => patch("provider", e.target.value)}
              style={{ display: "block", width: "100%", marginTop: 4 }}
            >
              {TTS_PROVIDERS.map((p) => (
                <option key={p} value={p}>
                  {t(PROVIDER_KEYS[p])}
                </option>
              ))}
            </select>
          </label>
          {form.provider !== "none" ? (
            <>
              <input
                className="z-field"
                placeholder={t("tts.voice")}
                value={form.voice}
                disabled={Boolean(busy)}
                onChange={(e) => patch("voice", e.target.value)}
              />
              {form.provider === "openai" || form.provider === "elevenlabs" ? (
                <input
                  className="z-field"
                  placeholder={t("tts.model")}
                  value={form.model}
                  disabled={Boolean(busy)}
                  onChange={(e) => patch("model", e.target.value)}
                />
              ) : null}
              {form.provider === "google" ? (
                <input
                  className="z-field"
                  placeholder={t("tts.language")}
                  value={form.language}
                  disabled={Boolean(busy)}
                  onChange={(e) => patch("language", e.target.value)}
                />
              ) : null}
              {form.provider === "azure" ? (
                <input
                  className="z-field"
                  placeholder={t("tts.region")}
                  value={form.region}
                  disabled={Boolean(busy)}
                  onChange={(e) => patch("region", e.target.value)}
                />
              ) : null}
              {form.provider === "openai" || form.provider === "azure" || form.provider === "elevenlabs" ? (
                <input
                  className="z-field"
                  placeholder={t("tts.endpoint")}
                  value={form.endpoint}
                  disabled={Boolean(busy)}
                  onChange={(e) => patch("endpoint", e.target.value)}
                />
              ) : null}
            </>
          ) : null}
          {needKey ? (
            <input
              className="z-field"
              type="password"
              autoComplete="off"
              placeholder={row.key_set ? t("tts.keyRotate") : t("tts.key")}
              value={form.api_key}
              disabled={envLocked || Boolean(busy)}
              onChange={(e) => patch("api_key", e.target.value)}
            />
          ) : null}
          {envLocked ? <p style={{ margin: 0, fontSize: 12.5, color: "var(--orange)" }}>{t("tts.envHint")}</p> : null}
          <label style={{ fontSize: 12.5, color: "var(--text-3)" }}>
            {t("tts.apply")}
            <select
              className="z-field"
              value={form.auto_apply}
              disabled={Boolean(busy)}
              onChange={(e) => patch("auto_apply", e.target.value)}
              style={{ display: "block", width: "100%", marginTop: 4 }}
            >
              {TTS_APPLY.map((a) => (
                <option key={a} value={a}>
                  {t(APPLY_KEYS[a])}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            onClick={() => setAdvanced((v) => !v)}
            style={{
              background: "transparent",
              border: "none",
              textAlign: "left",
              padding: 0,
              color: "var(--accent)",
              fontSize: 12.5,
            }}
          >
            {advanced ? t("tts.advanced.hide") : t("tts.advanced")}
          </button>
          {advanced ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <input
                className="z-field"
                placeholder={t("tts.maxChars")}
                value={form.max_chars}
                disabled={Boolean(busy)}
                onChange={(e) => patch("max_chars", e.target.value)}
              />
              <input
                className="z-field"
                placeholder={t("tts.timeout")}
                value={form.timeout_ms}
                disabled={Boolean(busy)}
                onChange={(e) => patch("timeout_ms", e.target.value)}
              />
            </div>
          ) : null}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button variant="primary" disabled={Boolean(busy)} onClick={() => void save()}>
              {t("tts.save")}
            </Button>
            <Button disabled={Boolean(busy)} onClick={() => void test()}>
              {t("tts.test")}
            </Button>
            <Button
              disabled={Boolean(busy) || envLocked || (!row.key_set && row.provider === "none")}
              onClick={() => {
                setConfirm(true);
                setTyped("");
              }}
            >
              {t("tts.clear")}
            </Button>
          </div>
        </div>
      </Card>

      {testView ? (
        <Card>
          <CardHeader icon="pulse" title={t("tts.testTitle")} />
          <div style={{ padding: "0 16px 16px", fontSize: 13, display: "flex", flexDirection: "column", gap: 6 }}>
            <div>
              <Badge tone={testView.ok ? "positive" : "critical"}>{testView.ok ? t("tts.testOk") : t("tts.testFail")}</Badge>
            </div>
            {testView.kind ? (
              <p style={{ margin: 0, color: "var(--text-3)" }}>
                {t("tts.testKind")}: {testView.kind}
              </p>
            ) : null}
            <p style={{ margin: 0, color: "var(--text-3)" }}>
              {t("tts.latency")}: {testView.latency_ms}ms
            </p>
            {testView.error ? (
              <p role="alert" style={{ margin: 0, color: "var(--red)" }}>
                {testView.error}
              </p>
            ) : null}
          </div>
        </Card>
      ) : null}

      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={t("tts.confirmTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 8 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("tts.confirmPreview", { name: row.provider || "tts" })}</p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("tts.confirmHint")}</p>
            <input
              className="z-field"
              placeholder={t("tts.confirmPlaceholder")}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
            />
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" disabled={!matched || Boolean(busy)} onClick={() => void clearKey()}>
                {t("tts.confirmClear")}
              </Button>
              <Button
                onClick={() => {
                  setConfirm(false);
                  setTyped("");
                }}
              >
                {t("tts.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
    </div>
  );
}
