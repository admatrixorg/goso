import { jsonFetch } from "./client";
import { asPublicRecord, activityQs, type ActivityPage, type ActivityQuery, type ActivityRecord } from "./activity-ops";

export type { ActivityPage, ActivityQuery, ActivityRecord } from "./activity-ops";

export const activityApi = {
  list: async (q?: ActivityQuery): Promise<ActivityPage> => {
    const j = await jsonFetch<ActivityPage>(`/api/activity${activityQs(q)}`);
    const records = (j.records ?? []).map(asPublicRecord).filter((e): e is ActivityRecord => Boolean(e));
    return {
      records,
      total: typeof j.total === "number" ? j.total : records.length,
      limit: typeof j.limit === "number" ? j.limit : records.length,
      before: typeof j.before === "number" ? j.before : undefined,
      next_before: typeof j.next_before === "number" ? j.next_before : undefined,
    };
  },
};
