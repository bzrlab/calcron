import { useEffect, useState } from "react";
import { useAdmin } from "./admin";
import type { CalendarDefinition } from "./calendar";

type Start = { id: string; application_id: string; name: string; workflow_name: string; calendar_name: string; local_time: string; updated_at: string };

// Display zones differ from the browser by at most 26 hours, so fetch two extra days each side of the grid.
const PAD_DAYS = 2;

export const startEvent = (s: Start, at: Date) => ({ key: `start:${s.id}:${at.getTime()}`, at, label: `${s.name} → ${s.workflow_name}`, tone: "primary" as const });

// Start schedules store only their next run; later runs come from the server's calendar.occurrences.
// Passing `draft` projects every Start schedule against that unsaved definition instead of its stored calendar.
// ponytail: one request per Start schedule (dashboard.list caps them at 100); batch in one op if that cap grows.
export function useStartOccurrences<S extends Start>(starts: S[], shown: Date[], version: string, draft?: CalendarDefinition) {
  const { send } = useAdmin();
  const [out, setOut] = useState<{ s: S; at: Date }[]>([]);
  const first = shown[0];
  const last = shown[shown.length - 1];
  const from = new Date(first.getFullYear(), first.getMonth(), first.getDate() - PAD_DAYS);
  const to = new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1 + PAD_DAYS);
  const key = `${starts.map((s) => s.id + s.updated_at).join()}|${from.getTime()}|${to.getTime()}|${version}|${JSON.stringify(draft ?? null)}`;
  useEffect(() => {
    let cancelled = false;
    const at = new Date(Math.max(from.getTime(), Date.now()));
    if (at >= to) {
      setOut([]);
      return;
    }
    void Promise.all(
      starts.map(async (s) => {
        const r = await send({
          op: "calendar.occurrences",
          applicationId: s.application_id,
          localTime: s.local_time,
          at: at.toISOString(),
          until: to.toISOString(),
          ...(draft ? { data: draft } : { calendar: s.calendar_name }),
        });
        return r.ok ? (r.data as { occurrences: string[] }).occurrences.map((x) => ({ s, at: new Date(x) })) : [];
      }),
    ).then((all) => !cancelled && setOut(all.flat()));
    return () => { cancelled = true; };
  }, [send, key]);
  return out;
}
