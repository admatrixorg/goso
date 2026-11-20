import { useState } from "react";
import { useI18n, type MsgKey } from "../i18n";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { BackupPanel } from "./BackupPanel";
import { WebhooksPage } from "./WebhooksPage";

function FoldNote({ href, label }: { href: string; label: string }) {
  const { t } = useI18n();
  return (
    <p className="z-fold">
      <a href={href}>{label}</a>
      <span> — {t("fold.note")}</span>
    </p>
  );
}

export function FoldedTeamsLink() {
  const { t } = useI18n();
  return <FoldNote href="#/teams" label={t("fold.teams")} />;
}

export function FoldedHeatmapLink() {
  const { t } = useI18n();
  return <FoldNote href="#/heatmap" label={t("fold.heatmap")} />;
}

export function FoldedMarketingLink() {
  const { t } = useI18n();
  return <FoldNote href="#/marketing" label={t("fold.marketing")} />;
}

const DECISION_TABS: MsgKey[] = [
  "feature.decision.tab.route",
  "feature.decision.tab.questions",
  "feature.decision.tab.model",
  "feature.decision.tab.log",
];

export function DecisionPage() {
  const { t } = useI18n();
  const [tab, setTab] = useState(0);
  return (
    <PageChrome icon="gavel" title={t("nav.decision")} description={t("feature.decision.desc")}>
      <div className="z-seg" role="tablist">
        {DECISION_TABS.map((key, i) => (
          <button key={key} type="button" role="tab" aria-selected={tab === i} className={tab === i ? "on" : ""} onClick={() => setTab(i)}>
            {t(key)}
          </button>
        ))}
      </div>
      <Card>
        <CardHeader title={t(DECISION_TABS[tab])} />
        <EmptyState>{t("feature.decision.empty")}</EmptyState>
      </Card>
      <p className="z-fold">{t("feature.decision.rules")}</p>
    </PageChrome>
  );
}

const FLOWS: MsgKey[] = ["feature.flow.in", "feature.flow.tool", "feature.flow.reply"];

export function DecisionFlowPage() {
  const { t } = useI18n();
  return (
    <PageChrome icon="branch" title={t("nav.decisionFlow")} description={t("feature.flow.desc")}>
      <div className="z-flow-grid">
        {FLOWS.map((key) => (
          <Card key={key}>
            <CardHeader title={t(key)} />
            <EmptyState>{t("feature.flow.empty")}</EmptyState>
          </Card>
        ))}
      </div>
    </PageChrome>
  );
}

export function HooksPage() {
  const { t } = useI18n();
  return (
    <div>
      <PageChrome icon="webhook" title={t("nav.hooks")} description={t("feature.hooks.desc")}>
        <EmptyState>{t("feature.hooks.empty")}</EmptyState>
      </PageChrome>
      <div style={{ padding: "0 22px 8px", fontSize: 13, fontWeight: 600 }}>{t("feature.hooks.webhooks")}</div>
      <WebhooksPage />
    </div>
  );
}

export function MeetingsLivePage() {
  const { t } = useI18n();
  const [settings, setSettings] = useState(false);
  const [upload, setUpload] = useState(false);
  const [consent, setConsent] = useState(false);
  const [note, setNote] = useState("");
  return (
    <div className="z-meet">
      <header className="z-meet-head">
        <div className="z-meet-title">
          <h1>{t("nav.meetings")}</h1>
          <p>{t("feature.meetings.desc")}</p>
        </div>
        <div className="z-meet-acts">
          <button type="button" className="z-meet-btn" onClick={() => setSettings((v) => !v)}>
            {t("feature.meetings.settings")}
          </button>
          <button
            type="button"
            className="z-meet-btn is-pri"
            onClick={() => {
              setUpload((v) => !v);
              setNote("");
            }}
          >
            + {t("feature.meetings.upload")}
          </button>
        </div>
      </header>
      <p className="z-meet-sum">
        {t("feature.meetings.sources")} <a href="#/connectors">{t("feature.meetings.openConnectors")}</a>
      </p>
      {settings ? <p className="z-meet-note">{t("feature.meetings.settingsNote")}</p> : null}
      {upload ? (
        <form
          className="z-meet-upload"
          onSubmit={(e) => {
            e.preventDefault();
            setNote(consent ? t("feature.meetings.noAsr") : t("feature.meetings.needConsent"));
          }}
        >
          <label>
            <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} />
            <span>{t("feature.meetings.consent")}</span>
          </label>
          <button type="submit" className="z-meet-btn is-pri">
            {t("feature.meetings.upload")}
          </button>
          {note ? <p className="z-meet-note">{note}</p> : null}
        </form>
      ) : null}
      <div className="z-meet-list">{t("feature.meetings.empty")}</div>
    </div>
  );
}

export function BackupPage() {
  const { t } = useI18n();
  return (
    <PageChrome icon="archive" title={t("nav.backup")} description={t("feature.backup.desc")}>
      <BackupPanel />
    </PageChrome>
  );
}
