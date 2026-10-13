import { useCallback, useEffect, useRef, useState } from "react";
import { useAdmin } from "../lib/admin";
import { toastOnce, useShell } from "./shell";
import type { DashboardList } from "./dashboard";

// useList loads one dashboard.list name and re-fetches on every shared refresh
// tick. `params` are server-side filters (history only).
export function useList<T = Record<string, unknown>>(
  name: DashboardList,
  params: Record<string, string | number> = {},
) {
  const { send, state } = useAdmin();
  const { refresh, markUpdated, notify } = useShell();
  const [rows, setRows] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const seen = useRef("");
  const key = JSON.stringify(params);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await send({ op: "dashboard.list", name, ...JSON.parse(key) });
      if (r.ok) {
        toastOnce(seen, notify, null);
        setRows((r.data as T[]) ?? []);
        markUpdated();
        return;
      }
      toastOnce(seen, notify, r.error ?? `${name} failed`);
    } catch (error: unknown) {
      toastOnce(seen, notify, error instanceof Error ? error.message : `${name} failed`);
    } finally {
      setLoading(false);
    }
  }, [send, name, key, markUpdated, notify]);

  useEffect(() => {
    if (state === "open") void load();
  }, [state, load, refresh]);

  return { rows, loading, reload: load };
}
