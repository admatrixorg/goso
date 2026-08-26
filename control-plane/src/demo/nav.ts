import type { Locale } from "../i18n";
import type { IconName } from "../ui/Icon";

/** DEMO-only nav copy. Imported from App behind the DEMO const so non-demo DCE drops "Trang chủ". */
export function demoTop(locale: Locale): { id: "home" | "tasks"; label: string }[] {
  const en = locale === "en";
  return [
    { id: "home", label: en ? "Home" : "Trang chủ" },
    { id: "tasks", label: en ? "My work" : "Việc của tôi" },
  ];
}

export function demoOverviewItems(locale: Locale): { id: "home" | "tasks" | "crm" | "meetings"; label: string; ic: IconName }[] {
  const en = locale === "en";
  return [
    { id: "home", label: en ? "Home" : "Trang chủ", ic: "dash" },
    { id: "tasks", label: en ? "My work" : "Việc của tôi", ic: "check" },
    { id: "crm", label: en ? "Overview" : "Tổng quan", ic: "gauge" },
    { id: "meetings", label: en ? "Meetings" : "Cuộc họp", ic: "mic" },
  ];
}

export function demoWorkExtra(locale: Locale): { id: "friends" | "calendar" | "gallery"; label: string; ic: IconName }[] {
  const en = locale === "en";
  return [
    { id: "friends", label: en ? "Friends" : "Bạn bè", ic: "friends" },
    { id: "calendar", label: en ? "Calendar" : "Lịch hẹn", ic: "cal" },
    { id: "gallery", label: en ? "Gallery" : "Kho ảnh", ic: "gallery" },
  ];
}
