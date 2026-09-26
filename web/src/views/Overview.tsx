import { useCallback, useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";

type Stats = {
  scheduled: number;
  pending: number;
  blocked: number;
  workflows: number;
};

const CARDS: { key: keyof Stats; label: string; hint: string; tone: string }[] = [
  { key: "scheduled", label: "Scheduled", hint: "waiting for deadline", tone: "text-info" },
  { key: "pending", label: "Pending deliveries", hint: "awaiting acknowledgement", tone: "text-warning" },
  { key: "blocked", label: "Blocked", hint: "needs replay or cancel", tone: "text-error" },
  { key: "workflows", label: "Active workflows", hint: "instances not completed", tone: "text-success" },
];

export function Overview({ refresh }: { refresh: number }) {
  const { send, state } = useAdmin();
  const [stats, setStats] = useState<Stats | null>(null);

  const load = useCallback(async () => {
    const r = await send({ op: "dashboard.stats" });
    if (r.ok) setStats(r.data as Stats);
  }, [send]);

  useEffect(() => {
    if (state !== "open") return;
    void load();
    const t = setInterval(() => void load(), 4000);
    return () => clearInterval(t);
  }, [state, load, refresh]);

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Overview</h2>
        <p className="text-sm text-base-content/60">
          Live counts. Refresh every 4s.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {CARDS.map((c) => (
          <div
            key={c.key}
            className="card bg-base-100 border border-base-300"
          >
            <div className="card-body gap-1">
              <span className="text-xs uppercase tracking-wider text-base-content/50">
                {c.label}
              </span>
              <span className={`text-3xl font-semibold tabular-nums ${c.tone}`}>
                {stats ? stats[c.key] : "—"}
              </span>
              <span className="text-xs text-base-content/50">{c.hint}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
