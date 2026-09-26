const MAP: Record<string, string> = {
  scheduled: "badge-info",
  pending: "badge-warning",
  blocked: "badge-error",
  cancelled: "badge-ghost",
  delivered: "badge-info",
  acked: "badge-success",
  running: "badge-info",
  waiting_time: "badge-warning",
  waiting_signal: "badge-warning",
  waiting_ack: "badge-warning",
  completed: "badge-success",
  online: "badge-success",
  offline: "badge-ghost",
};

export function StatusBadge({ value }: { value: string }) {
  const cls = MAP[value] ?? "badge-ghost";
  return (
    <span className={`badge badge-sm gap-1 font-mono ${cls}`}>
      {value}
    </span>
  );
}

export function BoolBadge({ value, yes = "yes", no = "no" }: { value: boolean; yes?: string; no?: string }) {
  return (
    <span className={`badge badge-sm ${value ? "badge-success" : "badge-ghost"}`}>
      {value ? yes : no}
    </span>
  );
}
