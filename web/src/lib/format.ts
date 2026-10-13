const SECOND = 1;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

function compactDuration(s: number, prefix: string, suffix: string): string {
  if (s < MINUTE) return `${prefix}${s}s${suffix}`;
  if (s < HOUR) {
    const minutes = Math.round(s / MINUTE);
    return minutes >= 60 ? `${prefix}1h${suffix}` : `${prefix}${minutes}m${suffix}`;
  }
  if (s < DAY) {
    const hours = Math.round(s / HOUR);
    return hours >= 24 ? `${prefix}1d${suffix}` : `${prefix}${hours}h${suffix}`;
  }
  return `${prefix}${Math.round(s / DAY)}d${suffix}`;
}

export function timeAgo(iso?: string | null, now = Date.now()): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return String(iso);
  const s = Math.floor((now - t) / 1000);
  if (s < 0) return inFuture(-s);
  return compactDuration(s, "", " ago");
}

function inFuture(s: number): string {
  return compactDuration(s, "in ", "");
}

export function fmt(iso?: string | null, timeZone?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return d.toLocaleString(undefined, timeZone ? { timeZone, timeZoneName: "short" } : undefined);
}
