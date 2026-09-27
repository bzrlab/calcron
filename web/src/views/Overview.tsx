import { useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useShell } from "../lib/shell";

type Stats = {
  scheduled: number;
  pending: number;
  blocked: number;
  workflows: number;
};

const CARDS: { key: keyof Stats; label: string; hint: string; tone: string; view: string }[] = [
  { key: "scheduled", label: "Scheduled", hint: "waiting for deadline", tone: "text-info", view: "schedules" },
  { key: "pending", label: "Pending deliveries", hint: "awaiting acknowledgement", tone: "text-warning", view: "deliveries" },
  { key: "blocked", label: "Blocked", hint: "needs replay or cancel", tone: "text-error", view: "deliveries" },
  { key: "workflows", label: "Active workflows", hint: "instances not completed", tone: "text-success", view: "workflows" },
];

export function Overview() {
  const { send, state } = useAdmin();
  const { refresh, markUpdated, go, apps } = useShell();
  const [stats, setStats] = useState<Stats | null>(null);

  useEffect(() => {
    if (state !== "open") return;
    void send({ op: "dashboard.stats" }).then((r) => {
      if (r.ok) {
        setStats(r.data as Stats);
        markUpdated();
      }
    });
  }, [send, state, refresh, markUpdated]);

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Overview</h2>
        <p className="text-sm text-base-content/60">Live counts. Select a card to open the matching view.</p>
      </header>

      {stats && stats.blocked > 0 && (
        <div role="alert" className="alert alert-error">
          <span>{stats.blocked} blocked {stats.blocked === 1 ? "delivery needs" : "deliveries need"} replay or cancel.</span>
          <button className="btn btn-sm" onClick={() => go("deliveries")}>Open Deliveries</button>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {CARDS.map((c) => (
          <button
            key={c.key}
            type="button"
            onClick={() => go(c.view)}
            className="card border border-base-300 bg-base-100 text-left transition-colors hover:border-primary/60"
          >
            <div className="card-body gap-1">
              <span className="text-xs uppercase tracking-wider text-base-content/50">{c.label}</span>
              <span className={`text-3xl font-semibold tabular-nums ${c.tone}`}>{stats ? stats[c.key] : "—"}</span>
              <span className="text-xs text-base-content/50">{c.hint}</span>
            </div>
          </button>
        ))}
      </div>

      {apps.length === 0 && (
        <div className="card border border-dashed border-base-300 bg-base-100">
          <div className="card-body items-start">
            <h3 className="font-semibold">No applications yet</h3>
            <p className="text-sm text-base-content/60">
              Register an application to get a token. Its SDK then creates Schedules and starts Workflow instances.
            </p>
            <button className="btn btn-primary btn-sm" onClick={() => go("apps")}>Register application</button>
          </div>
        </div>
      )}
    </div>
  );
}
