import { timeAgo } from "./format.ts";

export const DASHBOARD_LISTS = [
  "apps",
  "schedules",
  "deliveries",
  "workflows",
  "history",
  "calendars",
  "tokens",
  "start_schedules",
  "workflow_versions",
] as const;

export type DashboardList = (typeof DASHBOARD_LISTS)[number];

type Waiting = {
  status: string;
  waiting_event: string | null;
  correlation_key: string | null;
  wake_at: string | null;
};

export function waitingReason(w: Waiting, now = new Date()): string {
  switch (w.status) {
    case "waiting_signal":
      return [w.waiting_event, w.correlation_key].filter(Boolean).join(" / ");
    case "waiting_time":
      return `wakes ${timeAgo(w.wake_at, now.getTime())}`;
    case "waiting_ack":
      return "awaiting acknowledgement";
    default:
      return "";
  }
}

export function parseHash(hash: string): { view: string; id?: string } {
  const [view, ...rest] = hash.replace(/^#/, "").split("/");
  const id = rest.join("/");
  return { view, id: id ? decodeURIComponent(id) : undefined };
}

export function hashFor(view: string, id?: string): string {
  return id ? `${view}/${encodeURIComponent(id)}` : view;
}

export function findById(
  lists: Partial<Record<string, { id?: unknown }[]>>,
  raw: string,
): { view: string; id: string } | null {
  const id = raw.trim();
  if (!id) return null;
  for (const [view, rows] of Object.entries(lists)) {
    if (rows?.some((r) => r.id === id)) return { view, id };
  }
  return null;
}

export type JsonError = { message: string; line?: number; column?: number };

// jsonError maps JSON.parse's character position to a line and column. Engines
// that omit the position still get their raw message.
export function jsonError(raw: string): JsonError | null {
  try {
    JSON.parse(raw);
    return null;
  } catch (e) {
    const detail = (e instanceof Error ? e.message : String(e)).replace(/\s*\(line \d+ column \d+\)$/, "");
    const position = /position (\d+)/.exec(detail);
    if (!position) return { message: detail };
    const before = raw.slice(0, Number(position[1])).split("\n");
    const line = before.length;
    const column = before[before.length - 1].length + 1;
    return { line, column, message: `Line ${line}, column ${column}: ${detail.replace(/ in JSON at position \d+/, "")}` };
  }
}

export type DueBucket = "overdue" | "hour" | "today" | "later" | "done";

export function dueBucket(runAt: string, status: string, now = new Date()): DueBucket {
  if (status !== "scheduled") return "done";
  const t = new Date(runAt).getTime();
  const n = now.getTime();
  if (t <= n) return "overdue";
  if (t - n <= 3_600_000) return "hour";
  const endOfDay = new Date(now);
  endOfDay.setHours(23, 59, 59, 999);
  return t <= endOfDay.getTime() ? "today" : "later";
}
