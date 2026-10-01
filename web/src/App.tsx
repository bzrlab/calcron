import { useEffect, useState } from "react";
import { AdminProvider, useAdmin } from "./lib/admin";
import { ShellProvider, useShell } from "./lib/shell";
import { timeAgo } from "./lib/format";
import { CommandPalette } from "./components/CommandPalette";
import { Login } from "./views/Login";
import { Overview } from "./views/Overview";
import { Apps } from "./views/Apps";
import { Schedules } from "./views/Schedules";
import { Deliveries } from "./views/Deliveries";
import { Workflows } from "./views/Workflows";
import { Calendars } from "./views/Calendars";
import { CalendarView } from "./views/CalendarView";
import { History } from "./views/History";
import { Publish } from "./views/Publish";
import { Docs } from "./views/Docs";

const GROUPS: { label: string; items: { key: string; label: string; view: () => React.ReactNode; wide?: boolean }[] }[] = [
  {
    label: "Operate",
    items: [
      { key: "overview", label: "Overview", view: () => <Overview /> },
      { key: "calendar", label: "Calendar", view: () => <CalendarView />, wide: true },
      { key: "schedules", label: "Schedules", view: () => <Schedules /> },
      { key: "workflows", label: "Workflows", view: () => <Workflows /> },
      { key: "deliveries", label: "Deliveries", view: () => <Deliveries /> },
    ],
  },
  {
    label: "Configure",
    items: [
      { key: "apps", label: "Applications", view: () => <Apps /> },
      { key: "calendars", label: "Calendars", view: () => <Calendars />, wide: true },
      { key: "publish", label: "Publish", view: () => <Publish /> },
    ],
  },
  { label: "Audit", items: [{ key: "history", label: "History", view: () => <History /> }] },
];
const NAV = GROUPS.flatMap((g) => g.items);

const PILL: Record<string, { cls: string; text: string }> = {
  open: { cls: "badge-success", text: "connected" },
  connecting: { cls: "badge-warning", text: "reconnecting" },
  closed: { cls: "badge-error", text: "disconnected" },
  idle: { cls: "badge-ghost", text: "idle" },
};

function Freshness() {
  const { updatedAt } = useShell();
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  if (!updatedAt) return null;
  return <span className="text-xs text-base-content/50">updated {timeAgo(new Date(updatedAt).toISOString())}</span>;
}

function Shell() {
  const { state, disconnect } = useAdmin();
  const { view, go, bump } = useShell();
  const current = NAV.find((n) => n.key === view) ?? NAV[0];
  const pill = PILL[state];

  return (
    <div className="flex min-h-screen bg-base-200">
      <aside className="sticky top-0 flex h-screen w-60 shrink-0 flex-col border-r border-base-300 bg-base-100">
        <div className="flex items-center gap-3 border-b border-base-300 px-5 py-5">
          <div className="grid h-9 w-9 place-items-center rounded-box bg-primary font-bold text-primary-content">C</div>
          <div className="leading-tight">
            <div className="font-semibold">Calcron</div>
            <div className="text-xs text-base-content/50">operations</div>
          </div>
        </div>
        <div className="p-3">
          <CommandPalette nav={NAV} />
        </div>
        <nav className="flex-1 space-y-4 overflow-y-auto px-3 pb-3">
          {GROUPS.map((g) => (
            <div key={g.label}>
              <div className="px-3 pb-1 text-[0.65rem] uppercase tracking-widest text-base-content/40">{g.label}</div>
              {g.items.map((n) => (
                <button
                  key={n.key}
                  onClick={() => go(n.key)}
                  aria-current={current.key === n.key ? "page" : undefined}
                  className={`btn btn-ghost btn-sm w-full justify-start font-normal ${current.key === n.key ? "btn-active" : ""}`}
                >
                  {n.label}
                </button>
              ))}
            </div>
          ))}
        </nav>
        <div className="border-t border-base-300 p-3">
          <button className="btn btn-ghost btn-sm w-full justify-start" onClick={disconnect}>Disconnect</button>
        </div>
      </aside>

      <main className="min-w-0 flex-1">
        <header className="sticky top-0 z-30 flex items-center justify-between border-b border-base-300 bg-base-100 px-6 py-3">
          <h1 className="text-sm font-medium text-base-content/60">{current.label}</h1>
          <div className="flex items-center gap-3">
            <Freshness />
            <span className={`badge badge-sm gap-1 ${pill.cls}`} role="status">
              <span className="inline-block h-1.5 w-1.5 rounded-full bg-current" />
              {pill.text}
            </span>
            <button className="btn btn-ghost btn-xs" onClick={bump} disabled={state !== "open"}>refresh</button>
          </div>
        </header>
        <div className={`p-6 ${current.wide ? "" : "max-w-7xl"}`}>{current.view()}</div>
      </main>
    </div>
  );
}

function Gate() {
  const { session, connect, error } = useAdmin();
  if (!session) return <Login onConnect={connect} error={error} />;
  return (
    <ShellProvider defaultView="overview">
      <Shell />
    </ShellProvider>
  );
}

export default function App() {
  if (location.pathname === "/docs" || location.pathname === "/docs/") return <Docs />;
  return (
    <AdminProvider>
      <Gate />
    </AdminProvider>
  );
}
