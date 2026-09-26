import { useList } from "../lib/useList";
import { DataTable, type Column } from "../components/DataTable";
import { Json } from "../components/Json";
import { fmt } from "../lib/format";

type HistoryRow = {
  application_id: string;
  subject_type: string;
  subject_id: string;
  event: string;
  data: unknown;
  created_at: string;
};

const TONE: Record<string, string> = {
  schedule: "text-info",
  delivery: "text-warning",
  workflow: "text-success",
};

export function History({ refresh }: { refresh: number }) {
  const { rows, loading, reload } = useList<HistoryRow>("history", refresh);

  const columns: Column<HistoryRow>[] = [
    { key: "when", label: "When", render: (h) => <span className="text-xs text-base-content/60">{fmt(h.created_at)}</span> },
    {
      key: "subject",
      label: "Subject",
      render: (h) => (
        <span className={`font-mono text-xs ${TONE[h.subject_type] ?? ""}`}>
          {h.subject_type}
        </span>
      ),
    },
    { key: "event", label: "Event", render: (h) => <span className="font-medium">{h.event}</span> },
    { key: "id", label: "Subject ID", render: (h) => <code className="text-xs text-base-content/60">{h.subject_id.slice(0, 12)}…</code> },
    { key: "app", label: "Application", render: (h) => <code className="text-xs text-base-content/60">{h.application_id.slice(0, 10)}…</code> },
    { key: "data", label: "Data", render: (h) => <Json value={h.data} /> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">History</h2>
          <p className="text-sm text-base-content/60">
            Append-only operational trail. Latest first.
          </p>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => void reload()}>Refresh</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No history yet." />
    </div>
  );
}
