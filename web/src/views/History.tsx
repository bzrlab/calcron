import { useEffect, useMemo, useRef, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { DataTable, type Column } from "../components/DataTable";
import { Json } from "../components/Json";
import { AppName, Chips, IdChip } from "../components/ui";
import { fmt } from "../lib/format";

type HistoryRow = {
  id: number;
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

const VIEW: Record<string, string> = { schedule: "schedules", delivery: "deliveries", workflow: "workflows" };

type Subject = "" | "schedule" | "delivery" | "workflow";

export function History() {
  const { send } = useAdmin();
  const { apps, go } = useShell();
  const [subject, setSubject] = useState<Subject>("");
  const [app, setApp] = useState("");
  const [event, setEvent] = useState("");
  const [older, setOlder] = useState<HistoryRow[]>([]);
  const [exhausted, setExhausted] = useState(false);
  const params = { subjectType: subject, applicationId: app };
  const { rows, loading } = useList<HistoryRow>("history", params);

  const filterRef = useRef({ subject, app });
  filterRef.current = { subject, app };

  useEffect(() => {
    setOlder([]);
    setExhausted(false);
  }, [subject, app]);

  const all = useMemo(() => [...rows, ...older.filter((o) => !rows.some((r) => r.id === o.id))], [rows, older]);
  const visible = event ? all.filter((h) => h.event.toLowerCase().includes(event.toLowerCase())) : all;

  async function loadOlder() {
    const last = all[all.length - 1];
    if (!last) return;
    const asked = { subject, app };
    const r = await send({ op: "dashboard.list", name: "history", ...params, before: last.id });
    if (!r.ok || filterRef.current.subject !== asked.subject || filterRef.current.app !== asked.app) return;
    const page = (r.data as HistoryRow[]) ?? [];
    setOlder((o) => [...o, ...page]);
    if (page.length < 100) setExhausted(true);
  }

  const columns: Column<HistoryRow>[] = [
    { key: "when", label: "When", render: (h) => <span className="text-xs text-base-content/60">{fmt(h.created_at)}</span> },
    { key: "subject", label: "Subject", render: (h) => <span className={`font-mono text-xs ${TONE[h.subject_type] ?? ""}`}>{h.subject_type}</span> },
    { key: "event", label: "Event", render: (h) => <span className="font-medium">{h.event}</span> },
    {
      key: "id",
      label: "Subject ID",
      render: (h) => <IdChip value={h.subject_id} onOpen={VIEW[h.subject_type] ? () => go(VIEW[h.subject_type], h.subject_id) : undefined} />,
    },
    { key: "app", label: "Application", render: (h) => <AppName id={h.application_id} /> },
    { key: "data", label: "Data", render: (h) => <Json value={h.data} /> },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">History</h2>
        <p className="text-sm text-base-content/60">
          Append-only operational trail, latest first. An audit trail, not an application's ledger.
        </p>
      </header>
      <div className="flex flex-wrap items-center gap-3">
        <Chips<Subject>
          value={subject}
          onChange={setSubject}
          options={[
            { value: "", label: "All" },
            { value: "schedule", label: "Schedules" },
            { value: "delivery", label: "Deliveries" },
            { value: "workflow", label: "Workflows" },
          ]}
        />
        <select className="select select-sm select-bordered" value={app} onChange={(e) => setApp(e.target.value)} aria-label="Filter by application">
          <option value="">All applications</option>
          {apps.map((a) => <option key={a.id} value={a.id}>{a.namespace}</option>)}
        </select>
        <input
          className="input input-sm input-bordered w-48 font-mono"
          placeholder="event contains…"
          value={event}
          onChange={(e) => setEvent(e.target.value)}
          aria-label="Filter by event"
        />
      </div>
      <DataTable columns={columns} rows={visible} loading={loading} empty="No history for these filters." />
      {all.length >= 100 && !exhausted && (
        <button className="btn btn-ghost btn-sm" onClick={() => void loadOlder()}>Load older</button>
      )}
    </div>
  );
}
