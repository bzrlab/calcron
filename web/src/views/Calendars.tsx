import { useList } from "../lib/useList";
import { DataTable, type Column } from "../components/DataTable";
import { Json } from "../components/Json";
import { fmt } from "../lib/format";

type Calendar = {
  application_id: string;
  name: string;
  definition: {
    timezone: string;
    weekdays: number[];
    overrides?: Record<string, boolean>;
  };
  updated_at: string;
};

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export function Calendars({ refresh }: { refresh: number }) {
  const { rows, loading, reload } = useList<Calendar>("calendars", refresh);

  const columns: Column<Calendar>[] = [
    { key: "name", label: "Calendar", render: (c) => <span className="font-medium">{c.name}</span> },
    { key: "app", label: "Application", render: (c) => <code className="text-xs text-base-content/60">{c.application_id.slice(0, 10)}…</code> },
    { key: "tz", label: "Timezone", render: (c) => <code className="text-xs">{c.definition?.timezone ?? "—"}</code> },
    {
      key: "days",
      label: "Eligible days",
      render: (c) => (
        <div className="flex gap-1">
          {DAYS.map((d, i) => {
            const on = c.definition?.weekdays?.includes(i);
            return (
              <span
                key={d}
                className={`badge badge-sm ${on ? "badge-success" : "badge-ghost"}`}
                title={d}
              >
                {d[0]}
              </span>
            );
          })}
        </div>
      ),
    },
    { key: "def", label: "Definition", render: (c) => <Json value={c.definition} /> },
    { key: "updated", label: "Updated", render: (c) => <span className="text-xs text-base-content/60">{fmt(c.updated_at)}</span> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Calendars</h2>
          <p className="text-sm text-base-content/60">
            Named timezone-aware business calendars. Eligible weekdays and date overrides.
          </p>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => void reload()}>Refresh</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No calendars." />
    </div>
  );
}
