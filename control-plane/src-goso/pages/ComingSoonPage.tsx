import { useI18n } from "../i18n";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function ComingSoonPage({ title }: { title: string }) {
  const { t } = useI18n();
  return (
    <div style={{ padding: "22px 28px" }}>
      <SectionHeader icon="flag" title={title} description={t("coming.desc")} />
      <EmptyState>{t("coming.empty")}</EmptyState>
    </div>
  );
}
