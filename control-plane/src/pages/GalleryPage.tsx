import { useDemoT } from "../demo/i18n";
import { Button } from "../ui/Button";
import { DemoBadge } from "../ui/DemoBadge";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function GalleryPage() {
  const { d } = useDemoT();
  return (
    <div style={{ padding: "14px 22px", display: "flex", flexDirection: "column", gap: 10 }}>
      <SectionHeader
        icon="gallery"
        title={d("gallery.title")}
        description={d("gallery.desc")}
        actions={
          <>
            <DemoBadge />
            <Button variant="primary" icon="plus">
              {d("gallery.upload")}
            </Button>
          </>
        }
      />
      <div style={{ display: "flex", alignItems: "center", borderBottom: "1px solid var(--border)", fontSize: 13.5 }}>
        <div style={{ padding: "8px 14px", fontWeight: 600, borderBottom: "2px solid var(--btn-dark-bg)" }}>{d("gallery.photos")}</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>{d("gallery.albums")}</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>{d("gallery.files")}</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>{d("gallery.video")}</div>
      </div>
      <EmptyState>
        {d("gallery.empty")}
        <div style={{ fontStyle: "normal", marginTop: 8, color: "var(--text-3)" }}>{d("gallery.hint")}</div>
      </EmptyState>
    </div>
  );
}
