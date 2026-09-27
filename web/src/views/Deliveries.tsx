import { useMemo, useState } from "react";
import { useList } from "../lib/useList";
import { canCancel, canReplay, useDeliveryActions } from "../lib/useDeliveryActions";
import { useShell } from "../lib/shell";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { AppName, Chips, Drawer, Fields, IdChip, Pre, Section, SubjectHistory } from "../components/ui";
import { fmt, timeAgo } from "../lib/format";

type Delivery = {
  id: string;
  schedule_id: string | null;
  workflow_instance_id: string | null;
  application_id: string;
  event: string;
  payload: unknown;
  status: string;
  attempts: number;
  next_attempt_at: string;
  last_sent_at: string | null;
  acked_at: string | null;
  created_at: string;
};

type Filter = "all" | "blocked" | "pending" | "acked" | "cancelled";
const FILTERS: Filter[] = ["blocked", "pending", "acked", "cancelled", "all"];

export function Deliveries() {
  const { rows, loading, reload } = useList<Delivery>("deliveries");
  const { selected, select, go, apps, appName } = useShell();
  const [picked, setPicked] = useState<Filter | null>(null);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: rows.length };
    for (const d of rows) c[d.status] = (c[d.status] ?? 0) + 1;
    return c;
  }, [rows]);
  const filter: Filter = picked ?? (counts.blocked ? "blocked" : "all");
  const visible = filter === "all" ? rows : rows.filter((d) => d.status === filter);
  const current = rows.find((d) => d.id === selected);

  const { replay, cancel } = useDeliveryActions(() => void reload());

  const columns: Column<Delivery>[] = [
    { key: "status", label: "Status", render: (d) => <StatusBadge value={d.status} /> },
    { key: "event", label: "Event", render: (d) => <span className="font-medium">{d.event}</span> },
    { key: "app", label: "Application", render: (d) => <AppName id={d.application_id} /> },
    { key: "attempts", label: "Attempts", render: (d) => <span className="tabular-nums">{d.attempts}</span> },
    {
      key: "next",
      label: "Next attempt",
      render: (d) => (d.status === "pending" ? <span title={fmt(d.next_attempt_at)}>{timeAgo(d.next_attempt_at)}</span> : <span className="text-base-content/40">—</span>),
    },
    { key: "created", label: "Created", render: (d) => <span className="text-xs text-base-content/60" title={fmt(d.created_at)}>{timeAgo(d.created_at)}</span> },
  ];

  const connected = current && apps.find((a) => a.id === current.application_id)?.connected;

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Deliveries</h2>
        <p className="text-sm text-base-content/60">
          Durable at-least-once attempts. Delivery ID is stable across retries; acknowledgement is not business completion.
        </p>
      </header>
      <Chips<Filter>
        value={filter}
        onChange={setPicked}
        options={FILTERS.map((f) => ({ value: f, label: f === "all" ? "All" : f[0].toUpperCase() + f.slice(1), count: counts[f] ?? 0 }))}
      />
      <DataTable
        columns={columns}
        rows={visible}
        loading={loading}
        empty={filter === "blocked" ? "Nothing blocked." : "No deliveries."}
        onRowClick={(d) => select(d.id)}
        selectedId={selected}
      />

      {current && (
        <Drawer title={current.event} onClose={() => select()}>
          <div className="flex gap-2">
            <button className="btn btn-primary btn-sm" disabled={!canReplay(current)} onClick={() => void replay(current)}>
              Replay
            </button>
            <button className="btn btn-outline btn-error btn-sm" disabled={!canCancel(current)} onClick={() => void cancel(current)}>
              Cancel
            </button>
          </div>
          {current.status === "pending" && !connected && (
            <div role="alert" className="alert py-2 text-sm">
              {appName(current.application_id)} is offline. Delivery resumes when it reconnects; this is not a failure.
            </div>
          )}
          {current.status === "pending" && connected && current.attempts > 0 && (
            <div role="alert" className="alert alert-warning py-2 text-sm">
              {appName(current.application_id)} is online but has not acknowledged after {current.attempts} attempt(s).
            </div>
          )}
          <Fields
            rows={[
              ["Delivery ID", <IdChip value={current.id} />],
              current.schedule_id
                ? ["Schedule", <IdChip value={current.schedule_id} onOpen={() => go("schedules", current.schedule_id!)} />]
                : ["Workflow instance", <IdChip value={current.workflow_instance_id ?? ""} onOpen={() => go("workflows", current.workflow_instance_id!)} />],
              ["Application", <AppName id={current.application_id} />],
              ["Status", <StatusBadge value={current.status} />],
              ["Attempts", current.attempts],
              ["Last sent", current.last_sent_at ? fmt(current.last_sent_at) : "never"],
              ["Acknowledged", current.acked_at ? fmt(current.acked_at) : "—"],
              ["Next attempt", current.status === "pending" ? fmt(current.next_attempt_at) : "—"],
            ]}
          />
          <Section title="Payload"><Pre value={current.payload} /></Section>
          <Section title="History"><SubjectHistory type="delivery" id={current.id} /></Section>
        </Drawer>
      )}
    </div>
  );
}
