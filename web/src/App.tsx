import { useCallback, useEffect, useState } from "react";
import { AdminProvider, useAdmin } from "./lib/admin";
import { Login } from "./views/Login";
import { Overview } from "./views/Overview";
import { Apps } from "./views/Apps";
import { Schedules } from "./views/Schedules";
import { Deliveries } from "./views/Deliveries";
import { Workflows } from "./views/Workflows";
import { Calendars } from "./views/Calendars";
import { History } from "./views/History";
import { Publish } from "./views/Publish";

type NavKey =
  | "overview"
  | "apps"
  | "schedules"
  | "deliveries"
  | "workflows"
  | "calendars"
  | "history"
  | "publish";

const NAV: { key: NavKey; label: string; icon: string }[] = [
  { key: "overview", label: "Overview", icon: "◧" },
  { key: "apps", label: "Applications", icon: "◆" },
  { key: "schedules", label: "Schedules", icon: "◷" },
  { key: "deliveries", label: "Deliveries", icon: "➤" },
  { key: "workflows", label: "Workflows", icon: "⌘" },
  { key: "calendars", label: "Calendars", icon: "▦" },
  { key: "history", label: "History", icon: "≡" },
  { key: "publish", label: "Publish", icon: "＋" },
];

function Shell() {
  const { state, disconnect, error, connect } = useAdmin();
  const [nav, setNav] = useState<NavKey>("overview");
  const [refresh, setRefresh] = useState(0);
  const bump = useCallback(() => setRefresh((r) => r + 1), []);

  useEffect(() => {
    const hash = location.hash.replace("#", "") as NavKey;
    if (NAV.some((n) => n.key === hash)) setNav(hash);
  }, []);

  useEffect(() => {
    location.hash = nav;
  }, [nav]);

  if (state !== "open") {
    return <Login onConnect={connect} error={error} />;
  }

  return (
    <div className="flex min-h-screen bg-base-200">
      <aside className="w-60 shrink-0 border-r border-base-300 bg-base-100 flex flex-col">
        <div className="flex items-center gap-3 px-5 py-5 border-b border-base-300">
          <div className="grid h-9 w-9 place-items-center rounded-box bg-primary text-primary-content font-bold">
            C
          </div>
          <div className="leading-tight">
            <div className="font-semibold">Calcron</div>
            <div className="text-xs text-base-content/50">operations</div>
          </div>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {NAV.map((n) => (
            <button
              key={n.key}
              onClick={() => setNav(n.key)}
              className={`btn btn-ghost btn-sm w-full justify-start gap-3 font-normal ${
                nav === n.key ? "btn-active" : ""
              }`}
            >
              <span className="w-4 text-center text-base-content/60">{n.icon}</span>
              {n.label}
            </button>
          ))}
        </nav>
        <div className="p-3 border-t border-base-300">
          <button className="btn btn-ghost btn-sm w-full justify-start" onClick={disconnect}>
            Disconnect
          </button>
        </div>
      </aside>

      <main className="flex-1 min-w-0">
        <header className="flex items-center justify-between border-b border-base-300 bg-base-100 px-6 py-3">
          <h1 className="text-sm font-medium text-base-content/60">
            {NAV.find((n) => n.key === nav)?.label}
          </h1>
          <div className="flex items-center gap-3">
            <span className="badge badge-success badge-sm gap-1">
              <span className="inline-block h-1.5 w-1.5 rounded-full bg-success-content" />
              connected
            </span>
            <button className="btn btn-ghost btn-xs" onClick={bump}>
              refresh
            </button>
          </div>
        </header>
        <div className="p-6 max-w-7xl">
          {nav === "overview" && <Overview refresh={refresh} />}
          {nav === "apps" && <Apps refresh={refresh} bump={bump} />}
          {nav === "schedules" && <Schedules refresh={refresh} />}
          {nav === "deliveries" && <Deliveries refresh={refresh} />}
          {nav === "workflows" && <Workflows refresh={refresh} />}
          {nav === "calendars" && <Calendars refresh={refresh} />}
          {nav === "history" && <History refresh={refresh} />}
          {nav === "publish" && <Publish />}
        </div>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <AdminProvider>
      <Shell />
    </AdminProvider>
  );
}
