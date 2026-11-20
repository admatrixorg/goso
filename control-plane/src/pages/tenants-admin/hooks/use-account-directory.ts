import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useTenantsAdmin } from "./use-tenants-admin";
import {
  mergeAccountDirectory,
  type DirectoryAccount,
  type ListUsersItem,
} from "../account-directory";

interface ListUsersResponse {
  users?: ListUsersItem[];
}

/**
 * Danh bạ tài khoản của toàn hệ thống (mọi tenant).
 *
 * GoClaw bắt buộc `GET /v1/users` phải kèm tenant_id, không có endpoint toàn
 * cục, nên hook gọi lần lượt theo từng tenant rồi gộp lại. Số tenant nhỏ (chục
 * cái) nên fan-out như vậy chấp nhận được; đổi lại không phải vá binary Go.
 *
 * Một tenant lỗi (mất quyền, mạng rớt) không được làm hỏng cả danh bạ — dùng
 * allSettled và bỏ qua nhánh hỏng.
 */
export function useAccountDirectory(enabled: boolean) {
  const http = useHttp();
  const { tenants, loading: tenantsLoading } = useTenantsAdmin();
  const tenantIds = tenants.map((t) => t.id).sort();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["account-directory", tenantIds],
    enabled: enabled && tenantIds.length > 0,
    staleTime: 30_000,
    queryFn: async (): Promise<DirectoryAccount[]> => {
      const settled = await Promise.allSettled(
        tenantIds.map((tid) =>
          http.get<ListUsersResponse>("/v1/users", { tenant_id: tid }),
        ),
      );
      const batches = settled.map((r) =>
        r.status === "fulfilled" ? (r.value?.users ?? []) : [],
      );
      return mergeAccountDirectory(batches);
    },
  });

  return {
    accounts: data ?? [],
    loading: tenantsLoading || isLoading,
    failed: isError,
    tenantNameById: new Map(tenants.map((t) => [t.id, t.name || t.slug])),
  };
}
