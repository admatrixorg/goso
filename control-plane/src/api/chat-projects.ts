/** Minh An (demo) projects shown in the Trò chuyện sidebar. Not a gateway store. */

export type ChatProject = {
  id: string;
  name: string;
  closed: boolean;
};

export const MINH_AN_PROJECTS: ChatProject[] = [
  { id: "mo-kenh-mien-tay", name: "Mở kênh đại lý miền Tây", closed: false },
  { id: "noi-chien-moi", name: "Ra mắt nồi chiên dòng mới", closed: false },
  { id: "kiem-toan-2025", name: "Kiểm toán báo cáo tài chính 2025", closed: true },
];

export function runningProjects(rows: ChatProject[] = MINH_AN_PROJECTS): ChatProject[] {
  return rows.filter((row) => !row.closed);
}

export function closedProjects(rows: ChatProject[] = MINH_AN_PROJECTS): ChatProject[] {
  return rows.filter((row) => row.closed);
}
