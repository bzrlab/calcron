import { useCallback, useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";

// useList polls one dashboard.list name and returns rows. Re-fetches whenever
// `refresh` changes so views can force an update after a mutation.
export function useList<T = Record<string, unknown>>(
  name: string,
  refresh: number,
) {
  const { send, state } = useAdmin();
  const [rows, setRows] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const r = await send({ op: "dashboard.list", name });
    if (r.ok) setRows((r.data as T[]) ?? []);
    setLoading(false);
  }, [send, name]);

  useEffect(() => {
    if (state === "open") void load();
  }, [state, load, refresh]);

  return { rows, loading, reload: load };
}
