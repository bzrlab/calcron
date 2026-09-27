import { useCallback, useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useShell } from "./shell";
import type { DashboardList } from "./dashboard";

// useList loads one dashboard.list name and re-fetches on every shared refresh
// tick. `params` are server-side filters (history only).
export function useList<T = Record<string, unknown>>(
  name: DashboardList,
  params: Record<string, string | number> = {},
) {
  const { send, state } = useAdmin();
  const { refresh, markUpdated } = useShell();
  const [rows, setRows] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const key = JSON.stringify(params);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await send({ op: "dashboard.list", name, ...JSON.parse(key) });
      if (r.ok) {
        setRows((r.data as T[]) ?? []);
        markUpdated();
      }
    } finally {
      setLoading(false);
    }
  }, [send, name, key, markUpdated]);

  useEffect(() => {
    if (state === "open") void load();
  }, [state, load, refresh]);

  return { rows, loading, reload: load };
}
