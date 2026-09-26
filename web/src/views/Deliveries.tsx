import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { fmt, timeAgo } from "../lib/format";

type Delivery = {
  id: string;
  application_id: string;
  event: string;
  status: string;
  attempts: number;
  next_attempt_at: string;
  created_at: string;
};

export function Deliveries({ refresh }: { refresh: number }) {
  const { send } = useAdmin();
  const { rows, loading, reload } = useList<Delivery>("deliveries", refresh);

  async function act(op: "delivery.replay" | "delivery.cancel", id: string) {
    await send({ op, deliveryId: id });
    void reload();
  }

  const columns: Column<Delivery>[] = [
    { key: "event", label: "Event", render: (d) => <span className="font-medium">{d.event}</span> },
    { key: "app", label: "Application", render: (d) => <code className="text-xs text-base-content/60">{d.application_id.slice(0, 10)}…</code> },
    { key: "status", label: "Status", render: (d) => <StatusBadge value={d.status} /> },
    { key: "attempts", label: "Attempts", render: (d) => <span className="tabular-nums">{d.attempts}</span> },
    { key: "next", label: "Next attempt", render: (d) => <span title={fmt(d.next_attempt_at)}>{timeAgo(d.next_attempt_at)}</span> },
    { key: "id", label: "Delivery ID", render: (d) => <code className="text-xs text-base-content/60">{d.id.slice(0, 10)}…</code> },
    {
      key: "actions",
      label: "",
      className: "text-right",
      render: (d) => (
        <div className="flex justify-end gap-1">
          <button
            className="btn btn-ghost btn-xs"
            disabled={d.status !== "blocked" && d.status !== "cancelled"}
            onClick={() => void act("delivery.replay", d.id)}
          >
            replay
          </button>
          <button
            className="btn btn-ghost btn-xs text-error"
            disabled={d.status !== "pending" && d.status !== "blocked"}
            onClick={() => void act("delivery.cancel", d.id)}
          >
            cancel
          </button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Deliveries</h2>
          <p className="text-sm text-base-content/60">
            Durable attempts. Replay blocked work or cancel it. Delivery ID is stable across retries.
          </p>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => void reload()}>Refresh</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No deliveries." />
    </div>
  );
}
