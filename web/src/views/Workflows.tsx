import { useList } from "../lib/useList";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { fmt, timeAgo } from "../lib/format";

type Instance = {
  id: string;
  application_id: string;
  workflow_name: string;
  workflow_version: number;
  current_state: string;
  status: string;
  updated_at: string;
};

export function Workflows({ refresh }: { refresh: number }) {
  const { rows, loading, reload } = useList<Instance>("workflows", refresh);

  const columns: Column<Instance>[] = [
    { key: "name", label: "Workflow", render: (w) => <span className="font-medium">{w.workflow_name}</span> },
    { key: "version", label: "Ver", render: (w) => <span className="badge badge-ghost badge-sm font-mono">v{w.workflow_version}</span> },
    { key: "state", label: "Current state", render: (w) => <code className="text-xs">{w.current_state}</code> },
    { key: "status", label: "Status", render: (w) => <StatusBadge value={w.status} /> },
    { key: "app", label: "Application", render: (w) => <code className="text-xs text-base-content/60">{w.application_id.slice(0, 10)}…</code> },
    { key: "updated", label: "Updated", render: (w) => <span title={fmt(w.updated_at)}>{timeAgo(w.updated_at)}</span> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Workflows</h2>
          <p className="text-sm text-base-content/60">
            Durable runs pinned to an immutable definition version.
          </p>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => void reload()}>Refresh</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No workflow instances." />
    </div>
  );
}
