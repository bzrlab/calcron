import { useList } from "../lib/useList";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { Json } from "../components/Json";
import { fmt, timeAgo } from "../lib/format";

type Schedule = {
  id: string;
  application_id: string;
  schedule_key: string;
  event: string;
  run_at: string;
  status: string;
  updated_at: string;
};

export function Schedules({ refresh }: { refresh: number }) {
  const { rows, loading, reload } = useList<Schedule>("schedules", refresh);

  const columns: Column<Schedule>[] = [
    { key: "key", label: "Schedule key", render: (s) => <code className="text-xs">{s.schedule_key}</code> },
    { key: "event", label: "Event", render: (s) => <span className="font-medium">{s.event}</span> },
    { key: "app", label: "Application", render: (s) => <code className="text-xs text-base-content/60">{s.application_id.slice(0, 10)}…</code> },
    { key: "run", label: "Due", render: (s) => <span title={fmt(s.run_at)}>{timeAgo(s.run_at)}</span> },
    { key: "status", label: "Status", render: (s) => <StatusBadge value={s.status} /> },
    { key: "id", label: "Schedule ID", render: (s) => <Json value={{ id: s.id, key: s.schedule_key }} label="id" /> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Schedules</h2>
          <p className="text-sm text-base-content/60">
            One named future delivery per key. Most recently updated first. Schedules are
            cancelled by their owning application.
          </p>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => void reload()}>Refresh</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No schedules yet." />
    </div>
  );
}
