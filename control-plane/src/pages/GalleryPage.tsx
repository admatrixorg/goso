import { Button } from "../ui/Button";
import { DemoBadge } from "../ui/DemoBadge";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function GalleryPage() {
  return (
    <div style={{ padding: "14px 22px", display: "flex", flexDirection: "column", gap: 10 }}>
      <SectionHeader
        icon="gallery"
        title="Kho phương tiện"
        description="Tải ảnh hay dùng (bảng giá, mặt bằng, brochure) để gửi khách 1 chạm. Chưa nối storage."
        actions={
          <>
            <DemoBadge />
            <Button variant="primary" icon="plus">
              Tải lên
            </Button>
          </>
        }
      />
      <div style={{ display: "flex", alignItems: "center", borderBottom: "1px solid var(--border)", fontSize: 13.5 }}>
        <div style={{ padding: "8px 14px", fontWeight: 600, borderBottom: "2px solid var(--btn-dark-bg)" }}>Ảnh</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>Album</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>Tệp</div>
        <div style={{ padding: "8px 14px", color: "var(--text-2)" }}>Video</div>
      </div>
      <EmptyState>
        Kho ảnh của bạn đang trống.
        <div style={{ fontStyle: "normal", marginTop: 8, color: "var(--text-3)" }}>Chưa có API media — placeholder đúng design.</div>
      </EmptyState>
    </div>
  );
}
