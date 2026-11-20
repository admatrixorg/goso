/**
 * account-directory.ts — danh bạ tài khoản toàn hệ thống, gộp từ nhiều tenant.
 *
 * GoClaw không có endpoint liệt kê user toàn cục: `GET /v1/users` bắt buộc phải
 * kèm `tenant_id`. Nhưng response của nó đã mang sẵn `memberships`, nên chỉ cần
 * gọi lần lượt theo từng tenant rồi khử trùng là dựng được danh bạ đầy đủ —
 * không phải đụng tới backend Go (đang chạy bằng binary vá sẵn).
 *
 * Tách khỏi component để test được mà không cần dựng React.
 */

export interface ListUsersMembership {
  tenant_id: string;
  role: string;
  display_name?: string | null;
}

/** Một dòng trong response GET /v1/users?tenant_id=... */
export interface ListUsersItem {
  id: string;
  email: string;
  status: string;
  memberships?: ListUsersMembership[] | null;
}

export interface DirectoryAccount {
  id: string;
  email: string;
  status: string;
  /** Tenant tài khoản này đang thuộc về. */
  tenantIds: string[];
  /** Tên hiển thị đầu tiên tìm thấy trong các membership. */
  displayName: string | null;
}

/**
 * Gộp kết quả nhiều tenant thành một danh bạ, khử trùng theo id tài khoản.
 *
 * Một tài khoản ở nhiều tenant sẽ xuất hiện nhiều lần trong đầu vào; gộp lại
 * thành một dòng và hợp nhất danh sách tenant.
 */
export function mergeAccountDirectory(batches: Array<ListUsersItem[] | null | undefined>): DirectoryAccount[] {
  const byId = new Map<string, DirectoryAccount>();

  for (const batch of batches) {
    for (const item of batch ?? []) {
      if (!item?.id) continue;
      const existing = byId.get(item.id);
      const tenantIds = (item.memberships ?? []).map((m) => m.tenant_id).filter(Boolean);
      const displayName = (item.memberships ?? []).find((m) => m.display_name)?.display_name ?? null;

      if (!existing) {
        byId.set(item.id, {
          id: item.id,
          email: item.email ?? "",
          status: item.status ?? "",
          tenantIds: [...new Set(tenantIds)],
          displayName,
        });
        continue;
      }
      for (const tid of tenantIds) {
        if (!existing.tenantIds.includes(tid)) existing.tenantIds.push(tid);
      }
      if (!existing.displayName && displayName) existing.displayName = displayName;
    }
  }

  return [...byId.values()].sort((a, b) => a.email.localeCompare(b.email));
}

/**
 * Tài khoản có thể thêm vào tenant đang xem.
 *
 * Bỏ đi: (1) người đã ở sẵn trong tenant này — thêm nữa vô nghĩa, (2) tài khoản
 * đã bị khoá. Lọc theo từ khoá trên cả email lẫn tên hiển thị.
 */
export function selectableAccounts(
  directory: DirectoryAccount[],
  currentTenantId: string,
  query = "",
): DirectoryAccount[] {
  const q = query.trim().toLowerCase();
  return directory.filter((a) => {
    if (a.tenantIds.includes(currentTenantId)) return false;
    if (a.status && a.status !== "active") return false;
    if (!q) return true;
    return a.email.toLowerCase().includes(q) || (a.displayName ?? "").toLowerCase().includes(q);
  });
}
