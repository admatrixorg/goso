import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function ComingSoonPage({ title }: { title: string }) {
  return (
    <div style={{ padding: "22px 28px" }}>
      <SectionHeader icon="flag" title={title} description="Trạng thái đang phát triển — không có API trên GOSO." />
      <EmptyState>Màn hình này không có trên bản non-demo. Bật VITE_DEMO_MODE=true để xem mock + badge DEMO.</EmptyState>
    </div>
  );
}
